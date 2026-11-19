package cover

import (
	"image"
	"image/color"
	"strings"
	"testing"
)

func gradient(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 255 / w), uint8(y * 255 / h), 200, 255})
		}
	}
	return img
}

func TestPixelateSizeAndCrop(t *testing.T) {
	p := Pixelate(gradient(640, 480), 16, 0)
	if p.Size != 16 || len(p.Colors) != 256 {
		t.Fatalf("got size %d with %d colours", p.Size, len(p.Colors))
	}
	// Left edge must be darker in red than the right edge.
	if p.at(0, 0).R >= p.at(15, 0).R {
		t.Errorf("expected red to increase left to right")
	}
}

func TestPosterize(t *testing.T) {
	for _, v := range []uint8{0, 37, 128, 200, 255} {
		got := posterize(v, 2)
		if got != 0 && got != 255 {
			t.Errorf("posterize(%d, 2) = %d, want 0 or 255", v, got)
		}
	}
	if posterize(123, 0) != 123 {
		t.Errorf("levels 0 must keep the value")
	}
}

func TestRenderRows(t *testing.T) {
	p := Pixelate(gradient(100, 100), 10, 4)
	out := p.Render()
	if rows := strings.Count(out, "\n") + 1; rows != 5 {
		t.Errorf("got %d rows, want 5", rows)
	}
	if n := strings.Count(out, "▀"); n != 50 {
		t.Errorf("got %d half blocks, want 50", n)
	}
}

func TestAccent(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for i := range img.Pix {
		img.Pix[i] = 128 // grey
	}
	if _, ok := Accent(Pixelate(img, 8, 0)); ok {
		t.Errorf("grey image must not yield an accent")
	}

	if c, ok := Accent(Pixelate(gradient(64, 64), 16, 0)); !ok {
		t.Errorf("colourful image must yield an accent")
	} else if _, _, l := toHSL(c); l < 0.59 {
		t.Errorf("accent too dark: %v", c)
	}
}

func TestHSLRoundTrip(t *testing.T) {
	for _, c := range []color.RGBA{{255, 0, 0, 255}, {30, 185, 84, 255}, {12, 34, 200, 255}} {
		h, s, l := toHSL(c)
		got := fromHSL(h, s, l)
		if absDiff(got.R, c.R) > 1 || absDiff(got.G, c.G) > 1 || absDiff(got.B, c.B) > 1 {
			t.Errorf("round trip %v -> %v", c, got)
		}
	}
}

func absDiff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}
