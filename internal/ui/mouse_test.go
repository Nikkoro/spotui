package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/devgianlu/go-librespot/daemon"

	"github.com/Nikkoro/spotui/internal/config"
)

func clickModel(t *testing.T) Model {
	cfg := config.Default()
	m := New(nil, &cfg, nil)
	m.width, m.height = 100, 30
	m.phase = phaseReady
	m.page = pageLibrary
	var items []listItem
	for _, n := range []string{"Alpha", "Bravo", "Charlie", "Delta", "Echo"} {
		items = append(items, listItem{kind: kindPlaylist, title: n, uri: "spotify:playlist:" + n})
	}
	m.lib.lists[libPlaylists].setItems(items)
	m.lib.loaded[libPlaylists] = true
	m.track = &daemon.ApiTrack{Name: "Song", Duration: 200000, Uri: "spotify:track:x"}
	return m
}

func lineOf(t *testing.T, screen, text string) int {
	for i, l := range strings.Split(stripANSI(screen), "\n") {
		if strings.Contains(l, text) {
			return i
		}
	}
	t.Fatalf("%q not on screen", text)
	return -1
}

func TestClickSelectsRow(t *testing.T) {
	m := clickModel(t)
	screen := m.View()
	y := lineOf(t, screen, "Delta")
	next, _ := m.handleMouse(tea.MouseMsg{X: 10, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	nm := next.(Model)
	got := nm.lib.lists[libPlaylists].selected()
	if got == nil || got.title != "Delta" {
		t.Fatalf("clicked Delta, selected %+v", got)
	}
}

func TestClickProgressSeeks(t *testing.T) {
	m := clickModel(t)
	_ = m.View()
	h := *m.hits
	if h.progressW == 0 {
		t.Fatal("progress bar not recorded")
	}
	screen := strings.Split(stripANSI(m.View()), "\n")
	if h.progressY >= len(screen) || !strings.Contains(screen[h.progressY], "━") {
		t.Fatalf("progress line %d is %q", h.progressY, screen[min(h.progressY, len(screen)-1)])
	}
}

// On Now Playing every layout puts the bar somewhere else; a click on it
// seeks all the same.
func TestClickProgressSeeksOnNowPlaying(t *testing.T) {
	for _, layout := range []string{npSplit, npPanels, npCompact, npClassic} {
		m := clickModel(t)
		m.page = pageNowPlaying
		m.paused = true
		m.setNpLayout(layout)
		screen := strings.Split(stripANSI(m.View()), "\n")
		h := *m.hits
		if h.progressW < 5 || h.progressY >= len(screen) {
			t.Fatalf("%s: progress bar not found (%+v)", layout, h)
		}
		if bar := []rune(screen[h.progressY]); h.progressX+h.progressW > len(bar) || !strings.Contains(string(bar[h.progressX:h.progressX+h.progressW]), "━") {
			t.Fatalf("%s: row %d at %d is not the bar: %q", layout, h.progressY, h.progressX, screen[h.progressY])
		}
		x := h.progressX + (h.progressW-1)/2
		next, _ := m.handleMouse(tea.MouseMsg{X: x, Y: h.progressY, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if pos := next.(Model).position(); pos < 80000 || pos > 120000 {
			t.Fatalf("%s: clicked the middle of the bar, position %d of 200000", layout, pos)
		}
	}
}
