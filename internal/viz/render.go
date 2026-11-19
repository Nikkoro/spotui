package viz

import (
	"fmt"
	"image/color"
	"strings"
)

var eighths = []rune{' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// BarLayout picks how many bands fit width columns with the given bar width
// and a one column gap.
func BarLayout(width, barWidth int) int {
	return max((width+1)/(barWidth+1), 1)
}

// Render draws vertical bars height rows tall. Colours run from bottom to
// top through the gradient; peaks are drawn as a thin cap.
func Render(levels, peaks []float64, barWidth, height int, bottom, top color.RGBA) string {
	if height < 1 || len(levels) == 0 {
		return ""
	}
	var sb strings.Builder
	for row := height - 1; row >= 0; row-- {
		c := lerp(bottom, top, float64(row)/float64(max(height-1, 1)))
		peakC := lerp(c, color.RGBA{255, 255, 255, 255}, 0.35)
		fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm", c.R, c.G, c.B)

		for b, lv := range levels {
			if b > 0 {
				sb.WriteByte(' ')
			}
			fill := lv * float64(height) * 8
			cell := int(fill) - row*8
			var ch rune
			switch {
			case cell >= 8:
				ch = '█'
			case cell > 0:
				ch = eighths[cell]
			default:
				ch = ' '
			}

			// Peak cap on the row the peak sits in, above the bar.
			peakRow := int(peaks[b]*float64(height) - 0.001)
			if ch == ' ' && row == peakRow && peaks[b] > 0.02 {
				fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm%s\x1b[38;2;%d;%d;%dm",
					peakC.R, peakC.G, peakC.B, strings.Repeat("▔", barWidth), c.R, c.G, c.B)
				continue
			}
			sb.WriteString(strings.Repeat(string(ch), barWidth))
		}
		sb.WriteString("\x1b[0m")
		if row > 0 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

func lerp(a, b color.RGBA, t float64) color.RGBA {
	t = clamp01(t)
	return color.RGBA{
		R: uint8(float64(a.R) + (float64(b.R)-float64(a.R))*t),
		G: uint8(float64(a.G) + (float64(b.G)-float64(a.G))*t),
		B: uint8(float64(a.B) + (float64(b.B)-float64(a.B))*t),
		A: 255,
	}
}

// RenderMirror draws bars growing up and down from a centre line, the lower
// half a darker reflection.
func RenderMirror(levels, peaks []float64, barWidth, height int, bottom, top color.RGBA) string {
	upper := (height + 1) / 2
	lower := height - upper
	up := Render(levels, peaks, barWidth, upper, bottom, top)
	if lower == 0 {
		return up
	}
	// The reflection: same bars, no peaks, flipped, fading out.
	none := make([]float64, len(levels))
	dim := lerp(bottom, color.RGBA{16, 16, 20, 255}, 0.55)
	down := strings.Split(Render(levels, none, barWidth, lower, dim, lerp(dim, color.RGBA{16, 16, 20, 255}, 0.6)), "\n")
	for i, j := 0, len(down)-1; i < j; i, j = i+1, j-1 {
		down[i], down[j] = down[j], down[i]
	}
	// Flip the partial blocks too: a bar end that sits at the bottom of a
	// cell upwards sits at its top when mirrored.
	flip := strings.NewReplacer("▁", "▔", "▂", "▔", "▃", "▀", "▄", "▀", "▅", "▀", "▆", "█", "▇", "█")
	for i := range down {
		down[i] = flip.Replace(down[i])
	}
	return up + "\n" + strings.Join(down, "\n")
}

// Gradient derives bar colours from an accent: a deeper shade at the bottom
// rising to a lighter tint at the top.
func Gradient(accent color.RGBA) (bottom, top color.RGBA) {
	dark := color.RGBA{accent.R / 3, accent.G / 3, accent.B / 3, 255}
	bottom = lerp(dark, accent, 0.55)
	top = lerp(accent, color.RGBA{255, 255, 255, 255}, 0.25)
	return bottom, top
}

// ParseHex parses #rrggbb.
func ParseHex(s string) color.RGBA {
	var r, g, b uint8
	if _, err := fmt.Sscanf(strings.TrimPrefix(s, "#"), "%02x%02x%02x", &r, &g, &b); err != nil {
		return color.RGBA{29, 185, 84, 255}
	}
	return color.RGBA{r, g, b, 255}
}
