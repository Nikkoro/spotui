package ui

import (
	"math"
	"math/rand/v2"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Nikkoro/spotui/internal/viz"
)

// Beat effects on the lyric line being sung, each with its own switch
// (fxSet): a flash on each beat, flowing colours, a letter wave, a letter
// spread, sparkles, a shake when the track drops. The presets (subtle,
// party, nightcore) are ready-made sets; "custom" reads [lyrics.fx]. Beats come from the audio itself (viz.BeatDetector).

const (
	fxOff    = "off"
	fxSubtle = "subtle"
	fxParty  = "party"
	// fxNightcore adds to party: on each beat a wave runs through the line
	// letter by letter (the letter at its crest hops up: capital, bold,
	// brightest, with a soft tail behind it), the hardest beats spread the
	// letters a little, and colours flow in the track's tempo.
	fxNightcore = "nightcore"
	// fxCustom takes each effect's switch from [lyrics.fx].
	fxCustom = "custom"
)

// fxLevels are what effects may be set to; fxPresets the ready-made sets.
var (
	fxPresets = []string{fxOff, fxSubtle, fxParty, fxNightcore}
	fxLevels  = append(append([]string{}, fxPresets...), fxCustom)
)

// fxSet is which effects are on.
type fxSet struct {
	flash, colors, wave, spread, sparkles, shake bool
}

func (s fxSet) any() bool {
	return s.flash || s.colors || s.wave || s.spread || s.sparkles || s.shake
}

// letters reports whether the line is drawn letter by letter
// (nightcoreLine): for flowing colours, the wave or the spread.
func (s fxSet) letters() bool { return s.colors || s.wave || s.spread }

// strong: with the wave or the spread, sparkles burst twice as thick and
// drops shake longer and wider, as nightcore had them.
func (s fxSet) strong() bool { return s.wave || s.spread }

func presetSet(level string) fxSet {
	switch level {
	case fxSubtle:
		return fxSet{flash: true}
	case fxParty:
		return fxSet{flash: true, sparkles: true, shake: true}
	case fxNightcore:
		return fxSet{flash: true, colors: true, wave: true, spread: true, sparkles: true, shake: true}
	}
	return fxSet{}
}

// fxOn is the set of effects in use: a preset's, or [lyrics.fx]'s.
func (m Model) fxOn() fxSet {
	if m.fxLevel() == fxCustom {
		f := m.cfg.Lyrics.FX
		return fxSet{flash: f.Flash, colors: f.Colors, wave: f.Wave, spread: f.Spread, sparkles: f.Sparkles, shake: f.Shake}
	}
	return presetSet(m.fxLevel())
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
	beats  beatSource // what every effect follows (beats.go)
	pulse  float64    // 1 on a beat, fading to 0
	shake  int        // frames of shaking left
	sparks []sparkle

	// nightcore
	zoom      float64 // 1 on a hard beat: letters spread, pulling back to 0
	waves     []hop   // waves running through the line
	hue       float64 // colour phase, turning in the track's tempo
	sinceBeat float64 // ms since the last beat
	beatMs    float64 // the beat interval, smoothed (0: unknown yet)

	// for the Windows Terminal shader (wtshader.go), whatever the level
	shader shaderFx
}

// hop is a wave running through the line being sung: the letter at its
// crest hops up, the one or two behind it glow.
type hop struct {
	age    float64 // seconds
	dur    float64 // seconds to cross the line (or half of it, from the middle)
	bright float64 // 0…1, how far the crest goes towards white
	dir    int     // +1 left to right, -1 right to left
	middle bool    // starts from the middle of the line instead of an edge
}

const (
	sparkLife  = 0.7 // seconds
	sparkWidth = 10  // columns either side of the line they fly through

	hopTail = 2.5 // letters of tail behind a wave's crest
	maxHops = 6
)

// at is the crest's position in a line of n letters, -1…n+tail and on.
func (h hop) at(n int) float64 {
	p := h.age / h.dur
	if h.middle {
		c := float64(n-1) / 2
		return c + float64(h.dir)*p*(float64(n)/2+hopTail)
	}
	pos := -1 + p*(float64(n)+hopTail+1)
	if h.dir < 0 {
		pos = float64(n-1) - pos
	}
	return pos
}

// fxLevel is the effects level in use.
func (m Model) fxLevel() string {
	if m.cfg == nil || !validFxLevel(m.cfg.Lyrics.Effects) {
		return fxSubtle
	}
	return m.cfg.Lyrics.Effects
}

// fxActive reports whether synced lyrics with effects are on screen.
func (m Model) fxActive() bool {
	return m.fxOn().any() && m.syncedOnScreen()
}

// syncedOnScreen reports whether synced lyrics are on screen.
func (m Model) syncedOnScreen() bool {
	return m.lyricsPanelOnScreen() && m.lyrics != nil && len(m.lyrics.Synced) > 0
}

// lyricsPanelOnScreen reports whether the lyrics page or panel is on
// screen, whatever it shows: lyrics, "Instrumental", none found.
func (m Model) lyricsPanelOnScreen() bool {
	if m.phase != phaseReady || m.track == nil || m.overlay != overlayNone {
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
	waves := f.waves[:0]
	for _, h := range f.waves {
		h.age += dt
		if h.age < h.dur {
			waves = append(waves, h)
		}
	}
	f.waves = waves
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

	shader := m.shaderActive()
	if shader && samples == nil && f.shader.spec != nil {
		f.shader.bands, _ = f.shader.spec.Update(nil, dt) // paused: let the spectrum fall
	}
	if samples == nil || !(m.fxActive() || shader) {
		return
	}
	n := clamp(int(dt*viz.SampleRate), 1, len(samples))
	raw, b, t := f.beats.feed(samples[len(samples)-n:], dt)
	if shader {
		f.shader.feed(samples, samples[len(samples)-n:], raw, b, t, f.beats.onGrid, dt)
		if m.shaderIsAuto() && m.track != nil {
			f.shader.stepAuto(dt, b.Hit, raw.Drop, m.track.Uri, autoChoice(m.cfg.Lyrics.WTShaderAuto), m.eng)
		}
	}
	if f.beats.onGrid {
		f.beatMs = 60000 / t.BPM
	}
	if !b.Hit || !m.fxActive() {
		return
	}
	f.pulse = math.Max(f.pulse, math.Min(1, 0.55+(b.Strength-1)/3))
	if since := f.sinceBeat; !f.beats.onGrid && since > 250 && since < 1500 {
		if f.beatMs == 0 {
			f.beatMs = since
		} else {
			f.beatMs = f.beatMs*0.8 + since*0.2
		}
	}
	f.sinceBeat = 0
	on := m.fxOn()
	burst := 1
	if b.Strength > 2 {
		burst = 2
	}
	if b.Drop {
		burst = 4
		f.pulse = 1
		if on.shake {
			f.shake = 9
			if on.strong() {
				f.shake = 14
			}
		}
	}
	if on.spread {
		// Only the harder beats spread the letters (see nightcoreLine).
		f.zoom = math.Max(f.zoom, math.Min(1, 0.3+(b.Strength-1)/2))
	}
	if on.wave {
		// Harder beats run faster and brighter: 0.4 s across down to 0.28.
		k := math.Max(0, math.Min(1, (b.Strength-1)/2))
		h := hop{dur: 0.4 - 0.12*k, bright: 0.55 + 0.45*k, dir: 1}
		if b.Drop {
			// Out from the middle both ways at once.
			h = hop{dur: 0.25, bright: 1, dir: 1, middle: true}
			f.waves = append(f.waves, h)
			h.dir = -1
		}
		f.waves = append(f.waves, h)
		if len(f.waves) > maxHops {
			f.waves = f.waves[len(f.waves)-maxHops:]
		}
	}
	if on.strong() {
		burst *= 2
	}
	if !on.sparkles {
		return
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
	on := m.fxOn()
	if on.sparkles && len(m.fx.sparks) > 0 && ansi.StringWidth(line)+2*sparkWidth <= w {
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
	if on.shake && m.fx.shake > 0 {
		d := shakeOffsets[m.fx.shake%len(shakeOffsets)]
		if on.strong() {
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
	if m.fxOn().flash && m.fx.pulse > 0.35 {
		return m.theme.accentTxt.Bold(true)
	}
	return m.theme.title.Bold(true)
}

// fxMarkers are the marks around the current line, bigger on a beat.
func (m Model) fxMarkers() (string, string) {
	if m.fxOn().flash && m.fx.pulse > 0.7 {
		return m.theme.accentTxt.Bold(true).Render("❯ "), m.theme.accentTxt.Bold(true).Render(" ❮")
	}
	return m.theme.accentTxt.Render("› "), m.theme.accentTxt.Render(" ‹")
}

// --- nightcore

// nightcoreLine draws the line being sung (line i, text) for at most w
// columns: colours flowing in tempo over the sung part, waves running
// through it letter by letter on the beat, and on the hardest beats the
// letters spread a little.
func (m Model) nightcoreLine(text string, i, w int) string {
	f := m.fx
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

	on := m.fxOn()
	// Spread letters as far as the zoom asks and the width allows.
	avail := w - 4
	if on.sparkles {
		avail -= 2 * sparkWidth
	}
	// One column between letters at most, and only for a moment on the
	// harder beats: a wider spread moves the words too far to read.
	gap := 0
	if on.spread && f.zoom > 0.45 && n > 1 && avail >= 2*n-1 {
		gap = 1
	}

	base, ok := parseRGB(m.theme.accentHex)
	if !ok {
		base = rgb{29, 185, 84}
	}
	h, sat, l := base.hsl()
	sat = math.Max(sat, 0.55)
	l = math.Min(math.Max(l, 0.5), 0.68)
	text0, _ := parseRGB(string(colSubtle))
	plain := hslToRGB(h, sat, l) // the sung part without flowing colours

	var sb strings.Builder
	before, after := m.fxMarkers()
	sb.WriteString(before)
	for k, r := range runes {
		var c rgb
		if k < sung {
			c = plain
			if on.colors {
				hue := h + 0.12*math.Sin(2*math.Pi*(f.hue-float64(k)/24))
				c = hslToRGB(hue, sat, l)
			}
		} else {
			c = text0
			if c == (rgb{}) {
				c = rgb{200, 200, 205}
			}
		}
		var crest bool
		var glow float64
		if on.wave {
			crest, glow = f.hopAt(k, n)
		}
		bold := k < sung
		if glow > 0 {
			c = c.mix(rgb{255, 255, 255}, glow)
		}
		if crest {
			bold = true
			if u := unicode.ToUpper(r); ansi.StringWidth(string(u)) == ansi.StringWidth(string(r)) {
				r = u
			}
		}
		style := lipgloss.NewStyle().Foreground(c.color())
		if bold {
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

// hopAt reports whether letter k of n is at a wave's crest, and how far
// towards white the waves light it (0: not at all).
func (f lyricFx) hopAt(k, n int) (crest bool, glow float64) {
	for _, h := range f.waves {
		// Distance behind the crest, in the wave's direction.
		d := (h.at(n) - float64(k)) * float64(h.dir)
		switch {
		case d > -0.5 && d <= 0.5:
			crest = true
			glow = math.Max(glow, 0.85*h.bright)
		case d > 0.5 && d < hopTail:
			glow = math.Max(glow, 0.45*h.bright*(1-(d-0.5)/(hopTail-0.5)))
		}
	}
	return crest, glow
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
