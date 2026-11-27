package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecorderCountsListeningTime(t *testing.T) {
	dir := t.TempDir()
	r := NewRecorder(dir, func(err error) { t.Error(err) })
	t0 := time.Date(2026, 11, 27, 20, 0, 0, 0, time.UTC)
	a := Now{URI: "spotify:track:a", Title: "Paper Boats", Artists: []string{"The Invented"}, DurationMs: 200_000, Playing: true}
	b := Now{URI: "spotify:track:b", Title: "Short One", DurationMs: 40_000, Playing: true}

	r.Observe(a, "niko", t0)
	r.Observe(a, "niko", t0.Add(20*time.Second))
	a.Playing = false
	r.Observe(a, "niko", t0.Add(25*time.Second)) // paused after 25 s
	r.Observe(a, "niko", t0.Add(5*time.Minute))  // still paused: doesn't count
	a.Playing = true
	r.Observe(a, "niko", t0.Add(6*time.Minute))
	r.Observe(b, "niko", t0.Add(6*time.Minute+10*time.Second)) // a: 25 + 10 s
	r.Observe(b, "niko", t0.Add(6*time.Minute+25*time.Second)) // b: 15 s of 40: under half
	c := Now{URI: "spotify:track:c", Title: "Skipped", DurationMs: 180_000, Playing: true}
	r.Observe(c, "niko", t0.Add(6*time.Minute+30*time.Second)) // b: 20 s = half: counts
	r.Observe(c, "niko", t0.Add(6*time.Minute+35*time.Second))
	r.Close(t0.Add(6*time.Minute + 40*time.Second)) // c: 10 s, skipped

	plays, err := Load(Path(dir, "niko"))
	if err != nil {
		t.Fatal(err)
	}
	if len(plays) != 2 || plays[0].URI != a.URI || plays[0].ListenedMs != 35_000 || plays[1].URI != b.URI || plays[1].ListenedMs != 20_000 {
		t.Fatalf("plays = %+v", plays)
	}
	if !plays[0].At.Equal(t0) {
		t.Fatalf("a started at %v", plays[0].At)
	}
}

func TestLoadSkipsBrokenLines(t *testing.T) {
	p := filepath.Join(t.TempDir(), "h.jsonl")
	os.WriteFile(p, []byte(`{"uri":"spotify:track:a","ms":1000}`+"\n"+`{"uri":"spotify:tra`), 0o644)
	plays, err := Load(p)
	if err != nil || len(plays) != 1 {
		t.Fatalf("Load = %v, %v", plays, err)
	}
	if plays, err := Load(filepath.Join(t.TempDir(), "none")); err != nil || plays != nil {
		t.Fatalf("missing file: %v, %v", plays, err)
	}
}

func TestSummarize(t *testing.T) {
	loc := time.FixedZone("CET", 3600)
	now := time.Date(2026, 11, 27, 18, 0, 0, 0, loc)
	at := func(daysAgo, hour int) time.Time {
		return time.Date(2026, 11, 27-daysAgo, hour, 0, 0, 0, loc)
	}
	min := int64(60_000)
	plays := []Play{
		{At: at(40, 10), ListenedMs: 3 * min, URI: "x", Title: "Old", Artists: []string{"Past"}},
		{At: at(10, 22), ListenedMs: 3 * min, URI: "a", Title: "Paper Boats", Artists: []string{"The Invented"}},
		{At: at(2, 22), ListenedMs: 3 * min, URI: "a", Title: "Paper Boats", Artists: []string{"The Invented"}},
		{At: at(1, 22), ListenedMs: 4 * min, URI: "b", Title: "Neon Hum", Artists: []string{"The Invented", "Guest"}},
		{At: at(0, 9), ListenedMs: 2 * min, URI: "a", Title: "Paper Boats", Artists: []string{"The Invented"}},
	}
	s := Summarize(plays, now, 5)
	if s.Today != (Totals{Ms: 2 * min, Plays: 1, Tracks: 1}) {
		t.Errorf("today = %+v", s.Today)
	}
	if s.Week != (Totals{Ms: 9 * min, Plays: 3, Tracks: 2}) {
		t.Errorf("week = %+v", s.Week)
	}
	if s.Month.Plays != 4 || s.All.Plays != 5 || s.All.Tracks != 3 {
		t.Errorf("month %+v all %+v", s.Month, s.All)
	}
	if s.Streak != 3 {
		t.Errorf("streak = %d", s.Streak)
	}
	if len(s.TopTracks) != 2 || s.TopTracks[0].Key != "a" || s.TopTracks[0].Plays != 3 {
		t.Errorf("top tracks = %+v", s.TopTracks)
	}
	if s.TopArtists[0].Key != "The Invented" || s.TopArtists[0].Plays != 4 || s.TopArtists[1].Key != "Guest" {
		t.Errorf("top artists = %+v", s.TopArtists)
	}
	if s.PeakHour != 22 {
		t.Errorf("peak hour = %d", s.PeakHour)
	}
	// Nothing yet today: the streak runs up to yesterday.
	if s := Summarize(plays[:4], now, 5); s.Streak != 2 {
		t.Errorf("streak without today = %d", s.Streak)
	}
}
