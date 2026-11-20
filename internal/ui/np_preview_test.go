//go:build preview

package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/Nikkoro/spotui/internal/lyrics"
	"github.com/Nikkoro/spotui/internal/termimg"
)

// TestPreviewNpLayouts writes every Now Playing layout at a few sizes.
func TestPreviewNpLayouts(t *testing.T) {
	dir := os.Getenv("PREVIEW_DIR")
	if dir == "" {
		t.Skip("PREVIEW_DIR not set")
	}
	lipgloss.SetColorProfile(termenv.TrueColor)
	lipgloss.SetHasDarkBackground(true)
	_ = os.MkdirAll(dir, 0o755)

	m := sampleModel(t)
	m.lyrics = &lyrics.Lyrics{Source: "LRCLIB", Synced: lyrics.ParseLRC(
		"[01:20.00]I left the lights on by the water\n[01:26.00]to find my way back after dark\n" +
			"[01:31.00]the radio hums an old address\n[01:35.00]Neon harbour, hold me steady\n" +
			"[01:42.00]every wave is calling twice\n[01:48.00]I keep the night inside my pocket\n" +
			"[01:54.00]and spend it slowly, line by line\n[02:00.00]\n[02:06.00]Neon harbour, hold me steady")}
	m.queue.list.setItems([]listItem{
		{kind: kindHeader, title: "Next up"},
		{title: "Paper Lanterns", subtitle: "Odessa Vale", uri: "a"},
		{title: "Low Tide Radio", subtitle: "Lumen Drift", uri: "b"},
		{title: "Slow Static", subtitle: "Mirefield", uri: "c"},
	})
	variants := []struct {
		name   string
		images bool
	}{{"blocks", false}, {"sixel", true}}
	for _, v := range variants {
		m.imageMode = v.images
		m.caps = termimg.Caps{Protocol: termimg.Sixel, CellW: 10, CellH: 20}
		m.img = termimg.NewWriter(os.Stdout, termimg.Sixel)
		m.frame = &imgFrame{}
		for _, sz := range [][2]int{{200, 50}, {130, 40}, {100, 30}, {80, 24}} {
			for _, l := range []string{npAuto, npSplit, npPanels, npCompact} {
				m.width, m.height = sz[0], sz[1]
				m.npChoice = l
				out := m.View()
				name := filepath.Join(dir, v.name+"-"+l+"-"+itoa(sz[0])+"x"+itoa(sz[1])+".ans")
				if err := os.WriteFile(name, []byte(out), 0o644); err != nil {
					t.Fatal(err)
				}
				if h := lipgloss.Height(out); h > sz[1] {
					t.Errorf("%s at %v: %d rows, more than the terminal", l, sz, h)
				}
				if w := lipgloss.Width(out); w > sz[0] {
					t.Errorf("%s at %v: %d columns, wider than the terminal", l, sz, w)
				}
				if v.images && m.frame.tmpl == nil {
					t.Errorf("%s at %v: no image placed", l, sz)
				}
			}
		}
	}
}
