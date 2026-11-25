package ui

import (
	"fmt"
	"math"
	"math/rand/v2"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/Nikkoro/spotui/internal/lyrics"
	"github.com/Nikkoro/spotui/internal/viz"
	"github.com/Nikkoro/spotui/shaders"
)

func sigEscape() string {
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm ", shaderSig[0], shaderSig[1], shaderSig[2])
}

func TestShaderStripOnlyWhenAskedAndLyricsShow(t *testing.T) {
	m := fxModel(t, fxOff)
	first := func(m Model) string { l, _, _ := strings.Cut(m.View(), "\n"); return l }

	if strings.Contains(first(m), sigEscape()) {
		t.Fatal("strip drawn with wt_shader off")
	}
	m.cfg.Lyrics.WTShader = true
	row := first(m)
	if !strings.HasPrefix(row, sigEscape()) || strings.Count(row, sigEscape()) != 2 {
		t.Fatalf("no strip at the start of the top row: %q", row)
	}
	if w := lipgloss.Width(row); w != m.width {
		t.Fatalf("top row %d wide, want %d", w, m.width)
	}
	// An instrumental, or no lyrics found: the strip stays.
	m.lyrics = &lyrics.Lyrics{Instrumental: true}
	if !strings.HasPrefix(first(m), sigEscape()) {
		t.Fatal("no strip for an instrumental")
	}
	m.lyrics = nil
	if !strings.HasPrefix(first(m), sigEscape()) {
		t.Fatal("no strip without lyrics")
	}
	m.page = pageQueue
	if strings.Contains(first(m), sigEscape()) {
		t.Fatal("strip drawn without the lyrics page or panel")
	}
}

func TestShaderGetsTheBeatWithEffectsOff(t *testing.T) {
	m := fxModel(t, fxOff)
	m.cfg.Lyrics.WTShader = true
	dt := 1.0 / 30
	for i := 0; i < 40; i++ {
		m.stepFx(kick(0.02), dt)
	}
	m.stepFx(kick(0.8), dt)
	s := m.fx.shader
	if s.kick < 0.5 || s.level == 0 || len(s.bands) != shaderBands {
		t.Fatalf("shader not fed: kick %.2f, level %.2f, %d bands", s.kick, s.level, len(s.bands))
	}
	if s.bands[0] <= s.bands[shaderBands-1] {
		t.Fatalf("a 60 Hz kick should light the low bands: %.2f", s.bands)
	}
	if m.fx.pulse != 0 || len(m.fx.sparks) != 0 {
		t.Fatal("effects off but the line moved")
	}
}

func TestShaderMatchesTheHLSL(t *testing.T) {
	src := string(shaders.Source)
	for _, want := range []string{
		fmt.Sprintf("float3(%d.0, %d.0, %d.0) / 255.0", shaderSig[0], shaderSig[1], shaderSig[2]),
		fmt.Sprintf("STRIP_CELLS = %d;", shaderCells),
		fmt.Sprintf("BANDS = %d;", shaderBands),
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("shaders/spotui.hlsl lacks %q", want)
		}
	}
	// Style indices as the shader numbers them.
	var styles []string
	for i, s := range shaderStyles {
		styles = append(styles, fmt.Sprintf("%d %s", i, s))
	}
	if want := "(" + strings.Join(styles, ", ") + ")"; !strings.Contains(src, want) {
		t.Fatalf("shaders/spotui.hlsl does not number the styles %s", want)
	}
}

func TestShaderStripCarriesStyleAndTravel(t *testing.T) {
	m := fxModel(t, fxOff)
	m.cfg.Lyrics.WTShader = true
	m.cfg.Lyrics.WTShaderStyle = "tunnel"
	m.fx.shader.travel = 0.5
	strip := m.shaderStrip()
	if want := "\x1b[48;2;1;128;0m "; !strings.Contains(strip, want) {
		t.Fatalf("style 1, travel 0.5 not sent as %q: %q", want, strip)
	}
	if n := strings.Count(strip, "\x1b[48;2;"); n != shaderCells {
		t.Fatalf("%d cells, want %d", n, shaderCells)
	}

	// Travel wraps and moves faster with the bass.
	slow, fast := shaderFx{}, shaderFx{bass: 1}
	for i := 0; i < 6; i++ {
		slow.step(1.0 / 60)
		fast.step(1.0 / 60)
	}
	if fast.travel <= slow.travel {
		t.Fatalf("bass did not speed it up: %.3f vs %.3f", fast.travel, slow.travel)
	}
	s := shaderFx{travel: 0.99, speed: 1, bass: 1}
	s.step(0.1)
	if s.travel >= 0.99 || s.travel < 0 {
		t.Fatalf("travel did not wrap: %.3f", s.travel)
	}
}

func TestShaderStripCarriesQuality(t *testing.T) {
	m := fxModel(t, fxOff)
	m.cfg.Lyrics.WTShader = true
	m.cfg.Lyrics.WTShaderStyle = "chains"
	for i, q := range shaderQualities {
		m.cfg.Lyrics.WTShaderQuality = q
		// cell 14: the style faded from (none: chains itself), done, quality
		if want := fmt.Sprintf("\x1b[48;2;5;255;%dm ", i); !strings.Contains(m.shaderStrip(), want) {
			t.Fatalf("quality %s not sent as %q", q, want)
		}
	}
	m.cfg.Lyrics.WTShaderQuality = ""
	if m.shaderQuality() != 1 {
		t.Fatal("an unset quality is not medium")
	}
}

func TestFrameStatsSummary(t *testing.T) {
	var ds []time.Duration
	for i := 1; i <= 100; i++ {
		ds = append(ds, time.Duration(i)*time.Millisecond)
	}
	got := summary(ds)
	if got[0] != 50.5 || got[1] != 96 || got[2] != 100 {
		t.Fatalf("avg/p95/max = %v", got)
	}
	if summary(nil) != [3]float64{} {
		t.Fatal("empty summary not zero")
	}
}

func TestShaderKeepsFeedingWithEffectsOff(t *testing.T) {
	m := fxModel(t, fxOff)
	m.cfg.Lyrics.WTShader = true
	m.vizAt = time.Now().Add(-vizFrameInterval)
	if cmd := m.stepVisualizer(time.Now()); cmd == nil {
		t.Fatal("no next tick")
	}
	if m.vizAt.IsZero() {
		t.Fatal("went idle while feeding the shader")
	}
}

func tone(hz, amp float64, n int) []float32 {
	out := make([]float32, n)
	for i := range out {
		out[i] = float32(amp * math.Sin(2*math.Pi*hz*float64(i)/viz.SampleRate))
	}
	return out
}

func TestShaderBassFollowsTheBass(t *testing.T) {
	var s shaderFx
	dt := 1.0 / 30
	feed := func(hz, amp float64, frames int) {
		for i := 0; i < frames; i++ {
			s.feed(tone(hz, amp, 2048), tone(hz, amp, 1470), viz.Beat{}, viz.Beat{}, viz.Tempo{}, false, dt)
		}
	}
	feed(60, 0.8, 30)
	if s.bass < 0.9 {
		t.Fatalf("loud bass gives %.2f", s.bass)
	}
	feed(60, 0.4, 10) // the same line, quieter: well under the top
	if s.bass > 0.5 || s.bass < 0.1 {
		t.Fatalf("half the bass gives %.2f", s.bass)
	}
	s = shaderFx{}
	feed(5000, 0.8, 30) // highs only
	if s.bass > 0.1 {
		t.Fatalf("a 5 kHz tone gives bass %.2f", s.bass)
	}
	if s.level < 0.9 {
		t.Fatalf("a loud tone gives level %.2f", s.level)
	}
}

func TestShaderTempoAndAccents(t *testing.T) {
	var s shaderFx
	dt := 1.0 / 60
	beat := func(strength float64) {
		s.beat(viz.Beat{Hit: true, Strength: strength}, false)
	}
	// Four beats half a second apart: the tempo is learnt, the first of
	// four is the hardest, and the phase runs 0 to 1 over a beat.
	var hits []float64
	for i := 0; i < 4; i++ {
		beat(1.6)
		hits = append(hits, s.hit)
		for j := 0; j < 30; j++ {
			s.step(dt)
		}
	}
	for i := 1; i < 4; i++ {
		if hits[0]-hits[i] < 0.15 {
			t.Fatalf("first of four not accented: %.2f", hits)
		}
	}
	if math.Abs(s.beatSec-0.5) > 0.02 {
		t.Fatalf("tempo %.3f s, want 0.5", s.beatSec)
	}
	if p := s.phase(); p < 0.95 {
		t.Fatalf("phase %.2f at the next beat", p)
	}
	beat(3.5)
	if s.phase() != 0 || s.hit < 0.95 {
		t.Fatalf("hard first beat: phase %.2f hit %.2f", s.phase(), s.hit)
	}
}

func TestShaderEnergyRisesInALouderPart(t *testing.T) {
	var s shaderFx
	dt := 1.0 / 30
	feed := func(amp float64, seconds float64) {
		for i := 0; i < int(seconds*30); i++ {
			s.feed(tone(220, amp, 2048), tone(220, amp, 1470), viz.Beat{}, viz.Beat{}, viz.Tempo{}, false, dt)
		}
	}
	feed(0.3, 20) // a verse
	verse := s.energy
	feed(0.42, 3) // a chorus, 3 dB louder: mastered music is not more
	if s.energy-verse < 0.3 {
		t.Fatalf("energy %.2f in the verse, %.2f in the chorus", verse, s.energy)
	}
}

func TestShaderBandsAgainstTheirAverage(t *testing.T) {
	var s shaderFx
	dt := 1.0 / 30
	feed := func(hz, amp float64, frames int) {
		for i := 0; i < frames; i++ {
			s.feedBands3(tone(hz, amp, 1470), dt)
		}
	}
	// A steady bass note: as usual, 1.
	feed(80, 0.3, 90)
	if math.Abs(s.band3[0]-1) > 0.1 {
		t.Fatalf("steady bass reads %.2f, want about 1", s.band3[0])
	}
	// Twice as loud all of a sudden: well over 1, then settling back.
	feed(80, 0.6, 1)
	if s.band3[0] < 1.6 {
		t.Fatalf("a louder bass reads %.2f", s.band3[0])
	}
	feed(80, 0.6, 150)
	if math.Abs(s.band3[0]-1) > 0.1 || math.Abs(s.att[0]-1) > 0.1 {
		t.Fatalf("bass %.2f att %.2f after five seconds, want about 1", s.band3[0], s.att[0])
	}
	// Each band hears its own range: a hat-like 6 kHz tone after silence
	// lifts the treble, not the bass.
	s = shaderFx{}
	feed(80, 0.3, 60)
	feed(6000, 0.3, 1)
	if s.band3[2] < 1.5 || s.band3[0] > 1.2 {
		t.Fatalf("a 6 kHz hit: bass %.2f treble %.2f", s.band3[0], s.band3[2])
	}
}

func TestShaderAutoPicksForTheMood(t *testing.T) {
	var a shaderAutoState
	a.rng = rand.New(rand.NewPCG(1, 2))
	count := func(mood float64) map[int]int {
		got := map[int]int{}
		for i := 0; i < 4000; i++ {
			a.started, a.cur = true, 0 // the plasma showing
			got[a.pick(mood)]++
		}
		return got
	}
	calm, wild := count(0.1), count(0.9)
	if calm[0] != 0 || wild[0] != 0 {
		t.Fatal("picked the effect already showing")
	}
	if calm[3] <= calm[1] || calm[3] <= calm[2] {
		t.Fatalf("calm music should lean to the aurora: %v", calm)
	}
	if wild[1] <= wild[3] || wild[2] <= wild[3] {
		t.Fatalf("intense music should lean to the tunnel and warp: %v", wild)
	}
	if calm[1] == 0 || wild[3] == 0 {
		t.Fatalf("every effect should keep a chance: calm %v wild %v", calm, wild)
	}
}

func TestShaderAutoChangesWithTheMusic(t *testing.T) {
	var s shaderFx
	s.auto.rng = rand.New(rand.NewPCG(3, 4))
	dt := 1.0 / 30
	run := func(seconds float64, drop bool, track string) {
		for i := 0; i < int(seconds*30); i++ {
			s.stepAuto(dt, false, drop && i == 0, track, nil)
		}
	}
	run(1, false, "a")
	first := s.auto.cur
	if s.auto.blend != 1 {
		t.Fatal("the first effect should show at once")
	}
	run(3, true, "a") // a drop too soon after the start: no change
	if s.auto.cur != first {
		t.Fatal("changed on a drop two seconds after the last change")
	}
	run(4, false, "a")
	run(0.2, true, "a") // a drop now: a quick fade
	if s.auto.cur == first || s.auto.fade != autoDropFade {
		t.Fatalf("no quick change on a drop: cur %d fade %.2f", s.auto.cur, s.auto.fade)
	}
	run(1, false, "a")
	if s.auto.blend != 1 {
		t.Fatalf("the drop's fade should be over: %.2f", s.auto.blend)
	}
	before := s.auto.cur
	run(0.1, false, "b") // a new track
	if s.auto.cur == before || s.auto.prev != before || s.auto.fade != autoFade {
		t.Fatal("no fade to a new effect on a new track")
	}
	run(1, false, "b")
	if s.auto.blend <= 0.3 || s.auto.blend >= 0.5 {
		t.Fatalf("a second into a %.1f s fade: blend %.2f", autoFade, s.auto.blend)
	}
}

func TestShaderAutoDropsGoWild(t *testing.T) {
	calm := 0
	for i := 0; i < 400; i++ {
		var s shaderFx // a quiet build-up: no energy, no bass, no tempo
		s.auto.rng = rand.New(rand.NewPCG(uint64(i), 9))
		s.stepAuto(1.0/30, false, false, "a", nil)
		s.stepAuto(10, false, false, "a", nil)
		s.stepAuto(1.0/30, false, true, "a", nil)
		if s.auto.cur == 3 {
			calm++
		}
	}
	if calm > 60 {
		t.Fatalf("a drop out of a quiet build-up went to the aurora %d times in 400", calm)
	}
}

// HLSL is compiled by Windows Terminal only, so a name it reserves shows up
// as an error dialog there. Catch the ones that read like ordinary words.
func TestShaderUsesNoReservedNames(t *testing.T) {
	reserved := []string{"line", "lineadj", "point", "triangle", "triangleadj", "sample", "centroid",
		"linear", "vector", "matrix", "texture", "string", "shared", "uniform", "pass", "precise",
		"half", "register", "auto", "default", "case", "near", "far", "this", "new", "union", "long",
		"short", "class", "template", "char", "packoffset", "compile", "inline", "technique"}
	decl := regexp.MustCompile(`\b(?:float[234]?|int[234]?|bool|uint|half)\s+(\w+)`)
	for _, m := range decl.FindAllStringSubmatch(string(shaders.Source), -1) {
		if slices.Contains(reserved, m[1]) {
			t.Errorf("shaders/spotui.hlsl names something %q, which HLSL reserves", m[1])
		}
	}
}
