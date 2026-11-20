package ui

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

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
	m.page = pageQueue
	if strings.Contains(first(m), sigEscape()) {
		t.Fatal("strip drawn without lyrics on screen")
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
			s.feed(tone(hz, amp, 2048), tone(hz, amp, 1470), viz.Beat{}, dt)
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

func TestShaderTempoAccentsAndSnare(t *testing.T) {
	var s shaderFx
	dt := 1.0 / 60
	beat := func(strength float64) {
		s.beat(viz.Beat{Hit: true, Strength: strength})
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

	// A sudden rise in the high bands is a snare; a steady level is not.
	s.bands = make([]float64, shaderBands)
	s.snareOnset(1.0 / 30)
	for i := shaderBands / 2; i < shaderBands; i++ {
		s.bands[i] = 0.8
	}
	s.snareOnset(1.0 / 30)
	if s.snare != 1 {
		t.Fatal("no snare on a jump in the highs")
	}
	s.snare = 0
	for j := 0; j < 10; j++ {
		s.step(1.0 / 30)
		s.snareOnset(1.0 / 30)
	}
	if s.snare != 0 {
		t.Fatal("snare on steady highs")
	}
}
