package ui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/Nikkoro/spotui/internal/viz"
)

// A Windows Terminal pixel shader can only see the rendered window, so the
// music reaches it through colour: the first cells of the top row (the
// app's padding, blank anyway) are painted with background colours that
// carry the data, and shaders/spotui.hlsl reads them back, hides them and
// draws behind the text. The cells are written as raw 24-bit escapes so no
// colour profile rounds them.
//
//	cell 0    signature, shaderSig
//	cell 1    R hit: how hard the last beat was, with the bar's first beat
//	          the hardest (it stays until the next beat; the shader fades
//	          it by the beat phase), G bass, B loudness
//	cell 2    the accent colour, for the palette
//	cell 3    R colour phase in the track's tempo, G drop: 1 for half a
//	          second after one (debug style: the frame count), B energy:
//	          how loud this part of the track is against the last half
//	          minute
//	cell 4    R style (index in shaderStyles), G B travel (16 bits, 0…1
//	          wrapping): how far the music (bass, beats, drops) has pushed
//	          the effect on top of the steady pace the shader keeps itself
//	cell 5…8  the spectrum, shaderBands bands low to high, three a cell
//	cell 9    R beat phase: 0 on a beat, 1 at the next one by the tempo,
//	          G which beat of four it is (0, 85, 170, 255), B snare: a hit
//	          in the high bands (snare, claps, hats), fading fast
//	cell 10   the first plasma's values, kept as they were because that
//	          plasma followed the music best: R kick, G bass against its
//	          last second, B loudness, each jumping up and fading, at 30
//	          frames a second
//	cell 11   signature again, so the shader knows where the strip ends
//
// The shader keeps no state between frames, so anything that should move
// at a speed the music sets (travel) is added up here and sent as a
// position. Only the music's share is: the steady part is timed in the
// shader, smooth at the terminal's frame rate rather than spotui's 30.
const (
	shaderBands = 12
	shaderCells = 8 + shaderBands/3
)

var shaderSig = [3]int{13, 37, 241}

// shaderStyles are the effects in shaders/spotui.hlsl, by index. The last,
// "debug", is for checking how evenly frames arrive and is left out of
// the Settings page (shaderChoices).
var shaderStyles = []string{"plasma", "tunnel", "debug"}

var shaderChoices = shaderStyles[:len(shaderStyles)-1]

const shaderDebug = "debug"

func validShaderStyle(s string) bool {
	for _, x := range shaderStyles {
		if x == s {
			return true
		}
	}
	return false
}

// shaderStyle is the index of the style in use.
func (m Model) shaderStyle() int {
	if m.cfg != nil {
		for i, x := range shaderStyles {
			if x == m.cfg.Lyrics.WTShaderStyle {
				return i
			}
		}
	}
	return 0
}

// shaderActive reports whether the shader cells are drawn.
func (m Model) shaderActive() bool {
	return m.cfg != nil && m.cfg.Lyrics.WTShader && m.syncedOnScreen()
}

// shaderFx is what the shader is sent, kept with the lyric effects.
type shaderFx struct {
	kick   float64 // 1 on a beat, fading to 0 (for the speed)
	hit    float64 // how hard the last beat was, 0…1, accented
	bass   float64 // energy under ~150 Hz against its recent peak, 0…1
	level  float64 // loudness against its recent peak, 0…1
	energy float64 // loudness against the last half minute's peak, slow
	drop   float64 // 1 on a drop, fading to 0
	snare  float64 // 1 on a hit in the high bands, fading fast

	// tempo
	sinceHit float64 // seconds since the last beat
	beatSec  float64 // the beat interval, smoothed (0: unknown yet)
	count    int     // beats so far

	prevHigh  float64 // the high bands' sum last frame
	fluxAvg   float64 // their usual rise, frame to frame
	sinceSnap float64 // seconds since the last snare
	travel    float64 // 0…1, wrapping
	speed     float64 // travel a second, eased towards what the music asks
	spec      *viz.Analyzer
	bands     []float64

	lp, bassRef, levelRef, energyRef float64 // low-pass state and the recent peaks

	// the first plasma's values (cell 10)
	ck, cb, cl float64
	dropLeft   float64 // seconds the drop is still shown

	// for the debug style
	clock  float64 // 0…1 every 2 s of spotui's own time
	frames int     // effect frames so far
}

// stepShader ages the shader's levels and moves it along by dt seconds.
func (s *shaderFx) step(dt float64) {
	// bass and level are set by feed, at the analysis rate, and are not
	// faded here: fading at the faster rate in between would saw them.
	s.kick *= math.Exp(-dt * 6)
	s.drop *= math.Exp(-dt * 2.5)
	s.snare *= math.Exp(-dt * 10)
	s.sinceHit += dt
	s.sinceSnap += dt
	s.dropLeft = math.Max(0, s.dropLeft-dt)
	// Eased just enough that a beat is a lunge over a few frames rather
	// than a jump between two.
	target := 1.0*s.bass + 0.9*s.kick + 2.0*s.drop
	s.speed += (target - s.speed) * math.Min(1, dt*12)
	s.travel = math.Mod(s.travel+dt*s.speed, 1)
	s.clock = math.Mod(s.clock+dt/2, 1)
	s.frames++
}

// feed updates the levels from the analysed window, its newest samples
// and the beat detector's verdict on them.
func (s *shaderFx) feed(window, newest []float32, b viz.Beat, dt float64) {
	// Loudness, and the bass on its own through a one-pole low-pass.
	alpha := 1 - math.Exp(-2*math.Pi*shaderBassHz/viz.SampleRate)
	var e, eLow float64
	for _, x := range newest {
		v := float64(x)
		s.lp += alpha * (v - s.lp)
		e += v * v
		eLow += s.lp * s.lp
	}
	n := float64(max(len(newest), 1))
	s.level = follow(s.level, peakRatio(&s.levelRef, math.Sqrt(e/n), 0, dt), dt)
	// Against its own peak, but never one under a fifth of the loudness,
	// so music with hardly any bass does not count as all bass. Squared,
	// so a steady bass line sits in the middle and the hits stand out.
	r := peakRatio(&s.bassRef, math.Sqrt(eLow/n), 0.2*s.levelRef, dt)
	s.bass = follow(s.bass, r*r, dt)
	// Energy: the same loudness against a peak that relaxes over half a
	// minute, eased, so a chorus stands out from the verse before it.
	er := peakRatio(&s.energyRef, math.Sqrt(e/n), 0, dt*0.12)
	s.energy += (er - s.energy) * math.Min(1, dt*1.5)

	if b.Hit {
		s.beat(b)
	}
	s.feedClassic(newest, b, dt)
	if s.spec == nil {
		s.spec = viz.NewAnalyzer(shaderBands)
	}
	s.bands, _ = s.spec.Update(window, dt)
	s.snareOnset(dt)
}

// feedClassic works out cell 10 as the first plasma had it: each value
// jumps up with the music and fades, all at the analysis rate.
func (s *shaderFx) feedClassic(newest []float32, b viz.Beat, dt float64) {
	s.ck *= math.Exp(-dt * 6)
	s.cb *= math.Exp(-dt * 3)
	s.cl *= math.Exp(-dt * 2)
	var e float64
	for _, x := range newest {
		e += float64(x) * float64(x)
	}
	s.cl = math.Max(s.cl, math.Min(1, math.Sqrt(e/float64(max(len(newest), 1)))*4))
	if b.Strength > 0 {
		s.cb = math.Max(s.cb, math.Min(1, (b.Strength-0.6)/2))
	}
	if b.Hit {
		s.ck = math.Max(s.ck, math.Min(1, 0.5+(b.Strength-1)/3))
		if b.Drop {
			s.ck, s.dropLeft = 1, 0.47
		}
	}
}

// beat takes a beat: the tempo, how hard it is, which of four.
func (s *shaderFx) beat(b viz.Beat) {
	if since := s.sinceHit; since > 0.25 && since < 1.5 {
		if s.beatSec == 0 {
			s.beatSec = since
		} else {
			s.beatSec += (since - s.beatSec) * 0.2
		}
	}
	s.sinceHit = 0
	s.count++
	// From a soft beat (just over the detector's threshold) to a hard one.
	hard := math.Max(0, math.Min(1, (b.Strength-1.45)/1.5))
	s.hit = 0.35 + 0.45*hard
	if s.count%4 == 1 {
		s.hit += 0.2 // the first of four
	}
	s.kick = math.Max(s.kick, s.hit)
	if b.Drop {
		s.hit, s.kick, s.drop = 1, 1, 1
	}
}

// snareOnset looks for a sudden rise in the upper half of the spectrum
// (snares, claps, hats), against how much it usually rises.
func (s *shaderFx) snareOnset(dt float64) {
	var high float64
	for _, v := range s.bands[shaderBands/2:] {
		high += v
	}
	flux := math.Max(0, high-s.prevHigh)
	s.prevHigh = high
	hit := flux > 0.25 && flux > 2.5*s.fluxAvg && s.sinceSnap > 0.1
	s.fluxAvg += (flux - s.fluxAvg) * math.Min(1, dt*3)
	if hit {
		s.snare, s.sinceSnap = 1, 0
	}
}

// phase is how far it is from the last beat to the next, 0…1, by the
// tempo; 1 when no beat has come for longer than that.
func (s *shaderFx) phase() float64 {
	sec := s.beatSec
	if sec == 0 {
		sec = 0.5
	}
	return math.Min(1, s.sinceHit/sec)
}

// shaderStrip is the row of data cells, shaderCells wide.
func (m Model) shaderStrip() string {
	s := m.fx.shader
	byte8 := func(v float64) int { return clamp(int(math.Round(v*255)), 0, 255) }
	accent, ok := parseRGB(m.theme.accentHex)
	if !ok {
		accent = rgb{29, 185, 84}
	}
	drop := 0
	if s.dropLeft > 0 {
		drop = 255
	}
	travel := s.travel
	if shaderStyles[m.shaderStyle()] == shaderDebug {
		// spotui's clock in place of travel, to set beside the shader's,
		// and the frame count in place of the drop.
		travel, drop = s.clock, s.frames&0xff
	}
	t16 := clamp(int(travel*65536), 0, 65535)
	cells := [shaderCells][3]int{
		shaderSig,
		{byte8(s.hit), byte8(s.bass), byte8(s.level)},
		{int(accent.r), int(accent.g), int(accent.b)},
		{byte8(m.fx.hue), drop, byte8(s.energy)},
		{m.shaderStyle(), t16 >> 8, t16 & 0xff},
	}
	cells[9] = [3]int{byte8(s.phase()), ((s.count + 3) % 4) * 85, byte8(s.snare)}
	cells[10] = [3]int{byte8(s.ck), byte8(s.cb), byte8(s.cl)}
	for i := 0; i < shaderBands; i++ {
		if i < len(s.bands) {
			cells[5+i/3][i%3] = byte8(s.bands[i])
		}
	}
	cells[shaderCells-1] = shaderSig

	var sb strings.Builder
	for _, c := range cells {
		fmt.Fprintf(&sb, "\x1b[48;2;%d;%d;%dm ", c[0], c[1], c[2])
	}
	sb.WriteString("\x1b[0m")
	return sb.String()
}

// withShaderStrip puts the data cells at the start of out's first row,
// which is the app's top padding.
func (m Model) withShaderStrip(out string) string {
	if !m.shaderActive() || m.width < shaderCells+1 {
		return out
	}
	first, rest, _ := strings.Cut(out, "\n")
	if strings.TrimSpace(ansi.Strip(first)) != "" {
		return out // not the blank padding row: leave it alone
	}
	return m.shaderStrip() + strings.Repeat(" ", m.width-shaderCells) + "\n" + rest
}

// shaderBassHz is where the bass sent to the shader stops.
const shaderBassHz = 150.0

// peakRatio is v against its recent peak, 0…1, the peak taken as at least
// floor. The peak (*ref) follows v up at once and relaxes over several
// seconds, so a quiet passage comes back up to fill the range, but not
// between two beats.
func peakRatio(ref *float64, v, floor, dt float64) float64 {
	if v > *ref {
		*ref = v
	} else {
		*ref += (v - *ref) * math.Min(1, dt*0.25)
	}
	top := math.Max(*ref, floor)
	if top < 1e-4 {
		return 0
	}
	return math.Min(1, v/top)
}

// follow eases v towards target: up quickly, down a little slower.
func follow(v, target, dt float64) float64 {
	k := dt * 8
	if target > v {
		k = dt * 30
	}
	return v + (target-v)*math.Min(1, k)
}
