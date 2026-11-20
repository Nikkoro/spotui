package ui

import (
	"math"
	"strings"
	"testing"
	"time"
	"unicode"

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

func TestNightcoreZoomsAndStaysInside(t *testing.T) {
	m := fxModel(t, fxNightcore)
	dt := 1.0 / 30
	for i := 0; i < 40; i++ {
		m.stepFx(kick(0.02), dt)
	}
	m.stepFx(kick(0.9), dt)
	if m.fx.zoom < 0.5 {
		t.Fatalf("no zoom on a kick: %.2f", m.fx.zoom)
	}
	if len(m.fx.waves) == 0 {
		t.Fatal("no wave on a kick")
	}

	// The spread is one column at most.
	m.fx.waves = nil
	plain := ansi.Strip(m.nightcoreLine("an invented line", 0, 80))
	if !strings.Contains(plain, "i n v") || strings.Contains(plain, "i  n") {
		t.Fatalf("letters not spread by one column on the beat: %q", plain)
	}

	// However long the line and narrow the panel, the row stays w wide.
	long := strings.Repeat("a long invented line ", 6)
	for _, w := range []int{30, 60, 120} {
		row := m.fxRow(m.nightcoreLine(ansi.Truncate(long, w-4, "…"), 0, w), w)
		if got := lipgloss.Width(row); got > w {
			t.Fatalf("row %d wide in %d columns", got, w)
		}
	}

	// The zoom fades back within half a second.
	for i := 0; i < 15; i++ {
		m.stepFx(nil, dt)
	}
	if m.fx.zoom > 0.15 {
		t.Fatalf("zoom did not settle: %.2f", m.fx.zoom)
	}
	if plain := ansi.Strip(m.nightcoreLine("an invented line", 0, 80)); plain != "› an invented line ‹" {
		t.Fatalf("line not back to normal: %q", plain)
	}
}

// capitals returns the upper-case letters in s.
func capitals(s string) string {
	var out []rune
	for _, r := range s {
		if unicode.IsUpper(r) {
			out = append(out, r)
		}
	}
	return string(out)
}

func TestNightcoreWaveHopsLetterByLetter(t *testing.T) {
	m := fxModel(t, fxNightcore)
	const text = "an invented line"
	m.fx.waves = []hop{{dur: 0.35, bright: 1, dir: 1}}

	// One letter at a time is a capital, and the crest moves left to right
	// across the whole line within the wave's time.
	last, seen := -1, 0
	for step := 0; step < 40; step++ {
		plain := ansi.Strip(m.nightcoreLine(text, 0, 80))
		if caps := capitals(plain); len(caps) > 1 {
			t.Fatalf("more than one capital at once: %q", plain)
		} else if len(caps) == 1 {
			at := strings.IndexFunc(plain, unicode.IsUpper)
			if at < last {
				t.Fatalf("crest went backwards: %d after %d in %q", at, last, plain)
			}
			if at > last {
				seen++
			}
			last = at
		}
		if strings.ToLower(plain) != "› "+text+" ‹" {
			t.Fatalf("the text itself changed: %q", plain)
		}
		m.stepFx(nil, 0.35/40)
	}
	if seen < 10 {
		t.Fatalf("crest touched only %d letters", seen)
	}
	m.stepFx(nil, 0.01)
	if len(m.fx.waves) != 0 {
		t.Fatal("wave did not end")
	}
}

func TestNightcoreDropRunsFromTheMiddle(t *testing.T) {
	m := fxModel(t, fxNightcore)
	m.fx.waves = []hop{{dur: 0.25, bright: 1, dir: 1, middle: true}, {dur: 0.25, bright: 1, dir: -1, middle: true}}
	m.stepFx(nil, 0.06) // both crests on letters, not the space
	plain := ansi.Strip(m.nightcoreLine("an invented line", 0, 80))
	caps := capitals(plain)
	if len(caps) != 2 {
		t.Fatalf("want two crests on a drop, got %q in %q", caps, plain)
	}
	l := strings.IndexFunc(plain, unicode.IsUpper)
	r := strings.LastIndexFunc(plain, unicode.IsUpper)
	mid := strings.Index(strings.ToLower(plain), "invented") + 4
	if !(l < mid && r > mid) {
		t.Fatalf("crests not either side of the middle: %q", plain)
	}
}
