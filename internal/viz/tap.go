// Package viz turns the decoded audio stream into spectrum bars.
package viz

import (
	"sync"
	"time"

	librespot "github.com/devgianlu/go-librespot"
)

// SampleRate and Channels match what go-librespot hands to the output.
const (
	SampleRate = 44100
	Channels   = 2
)

// Tap keeps the most recent mono samples written to the audio device.
type Tap struct {
	mu    sync.Mutex
	ring  []float32
	total int64 // mono samples written since start

	// The last read: when, and how many samples it took. Some outputs
	// (PulseAudio under WSLg) pull a second or more at once; the playhead
	// then walks through that chunk in real time instead of the bars
	// freezing until the next pull and jumping.
	lastAt    time.Time
	lastChunk int64
	now       func() time.Time
}

// NewTap keeps about three seconds of audio.
func NewTap() *Tap {
	return &Tap{ring: make([]float32, 1<<17), now: time.Now}
}

// bigChunk is the read size from which the playhead walks through it in
// real time; smaller reads are heard about as soon as they are taken.
const bigChunk = SampleRate / 10

// Wrap is installed as output.ReaderHook.
func (t *Tap) Wrap(r librespot.Float32Reader) librespot.Float32Reader {
	return &tapReader{r: r, tap: t}
}

type tapReader struct {
	r   librespot.Float32Reader
	tap *Tap
}

func (tr *tapReader) Read(buf []float32) (int, error) {
	n, err := tr.r.Read(buf)
	if n > 0 {
		tr.tap.write(buf[:n])
	}
	return n, err
}

func (t *Tap) write(interleaved []float32) {
	t.mu.Lock()
	defer t.mu.Unlock()
	mask := int64(len(t.ring) - 1)
	before := t.total
	for i := 0; i+1 < len(interleaved); i += Channels {
		t.ring[t.total&mask] = (interleaved[i] + interleaved[i+1]) * 0.5
		t.total++
	}
	t.lastChunk = t.total - before
	t.lastAt = t.now()
}

// playheadLocked estimates the newest sample being played: the end of what
// was read, or for a big read, how far into it real time has got.
func (t *Tap) playheadLocked() int64 {
	if t.lastChunk < bigChunk || t.lastAt.IsZero() {
		return t.total
	}
	elapsed := int64(t.now().Sub(t.lastAt).Seconds() * SampleRate)
	return min(t.total, t.total-t.lastChunk+elapsed)
}

// Active reports whether the output is still pulling audio: a read within
// the last 1.5 s, or within two of its usual reads if they are longer.
func (t *Tap) Active() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.lastAt.IsZero() {
		return false
	}
	limit := max(1500*time.Millisecond, 2*time.Duration(t.lastChunk)*time.Second/SampleRate)
	return t.now().Sub(t.lastAt) < limit
}

// Snapshot copies the n samples that end delay samples before the playhead
// into dst (len n). The delay compensates for the device buffer: the
// output reads audio a little before it is heard. It returns false until
// enough audio has been seen.
func (t *Tap) Snapshot(dst []float32, delay int) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := int64(len(dst))
	end := t.playheadLocked() - int64(delay)
	start := end - n
	if start < 0 || t.total-start > int64(len(t.ring)) {
		return false
	}
	mask := int64(len(t.ring) - 1)
	for i := int64(0); i < n; i++ {
		dst[i] = t.ring[(start+i)&mask]
	}
	return true
}

// Total returns how many mono samples have been written, to detect when
// the device stopped pulling audio.
func (t *Tap) Total() int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.total
}
