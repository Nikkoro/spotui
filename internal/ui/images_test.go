package ui

import (
	"image"
	"image/color"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/devgianlu/go-librespot/daemon"

	"github.com/Nikkoro/spotui/internal/config"
	"github.com/Nikkoro/spotui/internal/termimg"
)

func imageModel(t *testing.T) (Model, *os.File) {
	f, err := os.CreateTemp(t.TempDir(), "term")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	m := New(nil, &cfg, nil).WithImages(termimg.NewWriter(f, termimg.Sixel), termimg.Caps{Protocol: termimg.Sixel, CellW: 10, CellH: 20})
	m.width, m.height = 120, 40
	m.phase = phaseReady
	m.page = pageNowPlaying
	m.track = &daemon.ApiTrack{Name: "Midnight City", ArtistNames: []string{"M83"}, AlbumName: "Hurry Up", Duration: 240000, Uri: "spotify:track:1"}
	m.coverURL = "u"
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 4), uint8(y * 4), 128, 255})
		}
	}
	m.coverImg = img
	m.imgPixels = 16
	m.paused = false
	m.stopped = false
	m.setPosition(10000)
	return m, f
}

func TestNowPlayingDrawsSixelOnce(t *testing.T) {
	m, f := imageModel(t)
	_ = m.View()
	if len(m.frame.imgs) == 0 {
		t.Fatal("no image placed on Now Playing")
	}
	_, _ = m.img.Write([]byte("F1"))
	b, _ := os.ReadFile(f.Name())
	if !strings.Contains(string(b), "\x1bP0;1;0q") {
		t.Fatal("no sixel written after the first frame")
	}
	first := len(b)

	// Time passes: the progress bar moves, but it sits below the cover,
	// so the image must not be sent again.
	m.posAt = m.posAt.Add(-3 * time.Second)
	_ = m.View()
	_, _ = m.img.Write([]byte("F2"))
	b, _ = os.ReadFile(f.Name())
	if extra := len(b) - first - 2; extra != 0 {
		t.Fatalf("image resent (%d bytes) for a progress tick", extra)
	}

	// Coarser pixels: new picture.
	m.imgPixels = 8
	_ = m.View()
	_, _ = m.img.Write([]byte("F3"))
	b2, _ := os.ReadFile(f.Name())
	if len(b2)-len(b) <= 2 {
		t.Fatal("changing the pixel grid did not redraw")
	}
}

func TestImageModeOffUsesBlocks(t *testing.T) {
	m, _ := imageModel(t)
	m.imageMode = false
	_ = m.View()
	if len(m.frame.imgs) != 0 {
		t.Fatal("blocks mode must not place an image")
	}
}

func TestContextPageCoverIsAnImage(t *testing.T) {
	m, _ := imageModel(t)
	cover := image.NewRGBA(image.Rect(0, 0, 32, 32))
	m.ctxPage = &contextPage{uri: "spotify:playlist:x", title: "A playlist", subtitle: "by someone", kind: "playlist",
		img: cover, imgURL: "p"}
	m.page = pageContext
	_ = m.View()
	// The page's cover and the playing track's in the bar below.
	if len(m.frame.imgs) != 2 {
		t.Fatalf("placed %d images, want 2", len(m.frame.imgs))
	}
}
