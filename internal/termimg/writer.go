package termimg

import (
	"image"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/charmbracelet/x/ansi"
)

// Marker is put (zero width) at the top left of the area an image should
// cover; Writer.Update finds it in the rendered frame.
const Marker = "\x1b[59m"

// Image is a picture to show over a block of cells.
type Image struct {
	Key        string // identifies content and size; same key, same picture
	Pix        image.Image
	Cols, Rows int
}

type placed struct {
	Image
	row, col int
}

// Writer sits between Bubble Tea and the terminal. Bubble Tea writes each
// frame in one call; after it, Writer draws the image again when it moved or
// changed, or after Invalidate. Update keeps the frame's text off the
// image's cells, so nothing else erases it.
type Writer struct {
	out   *os.File
	proto Protocol

	mu       sync.Mutex
	want     *placed
	shown    *placed
	dirty    bool
	sixels   map[string]string
	kittyIDs map[string]int
	nextID   int
}

// NewWriter wraps out (normally os.Stdout).
func NewWriter(out *os.File, proto Protocol) *Writer {
	return &Writer{out: out, proto: proto, sixels: map[string]string{}, kittyIDs: map[string]int{}, nextID: 100}
}

// Fd, Read and Close let Bubble Tea treat the Writer as the terminal.
func (w *Writer) Fd() uintptr                { return w.out.Fd() }
func (w *Writer) Read(p []byte) (int, error) { return w.out.Read(p) }
func (w *Writer) Close() error               { return nil }
func (w *Writer) Protocol() Protocol         { return w.proto }
func (w *Writer) Write(p []byte) (int, error) {
	n, err := w.out.Write(p)
	if err != nil {
		return n, err
	}
	w.mu.Lock()
	seq := w.pendingLocked()
	w.mu.Unlock()
	if seq != "" {
		_, _ = w.out.WriteString(seq)
	}
	return n, nil
}

// Invalidate draws the image again after the next frame, e.g. after the
// screen was cleared or resized.
func (w *Writer) Invalidate() {
	w.mu.Lock()
	w.dirty = true
	w.mu.Unlock()
}

// Update is called with every rendered frame and the image that belongs at
// the Marker in it (nil when no image should show). It returns the frame to
// hand to the terminal: on the rows the image covers, its cells become a
// cursor move. Bubble Tea rewrites a changed line whole, and writing over a
// Sixel image erases it; moving over it leaves it alone, so text beside the
// image can change every frame (karaoke lyrics, a scrolling title) without
// the image being drawn again, which flickered.
func (w *Writer) Update(frame string, img *Image) string {
	lines := strings.Split(frame, "\n")

	var want *placed
	if img != nil {
		for i, line := range lines {
			if idx := strings.Index(line, Marker); idx >= 0 {
				want = &placed{Image: *img, row: i, col: ansi.StringWidth(line[:idx])}
				break
			}
		}
	}
	if want != nil {
		for r := want.row; r < want.row+want.Rows && r < len(lines); r++ {
			lines[r] = SkipCells(lines[r], want.col, want.Cols)
		}
		frame = strings.Join(lines, "\n")
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if !samePlace(want, w.want) {
		w.dirty = true
	}
	w.want = want
	return frame
}

// SkipCells replaces n cells of line from col with a cursor move over them.
func SkipCells(line string, col, n int) string {
	if n <= 0 {
		return line
	}
	left := ansi.Truncate(line, col, "")
	right := ansi.TruncateLeft(line, col+n, "")
	return left + "\x1b[0m\x1b[" + strconv.Itoa(n) + "C" + right
}

func samePlace(a, b *placed) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Key == b.Key && a.row == b.row && a.col == b.col
}

// pendingLocked returns what to send after the frame just written.
func (w *Writer) pendingLocked() string {
	if !w.dirty {
		return ""
	}
	w.dirty = false

	var sb strings.Builder
	if w.proto == Kitty && w.shown != nil && !samePlace(w.shown, w.want) {
		sb.WriteString(KittyDelete(w.kittyIDs[w.shown.Key]))
	}
	w.shown = w.want
	if w.want == nil {
		return sb.String()
	}

	img := w.want
	sb.WriteString("\x1b7") // save the cursor; Bubble Tea relies on where it left it
	sb.WriteString("\x1b[" + strconv.Itoa(img.row+1) + ";" + strconv.Itoa(img.col+1) + "H")
	switch w.proto {
	case Sixel:
		data, ok := w.sixels[img.Key]
		if !ok {
			if len(w.sixels) > 8 {
				clear(w.sixels)
			}
			data = EncodeSixel(img.Pix)
			w.sixels[img.Key] = data
		}
		sb.WriteString(data)
	case Kitty:
		id, ok := w.kittyIDs[img.Key]
		if !ok {
			id = w.nextID
			w.nextID++
			data, err := EncodeKitty(img.Pix, id)
			if err != nil {
				return ""
			}
			w.kittyIDs[img.Key] = id
			sb.WriteString(data)
		}
		sb.WriteString(KittyPlace(id, img.Cols, img.Rows))
	}
	sb.WriteString("\x1b8")
	return sb.String()
}
