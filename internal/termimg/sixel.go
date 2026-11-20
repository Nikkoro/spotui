// Package termimg shows real images in terminals that support Sixel or the
// kitty graphics protocol, next to a Bubble Tea interface.
package termimg

import (
	"image"
	"image/color"
	"sort"
	"strconv"
	"strings"
)

// EncodeSixel turns img into a Sixel escape sequence. Colours are reduced
// to at most 256 registers, the most terminals offer, by coarsening the
// image's colour levels until they fit. Pixelated covers have few colours,
// so this rarely changes anything visible.
func EncodeSixel(img image.Image) string {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return ""
	}

	pix := make([]color.RGBA, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			pix[y*w+x] = color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8), 255}
		}
	}
	palette, index := quantize(pix)

	var sb strings.Builder
	sb.Grow(w * h / 2)
	// P2=1: pixels we do not set keep what is under them. Raster attributes
	// give a 1:1 pixel aspect ratio and the size.
	sb.WriteString("\x1bP0;1;0q\"1;1;")
	sb.WriteString(strconv.Itoa(w))
	sb.WriteByte(';')
	sb.WriteString(strconv.Itoa(h))
	for i, c := range palette {
		sb.WriteByte('#')
		sb.WriteString(strconv.Itoa(i))
		sb.WriteString(";2;")
		sb.WriteString(strconv.Itoa(int(c.R) * 100 / 255))
		sb.WriteByte(';')
		sb.WriteString(strconv.Itoa(int(c.G) * 100 / 255))
		sb.WriteByte(';')
		sb.WriteString(strconv.Itoa(int(c.B) * 100 / 255))
	}

	row := make([]byte, w)
	for band := 0; band < h; band += 6 {
		// Which colours appear in this band of six pixel rows.
		used := map[int]bool{}
		for y := band; y < min(band+6, h); y++ {
			for x := 0; x < w; x++ {
				used[index[y*w+x]] = true
			}
		}
		colors := make([]int, 0, len(used))
		for c := range used {
			colors = append(colors, c)
		}
		sort.Ints(colors)

		for ci, c := range colors {
			for x := 0; x < w; x++ {
				var bits byte
				for dy := 0; dy < 6 && band+dy < h; dy++ {
					if index[(band+dy)*w+x] == c {
						bits |= 1 << dy
					}
				}
				row[x] = 63 + bits
			}
			sb.WriteByte('#')
			sb.WriteString(strconv.Itoa(c))
			writeRLE(&sb, row)
			if ci < len(colors)-1 {
				sb.WriteByte('$') // back to the start of the band for the next colour
			}
		}
		sb.WriteByte('-') // next band
	}
	sb.WriteString("\x1b\\")
	return sb.String()
}

// writeRLE writes sixel characters, compressing runs with !<count><char>.
func writeRLE(sb *strings.Builder, row []byte) {
	for i := 0; i < len(row); {
		j := i
		for j < len(row) && row[j] == row[i] {
			j++
		}
		n := j - i
		if n > 3 {
			sb.WriteByte('!')
			sb.WriteString(strconv.Itoa(n))
			sb.WriteByte(row[i])
		} else {
			for k := 0; k < n; k++ {
				sb.WriteByte(row[i])
			}
		}
		i = j
	}
}

// quantize maps pixels onto at most 256 colours.
func quantize(pix []color.RGBA) (palette []color.RGBA, index []int) {
	for _, levels := range []int{256, 32, 16, 12, 8, 6} {
		palette = palette[:0]
		seen := map[color.RGBA]int{}
		index = make([]int, len(pix))
		ok := true
		for i, p := range pix {
			if levels < 256 {
				p = color.RGBA{step(p.R, levels), step(p.G, levels), step(p.B, levels), 255}
			}
			idx, found := seen[p]
			if !found {
				if len(palette) == 256 {
					ok = false
					break
				}
				idx = len(palette)
				seen[p] = idx
				palette = append(palette, p)
			}
			index[i] = idx
		}
		if ok {
			return palette, index
		}
	}
	return palette, index // 6 levels: 216 colours, always fits
}

func step(v uint8, levels int) uint8 {
	s := 255.0 / float64(levels-1)
	return uint8(float64(int(float64(v)/s+0.5)) * s)
}
