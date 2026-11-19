// Package cover turns album art into pixel-art terminal graphics.
package cover

import (
	"context"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg" // Spotify serves covers as JPEG
	_ "image/png"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

// Pixels is a downscaled cover as a square grid of colours.
type Pixels struct {
	Size   int
	Colors []color.RGBA // row-major, Size*Size
}

func (p *Pixels) at(x, y int) color.RGBA {
	return p.Colors[y*p.Size+x]
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

// Fetch downloads and decodes an image.
func Fetch(ctx context.Context, url string) (image.Image, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cover download: %s", resp.Status)
	}

	img, _, err := image.Decode(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("decoding cover: %w", err)
	}
	return img, nil
}

// Pixelate box-filters img down to size×size and posterises each channel to
// the given number of levels (0 or >=256 keeps full colour). Box filtering
// averages every source pixel, which keeps the blocky look clean instead of
// the aliasing nearest-neighbour sampling gives.
func Pixelate(img image.Image, size, levels int) *Pixels {
	if size < 1 {
		size = 1
	}
	b := img.Bounds()
	// Crop to a centred square so non-square images are not stretched.
	side := min(b.Dx(), b.Dy())
	ox := b.Min.X + (b.Dx()-side)/2
	oy := b.Min.Y + (b.Dy()-side)/2

	out := &Pixels{Size: size, Colors: make([]color.RGBA, size*size)}
	for cy := 0; cy < size; cy++ {
		y0 := oy + cy*side/size
		y1 := max(oy+(cy+1)*side/size, y0+1)
		for cx := 0; cx < size; cx++ {
			x0 := ox + cx*side/size
			x1 := max(ox+(cx+1)*side/size, x0+1)

			var r, g, bl, n uint64
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					pr, pg, pb, _ := img.At(x, y).RGBA()
					r += uint64(pr >> 8)
					g += uint64(pg >> 8)
					bl += uint64(pb >> 8)
					n++
				}
			}
			out.Colors[cy*size+cx] = color.RGBA{
				R: posterize(uint8(r/n), levels),
				G: posterize(uint8(g/n), levels),
				B: posterize(uint8(bl/n), levels),
				A: 255,
			}
		}
	}
	return out
}

func posterize(v uint8, levels int) uint8 {
	if levels <= 1 || levels >= 256 {
		return v
	}
	step := 255.0 / float64(levels-1)
	return uint8(math.Round(math.Round(float64(v)/step) * step))
}

// Render draws the pixels with upper half blocks: each character cell shows
// two vertically stacked pixels (foreground on top, background below), so a
// size×size image takes size columns and size/2 rows and stays square in a
// typical 1:2 terminal cell.
func (p *Pixels) Render() string {
	var sb strings.Builder
	for y := 0; y < p.Size; y += 2 {
		for x := 0; x < p.Size; x++ {
			top := p.at(x, y)
			bottom := top
			if y+1 < p.Size {
				bottom = p.at(x, y+1)
			}
			fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀",
				top.R, top.G, top.B, bottom.R, bottom.G, bottom.B)
		}
		sb.WriteString("\x1b[0m")
		if y+2 < p.Size {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// Accent picks a vivid colour from the cover for theming: the average of the
// most saturated, reasonably bright pixels, nudged brighter so it reads well
// on a dark background. ok is false for greyscale covers.
func Accent(p *Pixels) (c color.RGBA, ok bool) {
	type scored struct {
		c     color.RGBA
		score float64
	}
	var picks []scored
	for _, px := range p.Colors {
		_, s, l := toHSL(px)
		if s < 0.25 || l < 0.2 || l > 0.85 {
			continue
		}
		picks = append(picks, scored{px, s * (1 - math.Abs(l-0.55))})
	}
	if len(picks) == 0 {
		return color.RGBA{}, false
	}

	// Average the top quarter by score.
	best := picks
	if len(best) > 4 {
		// partial selection sort is plenty for <= 1024 pixels
		for i := 0; i < len(best)/4; i++ {
			m := i
			for j := i + 1; j < len(best); j++ {
				if best[j].score > best[m].score {
					m = j
				}
			}
			best[i], best[m] = best[m], best[i]
		}
		best = best[:len(best)/4]
	}

	var r, g, b float64
	for _, s := range best {
		r += float64(s.c.R)
		g += float64(s.c.G)
		b += float64(s.c.B)
	}
	n := float64(len(best))
	avg := color.RGBA{uint8(r / n), uint8(g / n), uint8(b / n), 255}

	h, s, l := toHSL(avg)
	l = math.Max(l, 0.6)
	s = math.Min(1, s*1.15)
	return fromHSL(h, s, l), true
}

func toHSL(c color.RGBA) (h, s, l float64) {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	mx := math.Max(r, math.Max(g, b))
	mn := math.Min(r, math.Min(g, b))
	l = (mx + mn) / 2
	if mx == mn {
		return 0, 0, l
	}
	d := mx - mn
	if l > 0.5 {
		s = d / (2 - mx - mn)
	} else {
		s = d / (mx + mn)
	}
	switch mx {
	case r:
		h = (g - b) / d
		if g < b {
			h += 6
		}
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	return h / 6, s, l
}

func fromHSL(h, s, l float64) color.RGBA {
	if s == 0 {
		v := uint8(math.Round(l * 255))
		return color.RGBA{v, v, v, 255}
	}
	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q
	conv := func(t float64) uint8 {
		if t < 0 {
			t++
		}
		if t > 1 {
			t--
		}
		var v float64
		switch {
		case t < 1.0/6:
			v = p + (q-p)*6*t
		case t < 0.5:
			v = q
		case t < 2.0/3:
			v = p + (q-p)*(2.0/3-t)*6
		default:
			v = p
		}
		return uint8(math.Round(v * 255))
	}
	return color.RGBA{conv(h + 1.0/3), conv(h), conv(h - 1.0/3), 255}
}

// Hex formats a colour as #rrggbb.
func Hex(c color.RGBA) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}
