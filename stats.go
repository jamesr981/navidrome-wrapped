package main

import (
	"sort"
	"strings"
	"time"
)

// Play is a single scrobble: which track, and when (unix seconds).
type Play struct {
	TrackID string
	Time    int64
}

// ArtistRef is one credited artist on a track.
type ArtistRef struct {
	ID   string
	Name string
}

// Track is the metadata we need about a played song.
type Track struct {
	ID       string
	Title    string
	Artist   string      // display artist ("A feat. B")
	Artists  []ArtistRef // individual credited artists
	AlbumID  string
	Album    string
	CoverArt string
	Duration int // seconds
	Year     int
	Genres   []string
}

// Ranked is one entry in a top-N list.
type Ranked struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Subtitle string  `json:"subtitle,omitempty"`
	Plays    int     `json:"plays"`
	Minutes  int     `json:"minutes"`
	Share    float64 `json:"share,omitempty"` // share of total plays, 0..1
	CoverArt string  `json:"coverArt,omitempty"`
	Image    string  `json:"image,omitempty"` // data URI, filled in by the renderer
	// FallbackArt is a cover-art id to try when CoverArt yields no image (e.g. artists without photos).
	FallbackArt string `json:"-"`
}

// DayStat describes a notable day.
type DayStat struct {
	Date    string `json:"date"` // YYYY-MM-DD
	Plays   int    `json:"plays"`
	Minutes int    `json:"minutes"`
}

// Repeat is the song played the most times in a single day.
type Repeat struct {
	Song  Ranked `json:"song"`
	Date  string `json:"date"`
	Plays int    `json:"plays"`
}

// FirstPlay is the first song of the year.
type FirstPlay struct {
	Song Ranked `json:"song"`
	When string `json:"when"` // human readable
}

// Personality is the fun "listening personality" summary.
type Personality struct {
	Code    string   `json:"code"`
	Name    string   `json:"name"`
	Tagline string   `json:"tagline"`
	Traits  []string `json:"traits"`
}

// Stats is everything that goes into a user's Wrapped.
type Stats struct {
	Username      string      `json:"username"`
	DisplayName   string      `json:"displayName"`
	Year          int         `json:"year"`
	Timezone      string      `json:"timezone"`
	GeneratedAt   string      `json:"generatedAt"`
	TotalPlays    int         `json:"totalPlays"`
	TotalMinutes  int         `json:"totalMinutes"`
	UniqueTracks  int         `json:"uniqueTracks"`
	UniqueArtists int         `json:"uniqueArtists"`
	UniqueAlbums  int         `json:"uniqueAlbums"`
	TopArtists    []Ranked    `json:"topArtists"`
	TopSongs      []Ranked    `json:"topSongs"`
	TopAlbums     []Ranked    `json:"topAlbums"`
	TopGenres     []Ranked    `json:"topGenres"`
	TopDecade     string      `json:"topDecade,omitempty"`
	HourPlays     [24]int     `json:"hourPlays"`
	MonthMinutes  [12]int     `json:"monthMinutes"`
	WeekdayPlays  [7]int      `json:"weekdayPlays"` // Monday first
	PeakHour      int         `json:"peakHour"`
	TopMonth      int         `json:"topMonth"` // 1..12
	BiggestDay    DayStat     `json:"biggestDay"`
	ActiveDays    int         `json:"activeDays"`
	LongestStreak int         `json:"longestStreak"`
	StreakStart   string      `json:"streakStart,omitempty"`
	StreakEnd     string      `json:"streakEnd,omitempty"`
	OnRepeat      *Repeat     `json:"onRepeat,omitempty"`
	FirstSong     *FirstPlay  `json:"firstSong,omitempty"`
	Personality   Personality `json:"personality"`
	// PlaylistTrackIDs is the ordered list of top tracks used for the playlist (not rendered).
	PlaylistTrackIDs []string `json:"-"`
}

type counter struct {
	plays   int
	seconds int
}

// Compute aggregates plays into Wrapped stats. Plays whose track is unknown are ignored.
// topN controls the length of the top lists; playlistSize the length of PlaylistTrackIDs.
func Compute(plays []Play, tracks map[string]Track, loc *time.Location, year, topN, playlistSize int) Stats {
	if loc == nil {
		loc = time.UTC
	}
	st := Stats{Year: year, Timezone: loc.String()}

	trackC := map[string]*counter{}
	artistC := map[string]*counter{}
	artistName := map[string]string{}
	artistTracks := map[string]map[string]int{} // artist -> track -> plays (for cover art pick)
	albumC := map[string]*counter{}
	genreC := map[string]*counter{}
	decadeC := map[int]int{}
	dayC := map[string]*counter{}
	dayTrack := map[string]map[string]int{}
	recentPlays := 0
	totalSeconds := 0

	var first *Play
	sort.SliceStable(plays, func(i, j int) bool { return plays[i].Time < plays[j].Time })

	bump := func(m map[string]*counter, k string, secs int) {
		c := m[k]
		if c == nil {
			c = &counter{}
			m[k] = c
		}
		c.plays++
		c.seconds += secs
	}

	for i := range plays {
		p := plays[i]
		t, ok := tracks[p.TrackID]
		if !ok {
			continue
		}
		when := time.Unix(p.Time, 0).In(loc)
		if when.Year() != year {
			continue
		}
		if first == nil {
			first = &plays[i]
		}
		secs := t.Duration
		if secs <= 0 {
			secs = 180 // unknown duration: assume a typical 3 minute song
		}
		st.TotalPlays++
		totalSeconds += secs

		bump(trackC, t.ID, secs)
		for _, a := range creditedArtists(t) {
			key := a.ID
			if key == "" {
				key = "name:" + strings.ToLower(a.Name)
			}
			bump(artistC, key, secs)
			if artistName[key] == "" {
				artistName[key] = a.Name
			}
			if artistTracks[key] == nil {
				artistTracks[key] = map[string]int{}
			}
			artistTracks[key][t.ID]++
		}
		if t.AlbumID != "" {
			bump(albumC, t.AlbumID, secs)
		}
		for _, g := range t.Genres {
			g = strings.TrimSpace(g)
			if g != "" {
				bump(genreC, g, secs)
			}
		}
		if t.Year > 0 {
			decadeC[t.Year/10*10]++
			if t.Year >= year-1 {
				recentPlays++
			}
		}

		day := when.Format("2006-01-02")
		bump(dayC, day, secs)
		if dayTrack[day] == nil {
			dayTrack[day] = map[string]int{}
		}
		dayTrack[day][t.ID]++

		st.HourPlays[when.Hour()]++
		st.MonthMinutes[int(when.Month())-1] += secs
		st.WeekdayPlays[(int(when.Weekday())+6)%7]++
	}

	if st.TotalPlays == 0 {
		return st
	}
	st.TotalMinutes = totalSeconds / 60
	for i := range st.MonthMinutes {
		st.MonthMinutes[i] /= 60
	}
	st.UniqueTracks = len(trackC)
	st.UniqueArtists = len(artistC)
	st.UniqueAlbums = len(albumC)

	trackRanked := rank(trackC, func(id string) Ranked {
		t := tracks[id]
		return Ranked{ID: id, Name: t.Title, Subtitle: t.Artist, CoverArt: coverFor(t)}
	}, st.TotalPlays)
	st.TopSongs = head(trackRanked, topN)
	for _, r := range head(trackRanked, playlistSize) {
		st.PlaylistTrackIDs = append(st.PlaylistTrackIDs, r.ID)
	}

	st.TopArtists = head(rank(artistC, func(id string) Ranked {
		r := Ranked{ID: id, Name: artistName[id]}
		if strings.HasPrefix(id, "name:") {
			r.ID = ""
		} else {
			r.CoverArt = "ar-" + id
		}
		// fallback art: cover of the artist's most played track
		best, bestN := "", 0
		for tid, n := range artistTracks[id] {
			if n > bestN || (n == bestN && tid < best) {
				best, bestN = tid, n
			}
		}
		if t, ok := tracks[best]; ok {
			r.FallbackArt = coverFor(t)
		}
		return r
	}, st.TotalPlays), topN)

	albumInfo := map[string]Track{}
	for _, t := range tracks {
		if t.AlbumID != "" {
			if _, ok := albumInfo[t.AlbumID]; !ok {
				albumInfo[t.AlbumID] = t
			}
		}
	}
	st.TopAlbums = head(rank(albumC, func(id string) Ranked {
		t := albumInfo[id]
		return Ranked{ID: id, Name: t.Album, Subtitle: t.Artist, CoverArt: coverFor(t)}
	}, st.TotalPlays), topN)
	st.TopGenres = head(rank(genreC, func(id string) Ranked { return Ranked{ID: id, Name: id} }, st.TotalPlays), topN)

	// decade
	bestDecade, bestDecadeN := 0, 0
	for d, n := range decadeC {
		if n > bestDecadeN || (n == bestDecadeN && d > bestDecade) {
			bestDecade, bestDecadeN = d, n
		}
	}
	if bestDecadeN > 0 {
		st.TopDecade = itoa(bestDecade) + "s"
	}

	// peak hour / month
	st.PeakHour = argmax(st.HourPlays[:])
	st.TopMonth = argmax(st.MonthMinutes[:]) + 1

	// days
	days := make([]string, 0, len(dayC))
	for d := range dayC {
		days = append(days, d)
	}
	sort.Strings(days)
	st.ActiveDays = len(days)
	for _, d := range days {
		c := dayC[d]
		if c.plays > st.BiggestDay.Plays {
			st.BiggestDay = DayStat{Date: d, Plays: c.plays, Minutes: c.seconds / 60}
		}
	}
	st.LongestStreak, st.StreakStart, st.StreakEnd = longestStreak(days)

	// on repeat: most plays of a single song in one day (needs at least 3)
	var rep *Repeat
	for _, d := range days {
		for tid, n := range dayTrack[d] {
			if n < 3 {
				continue
			}
			if rep == nil || n > rep.Plays || (n == rep.Plays && tid < rep.Song.ID) {
				t := tracks[tid]
				rep = &Repeat{Song: Ranked{ID: tid, Name: t.Title, Subtitle: t.Artist, CoverArt: coverFor(t)}, Date: d, Plays: n}
			}
		}
	}
	st.OnRepeat = rep

	if first != nil {
		t := tracks[first.TrackID]
		st.FirstSong = &FirstPlay{
			Song: Ranked{ID: t.ID, Name: t.Title, Subtitle: t.Artist, CoverArt: coverFor(t)},
			When: time.Unix(first.Time, 0).In(loc).Format("Monday 2 January, 3:04 PM"),
		}
	}

	st.Personality = personality(st, recentPlays)
	return st
}

func creditedArtists(t Track) []ArtistRef {
	if len(t.Artists) > 0 {
		return t.Artists
	}
	if t.Artist == "" {
		return nil
	}
	return []ArtistRef{{Name: t.Artist}}
}

func coverFor(t Track) string {
	if t.CoverArt != "" {
		return t.CoverArt
	}
	return t.AlbumID
}

func rank(m map[string]*counter, mk func(string) Ranked, total int) []Ranked {
	out := make([]Ranked, 0, len(m))
	for id, c := range m {
		r := mk(id)
		r.Plays = c.plays
		r.Minutes = c.seconds / 60
		if total > 0 {
			r.Share = float64(c.plays) / float64(total)
		}
		if r.ID == "" && r.Name == "" {
			r.Name = id
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Plays != out[j].Plays {
			return out[i].Plays > out[j].Plays
		}
		if out[i].Minutes != out[j].Minutes {
			return out[i].Minutes > out[j].Minutes
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func head(r []Ranked, n int) []Ranked {
	if n >= 0 && len(r) > n {
		return r[:n]
	}
	return r
}

func argmax(v []int) int {
	best := 0
	for i := range v {
		if v[i] > v[best] {
			best = i
		}
	}
	return best
}

func longestStreak(sortedDays []string) (int, string, string) {
	best, bestStart, bestEnd := 0, "", ""
	cur, curStart := 0, ""
	var prev time.Time
	for _, d := range sortedDays {
		t, err := time.Parse("2006-01-02", d)
		if err != nil {
			continue
		}
		if cur > 0 && t.Sub(prev) == 24*time.Hour {
			cur++
		} else {
			cur, curStart = 1, d
		}
		if cur > best {
			best, bestStart, bestEnd = cur, curStart, d
		}
		prev = t
	}
	return best, bestStart, bestEnd
}

// personality picks a fun archetype from listening behaviour, in the spirit of Spotify's
// listening personalities. Four axes give a code like "LNRD":
//
//	L/E  Loyal (top artist ≥ 12% of plays) vs Explorer
//	N/T  New (≥ 40% of dated plays from this or last year) vs Timeless
//	R/V  Repeat (≤ 35% of plays are distinct songs) vs Variety
//	D/N  Day vs Night (peak hour 21:00–04:59 counts as night)
func personality(st Stats, recentPlays int) Personality {
	topShare := 0.0
	if len(st.TopArtists) > 0 {
		topShare = st.TopArtists[0].Share
	}
	loyal := topShare >= 0.12
	newish := float64(recentPlays) >= 0.4*float64(st.TotalPlays)
	repeat := float64(st.UniqueTracks) <= 0.35*float64(st.TotalPlays)
	night := st.PeakHour >= 21 || st.PeakHour < 5

	code := pick(loyal, "L", "E") + pick(newish, "N", "T") + pick(repeat, "R", "V") + pick(night, "N", "D")

	var traits []string
	traits = append(traits, pick(loyal, "Loyal", "Explorer"))
	traits = append(traits, pick(newish, "Trendsetter", "Timeless"))
	traits = append(traits, pick(repeat, "Repeater", "Variety seeker"))
	traits = append(traits, pick(night, "Night owl", "Daylight listener"))

	type archetype struct{ name, tagline string }
	var a archetype
	switch {
	case loyal && repeat && night:
		a = archetype{"The Devotee", "Lights off, same artist, same songs, again and again. You know every word."}
	case loyal && repeat:
		a = archetype{"The Specialist", "You found what you love and you went deep. Really deep."}
	case loyal && newish:
		a = archetype{"The Superfan", "First to every release from the artists you ride for."}
	case loyal:
		a = archetype{"The Fan Club President", "One artist ran your year, and you were happy to let them."}
	case !loyal && newish && !repeat:
		a = archetype{"The Trailblazer", "Always on the hunt for the next new thing before anyone else hears it."}
	case !loyal && !newish && !repeat:
		a = archetype{"The Time Traveler", "Decades are just suggestions. You dig through every era of music."}
	case !loyal && repeat && night:
		a = archetype{"The Night Shift DJ", "A wide crew of artists, but you know exactly which tracks get the 2am rotation."}
	default:
		a = archetype{"The Replayer", "You roam widely, but when a song hits, it lives on repeat."}
	}
	return Personality{Code: code, Name: a.name, Tagline: a.tagline, Traits: traits}
}

func pick(c bool, a, b string) string {
	if c {
		return a
	}
	return b
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
