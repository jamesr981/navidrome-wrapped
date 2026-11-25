package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/navidrome/navidrome/plugins/pdk/go/host"
)

// fetchPlays returns every scrobble for the user between from and to (unix seconds, inclusive).
func fetchPlays(username string, from, to int64) ([]Play, error) {
	var plays []Play
	opts := host.ScrobbleOptions{FromTimestamp: &from, ToTimestamp: &to, MaxItems: 5000}
	for page := 0; page < 10000; page++ {
		refs, next, err := host.ScrobbleRetrieverGetScrobbles(username, opts)
		if err != nil {
			return nil, fmt.Errorf("reading scrobbles: %w", err)
		}
		for _, r := range refs {
			if r.MediaFileID != "" {
				plays = append(plays, Play{TrackID: r.MediaFileID, Time: r.SubmissionTime})
			}
		}
		if next == nil {
			break
		}
		opts = *next
	}
	return plays, nil
}

// subsonic response envelopes (only the fields we use)
type ssError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type ssSong struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	ArtistID string `json:"artistId"`
	Album    string `json:"album"`
	AlbumID  string `json:"albumId"`
	CoverArt string `json:"coverArt"`
	Duration int    `json:"duration"`
	Year     int    `json:"year"`
	Genre    string `json:"genre"`
	Genres   []struct {
		Name string `json:"name"`
	} `json:"genres"`
	Artists []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"artists"`
	DisplayArtist string `json:"displayArtist"`
}

type ssResponse struct {
	Resp struct {
		Status   string   `json:"status"`
		Error    *ssError `json:"error"`
		Song     *ssSong  `json:"song"`
		Playlist *struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"playlist"`
		Playlists *struct {
			Playlist []struct {
				ID    string `json:"id"`
				Name  string `json:"name"`
				Owner string `json:"owner"`
			} `json:"playlist"`
		} `json:"playlists"`
	} `json:"subsonic-response"`
}

func subsonic(username, endpoint string, params url.Values) (*ssResponse, error) {
	if params == nil {
		params = url.Values{}
	}
	params.Set("u", username)
	raw, err := host.SubsonicAPICall(endpoint + "?" + params.Encode())
	if err != nil {
		return nil, err
	}
	var r ssResponse
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return nil, fmt.Errorf("%s: bad response: %w", endpoint, err)
	}
	if r.Resp.Status != "ok" {
		if r.Resp.Error != nil {
			return &r, fmt.Errorf("%s: %s (code %d)", endpoint, r.Resp.Error.Message, r.Resp.Error.Code)
		}
		return &r, fmt.Errorf("%s: status %q", endpoint, r.Resp.Status)
	}
	return &r, nil
}

// resolveTracks looks up metadata for every distinct track id. Tracks that no longer
// exist in the library are skipped (their plays are dropped from the stats).
func resolveTracks(username string, ids map[string]struct{}) map[string]Track {
	out := make(map[string]Track, len(ids))
	for id := range ids {
		r, err := subsonic(username, "getSong", url.Values{"id": {id}})
		if err != nil || r.Resp.Song == nil {
			continue
		}
		s := r.Resp.Song
		t := Track{
			ID: s.ID, Title: s.Title, Album: s.Album, AlbumID: s.AlbumID,
			CoverArt: s.CoverArt, Duration: s.Duration, Year: s.Year,
			Artist: s.DisplayArtist,
		}
		if t.Artist == "" {
			t.Artist = s.Artist
		}
		for _, a := range s.Artists {
			if a.Name != "" {
				t.Artists = append(t.Artists, ArtistRef{ID: a.ID, Name: a.Name})
			}
		}
		if len(t.Artists) == 0 && s.Artist != "" {
			t.Artists = []ArtistRef{{ID: s.ArtistID, Name: s.Artist}}
		}
		for _, g := range s.Genres {
			t.Genres = append(t.Genres, g.Name)
		}
		if len(t.Genres) == 0 && s.Genre != "" {
			t.Genres = []string{s.Genre}
		}
		out[id] = t
	}
	return out
}

// coverDataURI fetches cover art through the Subsonic API and returns it as a data URI.
func coverDataURI(username, id string, size int, cache map[string]string) string {
	if id == "" {
		return ""
	}
	key := fmt.Sprintf("%s@%d", id, size)
	if v, ok := cache[key]; ok {
		return v
	}
	ct, data, err := host.SubsonicAPICallRaw(fmt.Sprintf("getCoverArt?id=%s&size=%d&u=%s",
		url.QueryEscape(id), size, url.QueryEscape(username)))
	v := ""
	if err == nil && strings.HasPrefix(ct, "image/") && len(data) > 200 {
		v = "data:" + ct + ";base64," + base64.StdEncoding.EncodeToString(data)
	}
	cache[key] = v
	return v
}

// upsertPlaylist replaces the user's Wrapped playlist for the year with the given tracks.
func upsertPlaylist(username, name, comment string, trackIDs []string) (string, error) {
	if len(trackIDs) == 0 {
		return "", nil
	}
	// remove any previous version (by remembered id, and by exact name owned by the user)
	kvKey := "playlist:" + username + ":" + name
	if prev, ok, _ := host.KVStoreGet(kvKey); ok && len(prev) > 0 {
		_, _ = subsonic(username, "deletePlaylist", url.Values{"id": {string(prev)}})
	}
	if r, err := subsonic(username, "getPlaylists", nil); err == nil && r.Resp.Playlists != nil {
		for _, p := range r.Resp.Playlists.Playlist {
			if p.Name == name && (p.Owner == "" || p.Owner == username) {
				_, _ = subsonic(username, "deletePlaylist", url.Values{"id": {p.ID}})
			}
		}
	}

	params := url.Values{"name": {name}}
	for _, id := range trackIDs {
		params.Add("songId", id)
	}
	r, err := subsonic(username, "createPlaylist", params)
	if err != nil {
		return "", err
	}
	if r.Resp.Playlist == nil || r.Resp.Playlist.ID == "" {
		return "", fmt.Errorf("createPlaylist returned no playlist id")
	}
	id := r.Resp.Playlist.ID
	_ = host.KVStoreSet(kvKey, []byte(id))
	if _, err := subsonic(username, "updatePlaylist", url.Values{
		"playlistId": {id}, "comment": {comment}, "public": {"false"},
	}); err != nil {
		logWarn("could not set playlist comment for " + username + ": " + err.Error())
	}
	return id, nil
}
