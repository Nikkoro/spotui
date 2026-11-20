package ui

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/devgianlu/go-librespot/daemon"

	"github.com/Nikkoro/spotui/internal/config"
	"github.com/Nikkoro/spotui/internal/lyrics"
	"github.com/Nikkoro/spotui/internal/viz"
)

func fxModel(t *testing.T, level string) Model {
	cfg := config.Default()
	cfg.Lyrics.Effects = level
	m := New(nil, &cfg, viz.NewTap())
	m.phase = phaseReady
	m.page = pageLyrics
	m.width, m.height = 100, 30
	m.track = &daemon.ApiTrack{Name: "Song", Uri: "spotify:track:1", Duration: 200000}
	// Invented text.
	m.lyrics = &lyrics.Lyrics{Synced: []lyrics.Line{{At: 0, Text: "an invented line"}, {At: time.Minute, Text: "another one"}}}
	return m
}

// kick returns a frame-sized window holding a 60 Hz kick at amp.
func kick(amp float64) []float32 {
	out := make([]float32, 2048)
	for i := range out {
		out[i] = float32(amp * math.Sin(2*math.Pi*60*float64(i)/viz.SampleRate))
	}
	return out
}

func TestFxBeatsFlashAndSparkle(t *testing.T) {
	m := fxModel(t, fxParty)
	dt := 1.0 / 30
	// A steady quiet bed, then a kick.
	for i := 0; i < 40; i++ {
		m.stepFx(kick(0.02), dt)
	}
	m.stepFx(kick(0.8), dt)
	if m.fx.pulse < 0.5 || len(m.fx.sparks) == 0 {
		t.Fatalf("no flash or sparkles on a kick: pulse %.2f, %d sparks", m.fx.pulse, len(m.fx.sparks))
	}
	m.fx.shake = 3
	m.fx.sparks[0].age = 0.05
	row := m.fxRow(m.theme.title.Render("an invented line"), 80)
	if w := lipgloss.Width(row); w != 80 {
		t.Fatalf("row is %d wide, want 80", w)
	}
	if !strings.ContainsAny(ansi.Strip(row), "✦✧⋆·") {
		t.Fatalf("no sparkle drawn: %q", ansi.Strip(row))
	}
}

func TestFxOffDoesNothing(t *testing.T) {
	m := fxModel(t, fxOff)
	for i := 0; i < 40; i++ {
		m.stepFx(kick(0.02), 1.0/30)
	}
	m.stepFx(kick(0.8), 1.0/30)
	if m.fx.pulse != 0 || len(m.fx.sparks) != 0 {
		t.Fatal("effects off but something moved")
	}
}
