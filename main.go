// Navidrome Wrapped: a Spotify Wrapped-style "year in music" for every Navidrome user.
//
// Build:  make            (needs TinyGo; see README.md)
// Output: navidrome-wrapped.ndp  → copy into your Navidrome plugins folder.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // the wasm sandbox has no system zoneinfo

	"github.com/navidrome/navidrome/plugins/pdk/go/host"
	"github.com/navidrome/navidrome/plugins/pdk/go/lifecycle"
	"github.com/navidrome/navidrome/plugins/pdk/go/pdk"
	"github.com/navidrome/navidrome/plugins/pdk/go/scheduler"
)

const (
	recurringScheduleID = "wrapped-annual"
	oneTimeScheduleID   = "wrapped-now"
	topN                = 5
)

// version is set at build time by the Makefile (-ldflags "-X main.version=...")
var version = "0.0.0-dev"

type wrappedPlugin struct{}

func init() {
	p := &wrappedPlugin{}
	lifecycle.Register(p)
	scheduler.Register(p)
}

func main() {}

// ---------- configuration ----------

type config struct {
	Year           int
	Location       *time.Location
	Schedule       string
	GenerateOnLoad bool
	MinPlays       int
	CreatePlaylist bool
	PlaylistSize   int
	EmbedArtwork   bool
}

func cfgString(key, def string) string {
	if v, ok := host.ConfigGet(key); ok {
		return strings.TrimSpace(v)
	}
	return def
}

func cfgInt(key string, def int) int {
	if v, ok := host.ConfigGet(key); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return def
}

func cfgBool(key string, def bool) bool {
	if v, ok := host.ConfigGet(key); ok {
		if b, err := strconv.ParseBool(strings.TrimSpace(v)); err == nil {
			return b
		}
	}
	return def
}

func loadConfig() config {
	c := config{
		Year:           cfgInt("year", 0),
		Schedule:       cfgString("schedule", "0 6 1 12 *"),
		GenerateOnLoad: cfgBool("generateOnLoad", false),
		MinPlays:       cfgInt("minPlays", 25),
		CreatePlaylist: cfgBool("createPlaylist", true),
		PlaylistSize:   cfgInt("playlistSize", 50),
		EmbedArtwork:   cfgBool("embedArtwork", true),
		Location:       time.UTC,
	}
	if tz := cfgString("timezone", "UTC"); tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			c.Location = loc
		} else {
			logWarn("unknown timezone " + strconv.Quote(tz) + ", using UTC")
		}
	}
	if c.PlaylistSize < 1 {
		c.PlaylistSize = 50
	}
	if c.MinPlays < 1 {
		c.MinPlays = 1
	}
	if c.Year <= 0 {
		now := time.Now().In(c.Location)
		c.Year = now.Year()
		if now.Month() == time.January {
			c.Year-- // early in the year, people want last year's Wrapped
		}
	}
	return c
}

// ---------- lifecycle & scheduling ----------

func (p *wrappedPlugin) OnInit() error {
	c := loadConfig()
	logInfo("Navidrome Wrapped " + version)
	_ = host.SchedulerCancelSchedule(recurringScheduleID)
	if c.Schedule != "" {
		if _, err := host.SchedulerScheduleRecurring(c.Schedule, "run", recurringScheduleID); err != nil {
			logError("invalid schedule " + strconv.Quote(c.Schedule) + ": " + err.Error())
		} else {
			logInfo("Wrapped scheduled with cron " + strconv.Quote(c.Schedule))
		}
	}
	if c.GenerateOnLoad {
		// don't block plugin loading: run a few seconds later
		if _, err := host.SchedulerScheduleOneTime(5, "run", oneTimeScheduleID); err != nil {
			logError("could not schedule immediate run: " + err.Error())
		} else {
			logInfo(fmt.Sprintf("Generating Wrapped %d in 5 seconds", c.Year))
		}
	}
	return nil
}

func (p *wrappedPlugin) OnCallback(req scheduler.SchedulerCallbackRequest) error {
	return runAll(loadConfig())
}

// ---------- generation ----------

type runSummary struct {
	Year      int               `json:"year"`
	StartedAt string            `json:"startedAt"`
	Duration  string            `json:"duration"`
	Users     map[string]string `json:"users"` // username -> result
}

type indexEntry struct {
	Username    string
	DisplayName string
	File        string
	Plays       int
	Minutes     int
	TopArtist   string
	Personality string
}

func runAll(c config) error {
	start := time.Now()
	users, err := host.UsersGetUsers()
	if err != nil {
		return fmt.Errorf("listing users: %w", err)
	}
	if len(users) == 0 {
		logWarn("no users assigned to the Wrapped plugin; allow users in the plugin settings")
		return nil
	}

	outDir := path.Join(host.StorageGetStoragePath(), strconv.Itoa(c.Year))
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("creating output folder: %w", err)
	}

	sum := runSummary{Year: c.Year, StartedAt: start.Format(time.RFC3339), Users: map[string]string{}}
	var index []indexEntry
	for _, u := range users {
		st, res, err := runUser(c, u, outDir)
		if err != nil {
			logError("Wrapped failed for " + u.UserName + ": " + err.Error())
			sum.Users[u.UserName] = "error: " + err.Error()
			continue
		}
		sum.Users[u.UserName] = res
		if st != nil {
			top := ""
			if len(st.TopArtists) > 0 {
				top = st.TopArtists[0].Name
			}
			index = append(index, indexEntry{
				Username: u.UserName, DisplayName: st.DisplayName, File: safeName(u.UserName) + ".html",
				Plays: st.TotalPlays, Minutes: st.TotalMinutes, TopArtist: top, Personality: st.Personality.Name,
			})
		}
	}

	if html, err := renderIndex(c.Year, index); err == nil {
		_ = os.WriteFile(path.Join(outDir, "index.html"), html, 0o644)
	}
	sum.Duration = time.Since(start).Round(time.Second).String()
	if b, err := json.Marshal(sum); err == nil {
		_ = host.KVStoreSet("last-run", b)
		_ = os.WriteFile(path.Join(outDir, "last-run.json"), b, 0o644)
	}
	logInfo(fmt.Sprintf("Wrapped %d done for %d user(s) in %s", c.Year, len(index), sum.Duration))
	return nil
}

func runUser(c config, u host.User, outDir string) (*Stats, string, error) {
	from := time.Date(c.Year, 1, 1, 0, 0, 0, 0, c.Location).Unix()
	to := time.Date(c.Year+1, 1, 1, 0, 0, 0, 0, c.Location).Unix() - 1

	plays, err := fetchPlays(u.UserName, from, to)
	if err != nil {
		return nil, "", err
	}
	if len(plays) < c.MinPlays {
		return nil, fmt.Sprintf("skipped: %d plays (minimum %d)", len(plays), c.MinPlays), nil
	}

	ids := map[string]struct{}{}
	for _, p := range plays {
		ids[p.TrackID] = struct{}{}
	}
	tracks := resolveTracks(u.UserName, ids)
	if len(tracks) == 0 {
		return nil, "", errors.New("none of the played tracks could be found in the library")
	}

	st := Compute(plays, tracks, c.Location, c.Year, topN, c.PlaylistSize)
	if st.TotalPlays < c.MinPlays {
		return nil, fmt.Sprintf("skipped: %d plays (minimum %d)", st.TotalPlays, c.MinPlays), nil
	}
	st.Username = u.UserName
	st.DisplayName = u.Name
	if st.DisplayName == "" {
		st.DisplayName = u.UserName
	}
	st.GeneratedAt = time.Now().In(c.Location).Format("2 Jan 2006 15:04 MST")

	result := fmt.Sprintf("ok: %d plays, %d minutes", st.TotalPlays, st.TotalMinutes)

	if c.CreatePlaylist {
		name := fmt.Sprintf("Wrapped %d · Top Songs", c.Year)
		if _, err := upsertPlaylist(u.UserName, name, playlistComment(&st), st.PlaylistTrackIDs); err != nil {
			logWarn("playlist for " + u.UserName + ": " + err.Error())
			result += "; playlist failed: " + err.Error()
		} else {
			result += "; playlist created"
		}
	}

	if c.EmbedArtwork {
		embedImages(u.UserName, &st)
	}
	html, err := renderStory(&st)
	if err != nil {
		return nil, "", fmt.Errorf("rendering story: %w", err)
	}
	base := path.Join(outDir, safeName(u.UserName))
	if err := os.WriteFile(base+".html", html, 0o644); err != nil {
		return nil, "", fmt.Errorf("writing story: %w", err)
	}
	if b, err := json.MarshalIndent(st, "", "  "); err == nil {
		_ = os.WriteFile(base+".json", b, 0o644)
	}
	logInfo(fmt.Sprintf("Wrapped %d for %s: %d plays, %d min, top artist %q", c.Year, u.UserName,
		st.TotalPlays, st.TotalMinutes, firstName(st.TopArtists)))
	return &st, result, nil
}

func embedImages(username string, st *Stats) {
	cache := map[string]string{}
	fill := func(list []Ranked, size int) {
		for i := range list {
			list[i].Image = coverDataURI(username, list[i].CoverArt, size, cache)
			if list[i].Image == "" && list[i].FallbackArt != "" {
				list[i].Image = coverDataURI(username, list[i].FallbackArt, size, cache)
			}
		}
	}
	fill(st.TopArtists, 400)
	fill(st.TopSongs, 400)
	fill(st.TopAlbums, 400)
	if st.OnRepeat != nil {
		st.OnRepeat.Song.Image = coverDataURI(username, st.OnRepeat.Song.CoverArt, 400, cache)
	}
	if st.FirstSong != nil {
		st.FirstSong.Song.Image = coverDataURI(username, st.FirstSong.Song.CoverArt, 400, cache)
	}
}

// playlistComment is the summary that shows on the playlist page in Navidrome and other clients.
func playlistComment(st *Stats) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Your %d Wrapped 🎁 %s minutes · %s plays · %s artists.",
		st.Year, commas(st.TotalMinutes), commas(st.TotalPlays), commas(st.UniqueArtists))
	if len(st.TopArtists) > 0 {
		fmt.Fprintf(&b, " Top artist: %s.", st.TopArtists[0].Name)
	}
	if len(st.TopSongs) > 0 {
		fmt.Fprintf(&b, " Top song: %s.", st.TopSongs[0].Name)
	}
	if len(st.TopGenres) > 0 {
		fmt.Fprintf(&b, " Top genre: %s.", st.TopGenres[0].Name)
	}
	fmt.Fprintf(&b, " Listening personality: %s.", st.Personality.Name)
	return b.String()
}

func firstName(r []Ranked) string {
	if len(r) == 0 {
		return ""
	}
	return r[0].Name
}

var unsafeChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func safeName(s string) string {
	s = unsafeChars.ReplaceAllString(s, "_")
	s = strings.Trim(s, "._")
	if s == "" {
		s = "user"
	}
	return s
}

func commas(n int) string {
	s := strconv.Itoa(n)
	if n < 0 {
		return "-" + commas(-n)
	}
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func logInfo(s string)  { pdk.Log(pdk.LogInfo, s) }
func logWarn(s string)  { pdk.Log(pdk.LogWarn, s) }
func logError(s string) { pdk.Log(pdk.LogError, s) }
