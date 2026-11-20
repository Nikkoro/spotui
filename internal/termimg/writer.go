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
// cover; Writer.Update finds it in the rendered frame. It is the first
// image's; MarkerFor gives the others theirs.
const Marker = "\x1b[59m"

// MarkerFor is the marker of the i-th image of a frame (0-based): the
// Marker repeated, harmless to the terminal, and told apart by length.
func MarkerFor(i int) string { return strings.Repeat(Marker, i+1) }

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
	want     []placed
	shown    []placed
	dirty    bool // draw every image again
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

// Update is called with every rendered frame and the images that belong at
// its markers (imgs[i] at MarkerFor(i); nil entries are skipped). It
// returns the frame to hand to the terminal: on the rows an image covers,
// its cells become a cursor move. Bubble Tea rewrites a changed line whole,
// and writing over a Sixel image erases it; moving over it leaves it alone,
// so text beside an image can change every frame (karaoke lyrics, a
// scrolling title) without the image being drawn again, which flickered.
func (w *Writer) Update(frame string, imgs []*Image) string {
	lines := strings.Split(frame, "\n")

	// Longest markers first, each taken out once found, so a shorter one is
	// never found inside a longer one.
	var want []placed
	for i := len(imgs) - 1; i >= 0; i-- {
		if imgs[i] == nil {
			continue
		}
		mk := MarkerFor(i)
		for r, line := range lines {
			if idx := strings.Index(line, mk); idx >= 0 {
				want = append(want, placed{Image: *imgs[i], row: r, col: ansi.StringWidth(line[:idx])})
				lines[r] = line[:idx] + line[idx+len(mk):]
				break
			}
		}
	}
	for _, p := range want {
		for r := p.row; r < p.row+p.Rows && r < len(lines); r++ {
			lines[r] = SkipCells(lines[r], p.col, p.Cols)
		}
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	w.want = want
	return strings.Join(lines, "\n")
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

func samePlace(a, b placed) bool {
	return a.Key == b.Key && a.row == b.row && a.col == b.col
}

func contains(list []placed, p placed) bool {
	for _, q := range list {
		if samePlace(p, q) {
			return true
		}
	}
	return false
}

// pendingLocked returns what to send after the frame just written: the
// images that are new or moved (all of them after Invalidate), and for kitty
// the removal of those no longer shown.
func (w *Writer) pendingLocked() string {
	var sb strings.Builder
	if w.proto == Kitty {
		for _, p := range w.shown {
			if !contains(w.want, p) {
				sb.WriteString(KittyDelete(w.kittyIDs[p.Key]))
			}
		}
	}
	for _, p := range w.want {
		if !w.dirty && contains(w.shown, p) {
			continue
		}
		w.drawLocked(&sb, p)
	}
	w.dirty = false
	w.shown = append(w.shown[:0], w.want...)
	return sb.String()
}

func (w *Writer) drawLocked(sb *strings.Builder, img placed) {
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
				sb.WriteString("\x1b8")
				return
			}
			w.kittyIDs[img.Key] = id
			sb.WriteString(data)
		}
		sb.WriteString(KittyPlace(id, img.Cols, img.Rows))
	}
	sb.WriteString("\x1b8")
}
