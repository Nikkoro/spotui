package termimg

import (
	"image"
	"image/color"
	"os"
	"strings"
	"testing"
)

func gradient(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 255 / w), uint8(y * 255 / h), uint8((x + y) % 256), 255})
		}
	}
	return img
}

func TestEncodeSixelShape(t *testing.T) {
	s := EncodeSixel(gradient(40, 25))
	if !strings.HasPrefix(s, "\x1bP0;1;0q\"1;1;40;25") || !strings.HasSuffix(s, "\x1b\\") {
		t.Fatalf("bad framing: %q…%q", s[:20], s[len(s)-4:])
	}
	if bands := strings.Count(s, "-"); bands != 5 { // ceil(25/6)
		t.Fatalf("%d bands, want 5", bands)
	}
	if strings.ContainsAny(s, "\n\r") {
		t.Fatal("sixel must stay on one line for the renderer")
	}
	if n := strings.Count(s, ";2;"); n > 256 {
		t.Fatalf("%d colour registers, max 256", n)
	}
}

func TestEncodeSixelRunLength(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 6))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	s := EncodeSixel(img)
	if !strings.Contains(s, "!100~") {
		t.Fatalf("a flat band should compress to one run: %q", s)
	}
}

func TestWriterRedrawsOnlyWhenNeeded(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	w := NewWriter(f, Sixel)
	img := &Image{Key: "a", Pix: gradient(20, 20), Cols: 2, Rows: 1}

	frame := "header\n  " + Marker + "  info\nfooter"
	w.Update(frame, img)
	if w.want == nil || w.want.row != 1 || w.want.col != 2 {
		t.Fatalf("placement %+v, want row 1 col 2", w.want)
	}

	size := func() int64 { st, _ := f.Stat(); return st.Size() }
	_, _ = w.Write([]byte("frame1"))
	afterFirst := size()
	if afterFirst <= int64(len("frame1")) {
		t.Fatal("image not drawn after the first frame")
	}

	// Same rows again: nothing extra.
	w.Update(frame, img)
	_, _ = w.Write([]byte("frame2"))
	if got := size() - afterFirst; got != int64(len("frame2")) {
		t.Fatalf("redrew without a reason (%d extra bytes)", got-int64(len("frame2")))
	}

	// A change on the image's row means the text overwrote it: redraw.
	before := size()
	w.Update("header\n  "+Marker+"  INFO\nfooter", img)
	_, _ = w.Write([]byte("frame3"))
	if size()-before <= int64(len("frame3")) {
		t.Fatal("image not redrawn after its row changed")
	}

	// A change elsewhere does not touch it.
	before = size()
	w.Update("HEADER\n  "+Marker+"  INFO\nfooter", img)
	_, _ = w.Write([]byte("frame4"))
	if size()-before != int64(len("frame4")) {
		t.Fatal("redrew for a change on another row")
	}

	// Gone from the frame: nothing to draw.
	w.Update("other page", img)
	before = size()
	_, _ = w.Write([]byte("frame5"))
	if size()-before != int64(len("frame5")) {
		t.Fatal("drew an image that is not on screen")
	}
}

func TestKittyTransmitsOnce(t *testing.T) {
	f, _ := os.CreateTemp(t.TempDir(), "out")
	w := NewWriter(f, Kitty)
	img := &Image{Key: "a", Pix: gradient(8, 8), Cols: 2, Rows: 1}
	w.Update(Marker, img)
	_, _ = w.Write(nil)
	w.Invalidate()
	_, _ = w.Write(nil)
	b, _ := os.ReadFile(f.Name())
	if n := strings.Count(string(b), "a=t"); n != 1 {
		t.Fatalf("transmitted %d times, want once", n)
	}
	if n := strings.Count(string(b), "a=p"); n != 2 {
		t.Fatalf("placed %d times, want 2", n)
	}
}
