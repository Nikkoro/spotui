package ui

import (
	"math"
	"math/rand/v2"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Nikkoro/spotui/internal/viz"
)

// Beat effects on the lyric line being sung: it flashes with each beat
// (subtle), and at "party" sparkles fly off it and it shakes when the
// track drops. Beats come from the audio itself (viz.BeatDetector).

const (
	fxOff    = "off"
	fxSubtle = "subtle"
	fxParty  = "party"
)

var fxLevels = []string{fxOff, fxSubtle, fxParty}

func validFxLevel(s string) bool {
	for _, l := range fxLevels {
		if l == s {
			return true
		}
	}
	return false
}

type sparkle struct {
	side  int     // -1 left of the line, +1 right
	age   float64 // seconds
	speed float64 // columns a second
}

type lyricFx struct {
	beats  *viz.BeatDetector
	pulse  float64 // 1 on a beat, fading to 0
	shake  int     // frames of shaking left
	sparks []sparkle
}

const (
	sparkLife  = 0.7 // seconds
	sparkWidth = 10  // columns either side of the line they fly through
)

// fxLevel is the effects level in use.
func (m Model) fxLevel() string {
	if m.cfg == nil || !validFxLevel(m.cfg.Lyrics.Effects) {
		return fxSubtle
	}
	return m.cfg.Lyrics.Effects
}

// fxActive reports whether synced lyrics with effects are on screen.
func (m Model) fxActive() bool {
	if m.fxLevel() == fxOff || m.phase != phaseReady || m.track == nil || m.overlay != overlayNone ||
		m.lyrics == nil || len(m.lyrics.Synced) == 0 {
		return false
	}
	return m.page == pageLyrics || (m.page == pageNowPlaying && m.npLayout() != npClassic)
}

// stepFx ages the effects and starts new ones on a beat. samples is the
// analysed window (nil while paused); its newest dt of audio is fed in.
func (m *Model) stepFx(samples []float32, dt float64) {
	f := &m.fx
	f.pulse *= math.Exp(-dt * 7)
	if f.shake > 0 {
		f.shake--
	}
	kept := f.sparks[:0]
	for _, s := range f.sparks {
		s.age += dt
		if s.age < sparkLife {
			kept = append(kept, s)
		}
	}
	f.sparks = kept

	if samples == nil || !m.fxActive() {
		return
	}
	if f.beats == nil {
		f.beats = viz.NewBeatDetector()
	}
	n := clamp(int(dt*viz.SampleRate), 1, len(samples))
	b := f.beats.Feed(samples[len(samples)-n:], dt*1000)
	if !b.Hit {
		return
	}
	f.pulse = math.Max(f.pulse, math.Min(1, 0.55+(b.Strength-1)/3))
	if m.fxLevel() != fxParty {
		return
	}
	burst := 1
	if b.Strength > 2 {
		burst = 2
	}
	if b.Drop {
		burst = 4
		f.shake = 9
		f.pulse = 1
	}
	for i := 0; i < burst; i++ {
		for _, side := range []int{-1, 1} {
			f.sparks = append(f.sparks, sparkle{side: side, speed: 8 + rand.Float64()*10, age: rand.Float64() * 0.08})
		}
	}
	if len(f.sparks) > 24 {
		f.sparks = f.sparks[len(f.sparks)-24:]
	}
}

// fxRow centres the current line in w columns with its sparkles and shake.
func (m Model) fxRow(line string, w int) string {
	if m.fxLevel() == fxParty && len(m.fx.sparks) > 0 {
		left := []rune(strings.Repeat(" ", sparkWidth))
		right := []rune(strings.Repeat(" ", sparkWidth))
		for _, s := range m.fx.sparks {
			pos := int(1 + s.age*s.speed)
			if pos >= sparkWidth {
				continue
			}
			g := sparkGlyph(s.age / sparkLife)
			if s.side < 0 {
				left[sparkWidth-1-pos] = g
			} else {
				right[pos] = g
			}
		}
		style := m.theme.accentTxt
		line = style.Render(string(left)) + line + style.Render(string(right))
	}
	row := lipgloss.PlaceHorizontal(w, lipgloss.Center, line)
	if m.fxLevel() == fxParty && m.fx.shake > 0 {
		row = shift(row, shakeOffsets[m.fx.shake%len(shakeOffsets)], w)
	}
	return row
}

var shakeOffsets = []int{0, 1, -1, 2, -2, 1, -1, 2, -2, 1}

func sparkGlyph(life float64) rune {
	switch {
	case life < 0.25:
		return '✦'
	case life < 0.5:
		return '✧'
	case life < 0.75:
		return '⋆'
	}
	return '·'
}

// shift moves a w-wide row d columns right (left when negative).
func shift(row string, d, w int) string {
	switch {
	case d > 0:
		return ansi.Truncate(strings.Repeat(" ", d)+row, w, "")
	case d < 0:
		cut := ansi.TruncateLeft(row, -d, "")
		return cut + strings.Repeat(" ", max(w-ansi.StringWidth(cut), 0))
	}
	return row
}

// fxSungStyle colours the sung part of the line: the accent while a beat
// flashes, the title colour otherwise.
func (m Model) fxSungStyle() lipgloss.Style {
	if m.fxLevel() != fxOff && m.fx.pulse > 0.35 {
		return m.theme.accentTxt.Bold(true)
	}
	return m.theme.title.Bold(true)
}

// fxMarkers are the marks around the current line, bigger on a beat.
func (m Model) fxMarkers() (string, string) {
	if m.fxLevel() != fxOff && m.fx.pulse > 0.7 {
		return m.theme.accentTxt.Bold(true).Render("❯ "), m.theme.accentTxt.Bold(true).Render(" ❮")
	}
	return m.theme.accentTxt.Render("› "), m.theme.accentTxt.Render(" ‹")
}
