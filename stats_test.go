package main

import (
	"strings"
	"testing"
	"time"
)

func ts(loc *time.Location, y int, m time.Month, d, h, min int) int64 {
	return time.Date(y, m, d, h, min, 0, 0, loc).Unix()
}

func fixture() map[string]Track {
	return map[string]Track{
		"t1": {ID: "t1", Title: "Neon Rivers", Artist: "Aurora Vale", Artists: []ArtistRef{{"a1", "Aurora Vale"}}, AlbumID: "al1", Album: "Tidal", Duration: 240, Year: 2026, Genres: []string{"Synthpop"}},
		"t2": {ID: "t2", Title: "Quiet Engines", Artist: "Aurora Vale feat. Oslo Lights", Artists: []ArtistRef{{"a1", "Aurora Vale"}, {"a2", "Oslo Lights"}}, AlbumID: "al1", Album: "Tidal", Duration: 180, Year: 2025, Genres: []string{"Synthpop", "Electronic"}},
		"t3": {ID: "t3", Title: "Paper Moon", Artist: "The Lanterns", Artists: []ArtistRef{{"a3", "The Lanterns"}}, AlbumID: "al2", Album: "Harbour", Duration: 300, Year: 1994, Genres: []string{"Indie Rock"}},
	}
}

func TestComputeBasics(t *testing.T) {
	loc, _ := time.LoadLocation("Australia/Melbourne")
	var plays []Play
	// t1 played 5x on 3 March at 23:xx, t2 2x on 4 March, t3 once on 5 March, plus one outside the year
	for i := 0; i < 5; i++ {
		plays = append(plays, Play{"t1", ts(loc, 2026, 3, 3, 23, i*5)})
	}
	plays = append(plays, Play{"t2", ts(loc, 2026, 3, 4, 10, 0)}, Play{"t2", ts(loc, 2026, 3, 4, 11, 0)})
	plays = append(plays, Play{"t3", ts(loc, 2026, 3, 5, 9, 0)})
	plays = append(plays, Play{"t3", ts(loc, 2025, 12, 31, 23, 0)}) // previous year: ignored
	plays = append(plays, Play{"missing", ts(loc, 2026, 4, 1, 9, 0)}) // deleted track: ignored
	// first play of the year comes before the others but was added last
	plays = append(plays, Play{"t3", ts(loc, 2026, 1, 1, 0, 5)})

	st := Compute(plays, fixture(), loc, 2026, 5, 2)

	if st.TotalPlays != 9 {
		t.Fatalf("TotalPlays = %d, want 9", st.TotalPlays)
	}
	wantMin := (5*240 + 2*180 + 2*300) / 60
	if st.TotalMinutes != wantMin {
		t.Errorf("TotalMinutes = %d, want %d", st.TotalMinutes, wantMin)
	}
	if st.TopSongs[0].ID != "t1" || st.TopSongs[0].Plays != 5 {
		t.Errorf("top song = %+v", st.TopSongs[0])
	}
	// Aurora Vale credited on t1 (5) and t2 (2) = 7
	if st.TopArtists[0].Name != "Aurora Vale" || st.TopArtists[0].Plays != 7 {
		t.Errorf("top artist = %+v", st.TopArtists[0])
	}
	if st.TopArtists[0].CoverArt != "ar-a1" || st.TopArtists[0].FallbackArt != "al1" {
		t.Errorf("artist art = %q fallback %q", st.TopArtists[0].CoverArt, st.TopArtists[0].FallbackArt)
	}
	if st.UniqueArtists != 3 || st.UniqueTracks != 3 || st.UniqueAlbums != 2 {
		t.Errorf("uniques = %d/%d/%d", st.UniqueArtists, st.UniqueTracks, st.UniqueAlbums)
	}
	if st.TopGenres[0].Name != "Synthpop" || st.TopGenres[0].Plays != 7 {
		t.Errorf("top genre = %+v", st.TopGenres[0])
	}
	if st.PeakHour != 23 {
		t.Errorf("PeakHour = %d, want 23 (local time)", st.PeakHour)
	}
	if st.TopMonth != 3 {
		t.Errorf("TopMonth = %d", st.TopMonth)
	}
	if st.LongestStreak != 3 || st.StreakStart != "2026-03-03" || st.StreakEnd != "2026-03-05" {
		t.Errorf("streak = %d %s..%s", st.LongestStreak, st.StreakStart, st.StreakEnd)
	}
	if st.BiggestDay.Date != "2026-03-03" || st.BiggestDay.Plays != 5 {
		t.Errorf("biggest day = %+v", st.BiggestDay)
	}
	if st.OnRepeat == nil || st.OnRepeat.Song.ID != "t1" || st.OnRepeat.Plays != 5 {
		t.Errorf("on repeat = %+v", st.OnRepeat)
	}
	if st.FirstSong == nil || st.FirstSong.Song.ID != "t3" || !strings.Contains(st.FirstSong.When, "1 January") {
		t.Errorf("first song = %+v", st.FirstSong)
	}
	if len(st.PlaylistTrackIDs) != 2 || st.PlaylistTrackIDs[0] != "t1" || st.PlaylistTrackIDs[1] != "t3" {
		t.Errorf("playlist = %v", st.PlaylistTrackIDs)
	}
	if st.TopDecade != "2020s" {
		t.Errorf("TopDecade = %q", st.TopDecade)
	}
	if st.Personality.Name == "" || len(st.Personality.Code) != 4 {
		t.Errorf("personality = %+v", st.Personality)
	}
	if st.Personality.Code != "LNRN" { // loyal, new, repeat (3 unique / 9 plays), night
		t.Errorf("personality code = %q", st.Personality.Code)
	}
}

func TestComputeEmpty(t *testing.T) {
	st := Compute(nil, fixture(), time.UTC, 2026, 5, 50)
	if st.TotalPlays != 0 || len(st.TopSongs) != 0 {
		t.Fatalf("expected empty stats, got %+v", st)
	}
}

func TestArtistWithoutID(t *testing.T) {
	tracks := map[string]Track{"x": {ID: "x", Title: "Song", Artist: "No Id Band", Duration: 200}}
	st := Compute([]Play{{"x", ts(time.UTC, 2026, 6, 1, 12, 0)}}, tracks, time.UTC, 2026, 5, 5)
	if st.TopArtists[0].Name != "No Id Band" || st.TopArtists[0].ID != "" || st.TopArtists[0].CoverArt != "" {
		t.Errorf("artist = %+v", st.TopArtists[0])
	}
}

func TestInjectEscapesScript(t *testing.T) {
	out, err := inject("<script>const D = "+dataPlaceholder+";</script>", map[string]string{"x": "</script><script>alert(1)</script>"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(out), "</script>") != 1 {
		t.Errorf("payload escaped the script tag: %s", out)
	}
}

func TestHelpers(t *testing.T) {
	if commas(1234567) != "1,234,567" || commas(12) != "12" || commas(-1000) != "-1,000" {
		t.Error("commas")
	}
	if safeName("../Jo hn!") != "Jo_hn" {
		t.Errorf("safeName = %q", safeName("../Jo hn!"))
	}
	if itoa(1990) != "1990" {
		t.Error("itoa")
	}
}
