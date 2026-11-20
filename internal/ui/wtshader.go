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
//	cell 0  signature, shaderSig
//	cell 1  R kick (1 on a beat, fading), G bass, B loudness
//	cell 2  the accent colour, for the palette
//	cell 3  R colour phase in the track's tempo, G drop (shake), B zoom
//	cell 4  signature again, so the shader knows where the strip ends
const shaderCells = 5

var shaderSig = [3]int{13, 37, 241}

// shaderActive reports whether the shader cells are drawn.
func (m Model) shaderActive() bool {
	return m.cfg != nil && m.cfg.Lyrics.WTShader && m.syncedOnScreen()
}

// feedShader updates the levels sent to the shader from the newest
// samples and the beat detector's verdict on them.
func (f *lyricFx) feedShader(samples []float32, b viz.Beat) {
	var e float64
	for _, s := range samples {
		e += float64(s) * float64(s)
	}
	rms := math.Sqrt(e / float64(max(len(samples), 1)))
	f.level = math.Max(f.level, math.Min(1, rms*4))
	if b.Strength > 0 {
		f.bass = math.Max(f.bass, math.Min(1, (b.Strength-0.6)/2))
	}
	if b.Hit {
		f.kick = math.Max(f.kick, math.Min(1, 0.5+(b.Strength-1)/3))
		if b.Drop {
			f.kick = 1
		}
	}
}

// shaderStrip is the row of data cells, shaderCells wide.
func (m Model) shaderStrip() string {
	f := m.fx
	byte8 := func(v float64) int { return clamp(int(math.Round(v*255)), 0, 255) }
	accent, ok := parseRGB(m.theme.accentHex)
	if !ok {
		accent = rgb{29, 185, 84}
	}
	drop := 0.0
	if f.shake > 0 {
		drop = 1
	}
	cells := [shaderCells][3]int{
		shaderSig,
		{byte8(f.kick), byte8(f.bass), byte8(f.level)},
		{int(accent.r), int(accent.g), int(accent.b)},
		{byte8(f.hue), byte8(drop), byte8(f.zoom)},
		shaderSig,
	}
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
