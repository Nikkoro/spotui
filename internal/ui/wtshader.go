package ui

import (
	"fmt"
	"math"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/Nikkoro/spotui/internal/engine"
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
//	cell 11   bass, mid and treble the way MilkDrop hears them: each
//	          band's loudness against its own average of the last second
//	          or so, 1 = as usual, sent divided by 3 (R bass up to 250 Hz,
//	          G mid to 2 kHz, B treble above)
//	cell 12   the same, smoothed (MilkDrop's bass_att and so on)
//	cell 13   R how much the camera moves with them (wt_shader_motion),
//	          G B its turn (16 bits, 0…1 a whole turn), added up here: a
//	          slow spin that the treble speeds up
//	cell 14   R the style faded from (auto), G how far the fade has got,
//	          0 showing that one … 255 showing the style in cell 4,
//	          B wt_shader_quality (0 low, 1 medium, 2 high): how much the
//	          heavy effects (chains) may draw
//	cell 15   signature again, so the shader knows where the strip ends
//
// The shader keeps no state between frames, so anything that should move
// at a speed the music sets (travel) is added up here and sent as a
// position. Only the music's share is: the steady part is timed in the
// shader, smooth at the terminal's frame rate rather than spotui's 30.
const (
	shaderBands = 12
	shaderCells = 12 + shaderBands/3
)

var shaderSig = [3]int{13, 37, 241}

// shaderStyles are the effects in shaders/spotui.hlsl, by index. The last,
// "debug", is for checking how evenly frames arrive and is left out of
// the Settings page.
var shaderStyles = []string{"plasma", "tunnel", "warp", "aurora", "synthwave", "chains", "debug"}

// shaderChoices are what Settings offers: the effects, and auto, which
// moves between them by itself (shaderauto.go).
var shaderChoices = append(append([]string{}, shaderStyles[:len(shaderStyles)-1]...), shaderAuto)

const shaderDebug = "debug"

// shaderQualities are wt_shader_quality's values, sent as their index:
// how many chains the chains effect draws and how far it marches.
var shaderQualities = []string{"low", "medium", "high"}

func validShaderQuality(s string) bool {
	return slices.Contains(shaderQualities, s)
}

// shaderQuality is the index of wt_shader_quality, medium if unset.
func (m Model) shaderQuality() int {
	if m.cfg != nil {
		if i := slices.Index(shaderQualities, m.cfg.Lyrics.WTShaderQuality); i >= 0 {
			return i
		}
	}
	return 1
}

func validShaderStyle(s string) bool {
	if s == shaderAuto {
		return true
	}
	for _, x := range shaderStyles {
		if x == s {
			return true
		}
	}
	return false
}

// shaderStyle is the index of the style showing: the one set, or with
// auto, the one auto shows.
func (m Model) shaderStyle() int {
	if m.shaderIsAuto() {
		return m.fx.shader.auto.cur
	}
	if m.cfg != nil {
		for i, x := range shaderStyles {
			if x == m.cfg.Lyrics.WTShaderStyle {
				return i
			}
		}
	}
	return 0
}

// shaderIsAuto reports whether auto picks the effect.
func (m Model) shaderIsAuto() bool {
	return m.cfg != nil && m.cfg.Lyrics.WTShaderStyle == shaderAuto
}

// shaderActive reports whether the shader cells are drawn: wherever the
// lyrics page or panel is, with lyrics or without (an instrumental, a
// track with none found, plain lyrics).
func (m Model) shaderActive() bool {
	return m.cfg != nil && m.cfg.Lyrics.WTShader && m.lyricsPanelOnScreen()
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

	travel float64 // 0…1, wrapping
	speed  float64 // travel a second, eased towards what the music asks
	spec   *viz.Analyzer
	bands  []float64

	lp, bassRef, levelRef float64 // low-pass state and the recent peaks
	slowPow, longPow      float64 // loudness over ~2 s and ~20 s, for energy

	tempoNow viz.Tempo // the tempo tracker's last verdict
	pos      time.Duration
	dump     *os.File // SPOTUI_TEMPO_DUMP, for tuning the tempo tracker

	// bass, mid, treble against their averages, as MilkDrop has them
	// (cells 11 and 12): lp1, lp2 filter states, avg the averages
	lp1, lp2   float64
	band3, att [3]float64
	avg3       [3]float64
	spin       float64 // the camera's turn, 0…1 wrapping

	auto shaderAutoState // when the effect is "auto" (shaderauto.go)

	// the first plasma's values (cell 10)
	ck, cb, cl float64
	dropLeft   float64 // seconds the drop is still shown

	// for the debug style
	clock  float64 // 0…1 every 2 s of spotui's own time
	frames int     // effect frames so far

	// for the debug log (logValues): sums over the current second
	logT, logEnergy, logLevel, logBass, logCk float64
	logBands3                                 [3]float64
	logN, logBeats, logSnares                 int
}

// stepShader ages the shader's levels and moves it along by dt seconds.
func (s *shaderFx) step(dt float64) {
	// bass and level are set by feed, at the analysis rate, and are not
	// faded here: fading at the faster rate in between would saw them.
	s.kick *= math.Exp(-dt * 6)
	s.drop *= math.Exp(-dt * 2.5)
	s.snare *= math.Exp(-dt * 10)
	s.sinceHit += dt
	s.dropLeft = math.Max(0, s.dropLeft-dt)
	// Eased just enough that a beat is a lunge over a few frames rather
	// than a jump between two.
	// A pace in the track's tempo (0.3 a second at 120 bpm), so a fast
	// song flies faster, plus the push of the bass, the beats and drops.
	pace := 0.0
	if s.beatSec > 0 {
		pace = 0.3 * 0.5 / s.beatSec
	}
	target := pace + 1.0*s.bass + 1.3*s.kick + 2.0*s.drop
	s.speed += (target - s.speed) * math.Min(1, dt*12)
	s.travel = math.Mod(s.travel+dt*s.speed, 1)
	s.clock = math.Mod(s.clock+dt/2, 1)
	// The camera turns a full circle in about 50 s, up to five times
	// faster while the treble is up. MilkDrop's turns pile up frame on
	// frame through its feedback; without one, a turn that only swung
	// with the treble barely showed, so it is added up instead.
	s.spin = math.Mod(s.spin+dt*(0.02+0.08*math.Max(0, math.Min(1, s.att[2]-0.9))), 1)
	s.frames++
}

// feed updates the levels from the analysed window and its newest
// samples. b is the plain beat detector's verdict, beat the beat the
// effects follow and t the tempo tracker's (see beatSource): the grid's
// tempo is used while onGrid, the snares are always the tracker's.
func (s *shaderFx) feed(window, newest []float32, b, beat viz.Beat, t viz.Tempo, onGrid bool, dt float64) {
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
	// Energy: the last couple of seconds against the last twenty, in dB,
	// ±3 dB filling the range. Mastered music is loud throughout, so a
	// chorus is only a few dB over its verse; a ratio to the peak barely
	// moved.
	p := e / n
	if s.longPow == 0 {
		s.slowPow, s.longPow = p, p
	}
	s.slowPow += (p - s.slowPow) * math.Min(1, dt/2)
	s.longPow += (p - s.longPow) * math.Min(1, dt/20)
	if s.longPow > 1e-9 && s.slowPow > 1e-9 {
		db := 10 * math.Log10(s.slowPow/s.longPow)
		s.energy = math.Max(0, math.Min(1, 0.5+db/6))
	}

	s.tempoNow = t
	s.dumpFrame(t, dt)
	if onGrid {
		s.beatSec = 60 / t.BPM
	}
	if beat.Hit {
		s.beat(beat, onGrid)
	}
	if t.Snare {
		s.snare = 1
		s.logSnares++
	}
	s.feedClassic(newest, b, beat, dt)
	s.feedBands3(newest, dt)
	if s.spec == nil {
		s.spec = viz.NewAnalyzer(shaderBands)
	}
	s.bands, _ = s.spec.Update(window, dt)
}

// feedClassic works out cell 10 as the first plasma had it: each value
// jumps up with the music and fades, all at the analysis rate. The bass
// is the detector's (b), against its last second, as it was; the kick
// comes on the beats in use (beat), which now keep time.
func (s *shaderFx) feedClassic(newest []float32, b, beat viz.Beat, dt float64) {
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
	if beat.Hit {
		s.ck = math.Max(s.ck, math.Min(1, 0.5+(beat.Strength-1)/3))
		if beat.Drop {
			s.ck, s.dropLeft = 1, 0.47
		}
	}
}

// feedBands3 works out bass, mid and treble the way MilkDrop does: each
// band's loudness in this frame against its own average over the last
// second and a half, 1 being as usual, and the same smoothed.
func (s *shaderFx) feedBands3(newest []float32, dt float64) {
	a1 := 1 - math.Exp(-2*math.Pi*250/viz.SampleRate)
	a2 := 1 - math.Exp(-2*math.Pi*2000/viz.SampleRate)
	var e [3]float64
	for _, x := range newest {
		v := float64(x)
		s.lp1 += a1 * (v - s.lp1)
		s.lp2 += a2 * (v - s.lp2)
		lo, mid, hi := s.lp1, s.lp2-s.lp1, v-s.lp2
		e[0] += lo * lo
		e[1] += mid * mid
		e[2] += hi * hi
	}
	n := float64(max(len(newest), 1))
	for i := range e {
		imm := math.Sqrt(e[i] / n)
		if s.avg3[i] == 0 {
			s.avg3[i] = imm
		}
		s.avg3[i] += (imm - s.avg3[i]) * math.Min(1, dt/1.5)
		v := 0.0
		if s.avg3[i] > 1e-5 {
			v = math.Min(3, imm/s.avg3[i])
		}
		s.band3[i] = v
		s.att[i] += (v - s.att[i]) * math.Min(1, dt/0.25)
	}
}

// beat takes a beat: the tempo (unless the grid sets it), how hard it is,
// which of four.
func (s *shaderFx) beat(b viz.Beat, grid bool) {
	if since := s.sinceHit; !grid && since > 0.25 && since < 1.5 {
		if s.beatSec == 0 {
			s.beatSec = since
		} else {
			s.beatSec += (since - s.beatSec) * 0.2
		}
	}
	s.sinceHit = 0
	s.count++
	s.logBeats++
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
	cells[11] = [3]int{byte8(s.band3[0] / 3), byte8(s.band3[1] / 3), byte8(s.band3[2] / 3)}
	cells[12] = [3]int{byte8(s.att[0] / 3), byte8(s.att[1] / 3), byte8(s.att[2] / 3)}
	motion := 0
	if m.cfg != nil && m.cfg.Lyrics.WTShaderMotion {
		motion = 255
	}
	spin := clamp(int(s.spin*65536), 0, 65535)
	cells[13] = [3]int{motion, spin >> 8, spin & 0xff}
	cells[14] = [3]int{m.shaderStyle(), 255, m.shaderQuality()}
	if m.shaderIsAuto() {
		cells[14] = [3]int{s.auto.prev, byte8(s.auto.blend), m.shaderQuality()}
	}
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

// logValues writes what the shader is sent to the debug log once a second
// (averages over it), with the playback position, so the effects can be
// tuned against a known track: spotui --debug, then
// grep "shader values" spotui.log.
func (s *shaderFx) logValues(dt float64, pos time.Duration, eng *engine.Engine) {
	s.pos = pos
	s.logT += dt
	s.logN++
	s.logEnergy += s.energy
	s.logLevel += s.level
	s.logBass += s.bass
	s.logCk += s.ck
	for i := range s.logBands3 {
		s.logBands3[i] += s.band3[i]
	}
	if s.logT < 1 {
		return
	}
	n := float64(s.logN)
	bpm := 0.0
	if s.beatSec > 0 {
		bpm = 60 / s.beatSec
	}
	eng.Debugf("ui: shader values at %d:%02d: energy %.2f level %.2f bass %.2f kick %.2f, %d beats %d snares, tempo %.0f bpm (grid %.0f bpm, sure %.2f), milkdrop bass %.2f mid %.2f treb %.2f",
		int(pos.Minutes()), int(pos.Seconds())%60, s.logEnergy/n, s.logLevel/n, s.logBass/n, s.logCk/n,
		s.logBeats, s.logSnares, bpm, s.tempoNow.BPM, s.tempoNow.Confidence,
		s.logBands3[0]/n, s.logBands3[1]/n, s.logBands3[2]/n)
	s.logT, s.logN, s.logBeats, s.logSnares = 0, 0, 0, 0
	s.logEnergy, s.logLevel, s.logBass, s.logCk = 0, 0, 0, 0
	s.logBands3 = [3]float64{}
}

// dumpFrame appends this analysis frame to the file named by
// SPOTUI_TEMPO_DUMP, if set: dt, the playback position and the tracker's
// log energies, one CSV line a frame. That is enough to rerun the tempo
// tracker on a real track while tuning it; no audio is written.
func (s *shaderFx) dumpFrame(t viz.Tempo, dt float64) {
	if s.dump == nil {
		path := os.Getenv("SPOTUI_TEMPO_DUMP")
		if path == "" {
			return
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return
		}
		s.dump = f
		fmt.Fprintln(f, "dt,pos,log_low,log_high,log_all")
	}
	fmt.Fprintf(s.dump, "%.5f,%.3f,%.5f,%.5f,%.5f\n", dt, s.pos.Seconds(), t.LogLow, t.LogHigh, t.LogAll)
}
