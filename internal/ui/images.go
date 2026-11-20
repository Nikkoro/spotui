package ui

import (
	"fmt"
	"image"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Nikkoro/spotui/internal/cover"
	"github.com/Nikkoro/spotui/internal/termimg"
)

// imgFrame carries the image a View wants shown; View fills it, then hands
// it to the terminal writer with the finished frame.
type imgFrame struct {
	imgs []*termimg.Image
}

// placeImage registers img for this frame and returns the marker to put at
// its top left cell.
func (m Model) placeImage(img *termimg.Image) string {
	if m.frame == nil {
		return ""
	}
	m.frame.imgs = append(m.frame.imgs, img)
	return termimg.MarkerFor(len(m.frame.imgs) - 1)
}

// WithImages lets the interface draw real cover images through w.
func (m Model) WithImages(w *termimg.Writer, caps termimg.Caps) Model {
	m.img = w
	m.caps = caps
	m.imageMode = w != nil && caps.Protocol != termimg.Blocks
	if m.imageMode && m.savedImageMode != nil {
		m.imageMode = *m.savedImageMode // the user turned images off last time
	}
	return m
}

func (m Model) imagesOn() bool {
	return m.imageMode && m.img != nil && m.img.Protocol() != termimg.Blocks && m.cfg.Cover.Enabled
}

// Pixel grid presets for image covers; 0 means no pixelation.
var imagePixelPresets = []int{8, 12, 16, 24, 32, 48, 64, 96, 128, 0}

func nextImagePixels(cur, dir int) int {
	idx := len(imagePixelPresets) - 1
	for i, v := range imagePixelPresets {
		if v == cur {
			idx = i
			break
		}
	}
	return imagePixelPresets[clamp(idx+dir, 0, len(imagePixelPresets)-1)]
}

func imagePixelsLabel(n int) string {
	if n <= 0 {
		return "full detail"
	}
	return fmt.Sprintf("%d×%d pixels", n, n)
}

// coverImage renders the cover for cols×rows cells, pixelated to the chosen
// grid and scaled up with hard edges so every pixel stays a crisp square.
func (m Model) coverImage(cols, rows int) *termimg.Image {
	return m.imageOf(m.coverURL, m.coverImg, cols, rows)
}

// imageOf is any cover (src, known as id) drawn like coverImage.
func (m Model) imageOf(id string, src image.Image, cols, rows int) *termimg.Image {
	if src == nil {
		return nil
	}
	side := min(cols*m.caps.CellW, rows*m.caps.CellH)
	key := fmt.Sprintf("%s|%d|%d|%d|%d", id, m.imgPixels, m.imgLevels, cols, rows)
	if pix, ok := m.imgCache[key]; ok {
		return &termimg.Image{Key: key, Pix: pix, Cols: cols, Rows: rows}
	}

	grid := m.imgPixels
	if grid <= 0 || grid > side {
		grid = side
	}
	px := cover.Pixelate(src, grid, m.imgLevels)
	out := image.NewRGBA(image.Rect(0, 0, side, side))
	for y := 0; y < side; y++ {
		sy := y * px.Size / side
		for x := 0; x < side; x++ {
			c := px.Colors[sy*px.Size+x*px.Size/side]
			i := out.PixOffset(x, y)
			out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = c.R, c.G, c.B, 255
		}
	}
	if len(m.imgCache) > 6 {
		clear(m.imgCache)
	}
	m.imgCache[key] = out
	return &termimg.Image{Key: key, Pix: out, Cols: cols, Rows: rows}
}

// viewNowPlayingImage lays Now Playing out for a real image: only text that
// changes with the track sits beside the cover, everything that moves goes
// below it, so the image is drawn again only when the track changes.
func (m Model) viewNowPlayingImage(w int) (string, bool) {
	if m.track == nil {
		return "", false
	}
	availH := m.height - 12
	availW := w - 8
	cw, ch := max(m.caps.CellW, 1), max(m.caps.CellH, 1)

	vizH := 0
	if m.vizVisible() && availH >= 24 {
		vizH = clamp(availH/5, 3, 6)
		availH -= vizH + 1
	}
	dynamicH := 6
	if m.cfg.Lyrics.NowPlayingLine && m.lyrics != nil && len(m.lyrics.Synced) > 0 {
		dynamicH += 2
	}
	maxRows := availH - dynamicH - 1
	staticW := clamp(availW/3, 24, 44)

	cols := min(availW-4-staticW, maxRows*ch/cw)
	if size := m.cfg.Cover.Size; size > 0 {
		cols = min(cols, size)
	}
	if cols < 12 {
		return "", false
	}
	rows := max(1, (cols*cw+ch/2)/ch)

	img := m.coverImage(cols, rows)
	var coverBlock string
	if img == nil {
		coverBlock = m.coverPlaceholder(rows * 2)
		coverBlock = lipgloss.NewStyle().Width(cols).Height(rows).Render(coverBlock)
	} else {
		lines := make([]string, rows)
		blank := strings.Repeat(" ", cols)
		for i := range lines {
			lines[i] = blank
		}
		lines[0] = m.placeImage(img) + blank
		coverBlock = strings.Join(lines, "\n")
	}

	static := lipgloss.Place(staticW, rows, lipgloss.Left, lipgloss.Center, m.viewTrackStatic(staticW, false))
	top := lipgloss.JoinHorizontal(lipgloss.Top, coverBlock, "    ", static)
	width := lipgloss.Width(top)

	parts := []string{top, "", m.viewTrackDynamic(width)}
	if vizH > 0 {
		parts = append(parts, "", m.viewVisualizer(width, vizH))
	}
	return m.theme.card.Render(lipgloss.JoinVertical(lipgloss.Left, parts...)), true
}
