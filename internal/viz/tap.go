// Package viz turns the decoded audio stream into spectrum bars.
package viz

import (
	"sync"

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
}

// NewTap keeps about one second of audio.
func NewTap() *Tap {
	return &Tap{ring: make([]float32, 1<<16)}
}

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
	for i := 0; i+1 < len(interleaved); i += Channels {
		t.ring[t.total&mask] = (interleaved[i] + interleaved[i+1]) * 0.5
		t.total++
	}
}

// Snapshot copies the n samples that end delay samples before the newest
// one into dst (len n). The delay compensates for the device buffer: the
// output reads audio a little before it is heard. It returns false until
// enough audio has been seen.
func (t *Tap) Snapshot(dst []float32, delay int) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := int64(len(dst))
	end := t.total - int64(delay)
	start := end - n
	if start < 0 || int64(delay)+n > int64(len(t.ring)) {
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
