package ui

import (
	"testing"
	"time"

	"github.com/devgianlu/go-librespot/daemon"

	"github.com/Nikkoro/spotui/internal/config"
	"github.com/Nikkoro/spotui/internal/lyrics"
	"github.com/Nikkoro/spotui/internal/viz"
)

func TestNpFocusAndSkip(t *testing.T) {
	cfg := config.Default()
	m := New(nil, &cfg, viz.NewTap())
	m.phase = phaseReady
	m.track = &daemon.ApiTrack{Name: "Song", Uri: "spotify:track:1", Duration: 200000}
	m.width, m.height = 200, 50
	m.npChoice = npPanels
	m.queue.list.setItems([]listItem{
		{kind: kindHeader, title: "Next up"},
		{title: "A", uri: "spotify:track:a"},
		{title: "B", uri: "spotify:track:b"},
	})
	press := func(k string) {
		_, next, _ := m.handleNpKey(k)
		m = next.(Model)
	}
	press("tab")
	if !m.npFocused(npFocusLyrics) {
		t.Fatal("tab should focus lyrics first")
	}
	press(">")
	if m.lyricsOffset != lyricsOffsetStep {
		t.Fatal("> should shift the lyrics")
	}
	press("tab")
	press("down")
	if !m.npFocused(npFocusNext) || m.npNextCursor != 1 {
		t.Fatalf("focus %d cursor %d", m.npFocus, m.npNextCursor)
	}
	press("esc")
	if m.npFocus != npFocusNone {
		t.Fatal("esc should leave the section")
	}
}

func TestClickLyricLineSeeks(t *testing.T) {
	cfg := config.Default()
	m := New(nil, &cfg, viz.NewTap())
	m.phase = phaseReady
	m.track = &daemon.ApiTrack{Name: "Song", Uri: "spotify:track:1", Duration: 200000}
	m.width, m.height = 140, 40
	m.npChoice = npSplit
	m.lyrics = &lyrics.Lyrics{Synced: []lyrics.Line{
		{At: 10 * time.Second, Text: "first invented line"},
		{At: 20 * time.Second, Text: "second invented line"},
		{At: 30 * time.Second, Text: "third invented line"},
	}}
	m.posMs, m.posAt, m.paused = 21000, time.Now(), true
	_ = m.View()

	h := m.hits
	row := -1
	for r, i := range h.lyrIdx {
		if i == 2 {
			row = r
		}
	}
	if row < 0 {
		t.Fatalf("third line not on screen: %v", h.lyrIdx)
	}
	cmd, ok := m.clickLyricsOrNext(h.lyrX0+5, h.lyrTop+row)
	if !ok || cmd == nil {
		t.Fatal("click on a lyric line did nothing")
	}
	if m.posMs != 30000 {
		t.Fatalf("seeked to %d ms, want 30000", m.posMs)
	}
	if _, ok := m.clickLyricsOrNext(0, h.lyrTop+row); ok {
		t.Fatal("a click left of the lyrics must not seek")
	}
}
