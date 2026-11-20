package ui

import (
	"math"
	"math/rand/v2"
	"strings"
	"time"

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
	// fxNightcore adds to party: the line zooms out on each beat (letters
	// spread apart and pull back), flashes to capitals on the hardest ones,
	// a light sweeps through it letter by letter, colours flow in the
	// track's tempo, and the lines around it pulse.
	fxNightcore = "nightcore"
)

var fxLevels = []string{fxOff, fxSubtle, fxParty, fxNightcore}

// fxAtLeast reports whether the level in use includes level's effects.
func (m Model) fxAtLeast(level string) bool {
	rank := func(l string) int {
		for i, x := range fxLevels {
			if x == l {
				return i
			}
		}
		return 0
	}
	return rank(m.fxLevel()) >= rank(level)
}

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

	// nightcore
	zoom      float64 // 1 on a beat: letters spread, pulling back to 0
	caps      float64 // seconds of capitals left
	wave      float64 // the light's sweep through the line, 0…1 and on
	hue       float64 // colour phase, turning in the track's tempo
	sinceBeat float64 // ms since the last beat
	beatMs    float64 // the beat interval, smoothed (0: unknown yet)
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
	f.zoom *= math.Exp(-dt * 9)
	f.caps = math.Max(0, f.caps-dt)
	f.wave += dt / 0.35
	f.sinceBeat += dt * 1000
	beat := 500.0
	if f.beatMs > 0 {
		beat = f.beatMs
	}
	f.hue = math.Mod(f.hue+dt*1000/beat*0.25, 1)
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
	if since := f.sinceBeat; since > 250 && since < 1500 {
		if f.beatMs == 0 {
			f.beatMs = since
		} else {
			f.beatMs = f.beatMs*0.8 + since*0.2
		}
	}
	f.sinceBeat = 0
	if !m.fxAtLeast(fxParty) {
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
	if m.fxAtLeast(fxNightcore) {
		f.zoom = math.Max(f.zoom, math.Min(1, 0.6+(b.Strength-1)/2))
		f.wave = 0
		if b.Strength > 2.2 || b.Drop {
			f.caps = 0.18
		}
		burst *= 2
		if b.Drop {
			f.shake = 14
		}
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
	if m.fxAtLeast(fxParty) && len(m.fx.sparks) > 0 && ansi.StringWidth(line)+2*sparkWidth <= w {
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
	if m.fxAtLeast(fxParty) && m.fx.shake > 0 {
		d := shakeOffsets[m.fx.shake%len(shakeOffsets)]
		if m.fxAtLeast(fxNightcore) {
			d *= 2
		}
		row = shift(row, d, w)
	}
	return ansi.Truncate(row, w, "")
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

// --- nightcore

// nightcoreLine draws the line being sung (line i, text) for at most w
// columns: letters spread apart on the beat, capitals on the hardest ones,
// colours flowing in tempo over the sung part and a light sweeping through.
func (m Model) nightcoreLine(text string, i, w int) string {
	f := m.fx
	if f.caps > 0 {
		text = strings.ToUpper(text)
	}
	runes := []rune(text)
	n := len(runes)
	if n == 0 {
		return ""
	}

	// Sung so far, as karaokeLine works it out.
	sung := n
	lines := m.lyrics.Synced
	start := lines[i].At
	end := start + 4*time.Second
	if i+1 < len(lines) {
		end = lines[i+1].At
	}
	if span := end - start; span > 0 {
		p := math.Max(0, math.Min(1, float64(m.lyricsPos()-start)/float64(span)))
		sung = int(math.Round(p * float64(n)))
	}

	// Spread letters as far as the zoom asks and the width allows.
	avail := w - 4
	if m.fxAtLeast(fxParty) {
		avail -= 2 * sparkWidth
	}
	gap := 0
	if f.zoom > 0.12 && n > 1 {
		gap = min(int(math.Round(f.zoom*2)), max((avail-n)/(n-1), 0))
	}

	base, ok := parseRGB(m.theme.accentHex)
	if !ok {
		base = rgb{29, 185, 84}
	}
	h, sat, l := base.hsl()
	sat = math.Max(sat, 0.55)
	l = math.Min(math.Max(l, 0.5), 0.68)
	text0, _ := parseRGB(string(colSubtle))
	wavePos := f.wave * float64(n+4)

	var sb strings.Builder
	before, after := m.fxMarkers()
	sb.WriteString(before)
	for k, r := range runes {
		var c rgb
		if k < sung {
			hue := h + 0.12*math.Sin(2*math.Pi*(f.hue-float64(k)/24))
			c = hslToRGB(hue, sat, l)
		} else {
			c = text0
			if c == (rgb{}) {
				c = rgb{200, 200, 205}
			}
		}
		if d := math.Abs(float64(k) - wavePos); d < 2.5 {
			c = c.mix(rgb{255, 255, 255}, 0.75*(1-d/2.5))
		}
		style := lipgloss.NewStyle().Foreground(c.color())
		if k < sung {
			style = style.Bold(true)
		}
		sb.WriteString(style.Render(string(r)))
		if gap > 0 && k < n-1 {
			sb.WriteString(strings.Repeat(" ", gap))
		}
	}
	sb.WriteString(after)
	return sb.String()
}

// hsl returns hue (0…1), saturation and lightness.
func (c rgb) hsl() (float64, float64, float64) {
	r, g, b := c.r/255, c.g/255, c.b/255
	mx, mn := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	l := (mx + mn) / 2
	if mx == mn {
		return 0, 0, l
	}
	d := mx - mn
	s := d / (1 - math.Abs(2*l-1))
	var h float64
	switch mx {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h /= 6
	if h < 0 {
		h++
	}
	return h, s, l
}

func hslToRGB(h, s, l float64) rgb {
	h = math.Mod(h, 1)
	if h < 0 {
		h++
	}
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h*6, 2)-1))
	m := l - c/2
	var r, g, b float64
	switch int(h * 6) {
	case 0:
		r, g, b = c, x, 0
	case 1:
		r, g, b = x, c, 0
	case 2:
		r, g, b = 0, c, x
	case 3:
		r, g, b = 0, x, c
	case 4:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return rgb{(r + m) * 255, (g + m) * 255, (b + m) * 255}
}
