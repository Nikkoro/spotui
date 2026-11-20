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
// frame in one call; after it, Writer draws the image again if the frame
// touched the rows it lies on, or if it moved or changed. A Sixel image is
// erased by any text written over it, and Bubble Tea only rewrites lines
// that changed, so most frames need nothing at all.
type Writer struct {
	out   *os.File
	proto Protocol

	mu       sync.Mutex
	want     *placed
	shown    *placed
	prev     []string // previous frame's lines
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
// the Marker in it (nil when no image should show).
func (w *Writer) Update(frame string, img *Image) {
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

	w.mu.Lock()
	defer w.mu.Unlock()

	switch {
	case !samePlace(want, w.want):
		w.dirty = true
	case want != nil && rowsChanged(w.prev, lines, want.row, want.Rows):
		w.dirty = true
	}
	w.want = want
	w.prev = lines
}

func samePlace(a, b *placed) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Key == b.Key && a.row == b.row && a.col == b.col
}

func rowsChanged(prev, cur []string, from, n int) bool {
	for i := from; i < from+n; i++ {
		var p, c string
		if i < len(prev) {
			p = prev[i]
		}
		if i < len(cur) {
			c = cur[i]
		}
		if p != c {
			return true
		}
	}
	return false
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
