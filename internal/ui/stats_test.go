package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/devgianlu/go-librespot/daemon"

	"github.com/Nikkoro/spotui/internal/history"
)

func TestStatsRecordAndShow(t *testing.T) {
	m := remoteModel(t)
	m.width, m.height = 120, 36
	m.username = "niko"
	rec := history.NewRecorder(m.cfg.ConfigDir, func(err error) { t.Error(err) })
	m = m.WithHistory(rec)

	// Played through the UI's own state: 40 s of a track, then another.
	m.track = &daemon.ApiTrack{Uri: "spotify:track:abc", Name: "Paper Lanterns", ArtistNames: []string{"The Invented"}, AlbumName: "Night Shift", Duration: 200000}
	m.stopped, m.paused = false, false
	t0 := time.Now()
	n := func(m Model) history.Now {
		st := m.remoteState()
		return history.Now{URI: st.URI, Title: st.Title, Artists: st.Artists, Album: st.Album, DurationMs: st.DurationMs, Playing: st.Status == "playing"}
	}
	rec.Observe(n(m), m.username, t0)
	m.track = &daemon.ApiTrack{Uri: "spotify:track:def", Name: "Next", Duration: 100000}
	rec.Observe(n(m), m.username, t0.Add(40*time.Second))
	m.recordHistory() // the real path: doesn't panic, takes the new track

	msg := m.loadStats()().(libLoadedMsg)
	if msg.err != nil || msg.stats == nil || msg.stats.All.Plays != 1 || len(msg.items) != 1 || msg.items[0].title != "Paper Lanterns" {
		t.Fatalf("loaded %+v (err %v)", msg, msg.err)
	}
	nm, _ := m.updatePages(msg)
	m = nm.(Model)
	m.page, m.lib.tab = pageLibrary, libStats
	out := m.View()
	for _, want := range []string{"Stats", "7 days", "Streak", "1 day", "Paper Lanterns", "1× · 40 s", "The Invented"} {
		if !strings.Contains(out, want) {
			t.Errorf("Stats page lacks %q", want)
		}
	}

	// History off: nothing new is recorded, and the page says so.
	m.cfg.Stats.History = false
	m.recordHistory()
	m.track = &daemon.ApiTrack{Uri: "spotify:track:ghi", Name: "Unheard", Duration: 100000}
	m.recordHistory()
	if s := m.viewStats(&m.lib.lists[libStats], 100, 20); !strings.Contains(s, "history off") {
		t.Errorf("history off not shown:\n%s", s)
	}
	if m.libCachePath(libStats) != "" {
		t.Error("Stats must not be cached")
	}
}

func TestFmtListened(t *testing.T) {
	for ms, want := range map[int64]string{12_000: "12 s", 42 * 60_000: "42m", (3*60 + 5) * 60_000: "3h 05m"} {
		if got := fmtListened(ms); got != want {
			t.Errorf("fmtListened(%d) = %q, want %q", ms, got, want)
		}
	}
}

func TestLibraryTabsFitEightyColumns(t *testing.T) {
	m := remoteModel(t)
	m.width = 80
	if w := lipgloss.Width(m.tabBar(libTabs, libStats, nil)); w > 78 {
		t.Fatalf("library tabs are %d wide at 80 columns", w)
	}
}
