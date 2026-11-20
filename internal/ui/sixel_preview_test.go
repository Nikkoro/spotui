//go:build preview

package ui

import (
	"os"
	"testing"

	"github.com/Nikkoro/spotui/internal/termimg"
)

func TestPreviewSixel(t *testing.T) {
	dir := os.Getenv("PREVIEW_DIR")
	if dir == "" {
		t.Skip()
	}
	m := sampleModel(t)
	m.caps = termimg.Caps{Protocol: termimg.Sixel, CellW: 10, CellH: 20}
	for _, px := range []int{64, 16, 0} {
		m.imgPixels = px
		img := m.coverImage(36, 18)
		_ = os.WriteFile(dir+"/cover-"+itoa(px)+".six", []byte(termimg.EncodeSixel(img.Pix)), 0o644)
	}
}
