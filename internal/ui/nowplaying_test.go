package ui

import (
	"testing"

	"github.com/devgianlu/go-librespot/daemon"

	"github.com/Nikkoro/spotui/internal/config"
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
