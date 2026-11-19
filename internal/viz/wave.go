package viz

import (
	"fmt"
	"image/color"
	"math"
	"strings"
)

// Braille cells give 2×4 dots per character, enough for a smooth
// oscilloscope line in a terminal.
var brailleBits = [4][2]rune{
	{0x01, 0x08},
	{0x02, 0x10},
	{0x04, 0x20},
	{0x40, 0x80},
}

// RenderWave draws samples as an oscilloscope trace width×height cells,
// using gain to scale the amplitude. Rows are coloured from the centre
// (bottom colour) to the edges (top colour).
func RenderWave(samples []float32, gain float64, width, height int, bottom, top color.RGBA) string {
	if width < 1 || height < 1 {
		return ""
	}
	dotsW, dotsH := width*2, height*4
	grid := make([][]rune, height)
	for i := range grid {
		grid[i] = make([]rune, width)
	}

	plot := func(x, y int) {
		if x < 0 || x >= dotsW || y < 0 || y >= dotsH {
			return
		}
		grid[y/4][x/2] |= brailleBits[y%4][x%2]
	}

	mid := float64(dotsH-1) / 2
	prevY := -1
	for x := 0; x < dotsW; x++ {
		v := 0.0
		if len(samples) > 0 {
			// Average the samples falling on this column.
			from := x * len(samples) / dotsW
			to := max((x+1)*len(samples)/dotsW, from+1)
			for _, s := range samples[from:min(to, len(samples))] {
				v += float64(s)
			}
			v /= float64(to - from)
		}
		y := int(math.Round(mid - math.Max(-1, math.Min(1, v*gain))*mid))
		// Join to the previous column so steep edges stay connected.
		if prevY >= 0 {
			step := 1
			if y < prevY {
				step = -1
			}
			for yy := prevY; yy != y; yy += step {
				plot(x, yy)
			}
		}
		plot(x, y)
		prevY = y
	}

	var sb strings.Builder
	for row := 0; row < height; row++ {
		dist := math.Abs(float64(row)-float64(height-1)/2) / math.Max(float64(height-1)/2, 1)
		c := lerp(bottom, top, dist)
		fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm", c.R, c.G, c.B)
		for _, r := range grid[row] {
			if r == 0 {
				sb.WriteByte(' ')
			} else {
				sb.WriteRune(0x2800 + r)
			}
		}
		sb.WriteString("\x1b[0m")
		if row < height-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// WaveGain picks a gain that fills most of the height for the window's
// peak, eased so quiet passages do not blow up into noise.
func WaveGain(samples []float32) float64 {
	peak := 0.0
	for _, s := range samples {
		peak = math.Max(peak, math.Abs(float64(s)))
	}
	if peak < 1e-4 {
		return 1
	}
	return math.Min(0.9/peak, 8)
}
