package viz

import (
	"math"
	"math/cmplx"
	"strings"
	"testing"
	"time"
)

func sine(freq float64) []float32 {
	s := make([]float32, fftSize)
	for i := range s {
		s[i] = float32(0.8 * math.Sin(2*math.Pi*freq*float64(i)/SampleRate))
	}
	return s
}

func TestFFTFindsTone(t *testing.T) {
	x := make([]complex128, fftSize)
	for i, v := range sine(1000) {
		x[i] = complex(float64(v), 0)
	}
	fft(x)
	best, bestMag := 0, 0.0
	for k := 1; k < fftSize/2; k++ {
		if m := cmplx.Abs(x[k]); m > bestMag {
			best, bestMag = k, m
		}
	}
	got := float64(best) * SampleRate / fftSize
	if math.Abs(got-1000) > SampleRate/fftSize {
		t.Fatalf("peak at %.0f Hz, want ~1000 Hz", got)
	}
}

func TestAnalyzerLocatesBands(t *testing.T) {
	a := NewAnalyzer(24)
	var lv []float64
	for i := 0; i < 30; i++ {
		lv, _ = a.Update(sine(100), 1.0/30)
	}
	lowBand := argmax(lv)

	b := NewAnalyzer(24)
	for i := 0; i < 30; i++ {
		lv, _ = b.Update(sine(5000), 1.0/30)
	}
	highBand := argmax(lv)

	if lowBand >= highBand {
		t.Fatalf("100 Hz in band %d, 5 kHz in band %d; want low < high", lowBand, highBand)
	}
}

func TestAnalyzerDecaysToIdle(t *testing.T) {
	a := NewAnalyzer(16)
	for i := 0; i < 10; i++ {
		a.Update(sine(440), 1.0/30)
	}
	if a.Idle() {
		t.Fatal("should not be idle while playing")
	}
	for i := 0; i < 200; i++ {
		a.Update(nil, 1.0/30)
	}
	if !a.Idle() {
		t.Fatal("should fall idle after silence")
	}
}

func TestTapSnapshot(t *testing.T) {
	tap := NewTap()
	r := tap.Wrap(readerFunc(func(buf []float32) (int, error) {
		for i := range buf {
			buf[i] = float32(i / 2)
		}
		return len(buf), nil
	}))
	buf := make([]float32, 2*3000)
	if _, err := r.Read(buf); err != nil {
		t.Fatal(err)
	}
	dst := make([]float32, 1000)
	if !tap.Snapshot(dst, 500) {
		t.Fatal("snapshot failed")
	}
	if dst[0] != 1500 || dst[999] != 2499 {
		t.Fatalf("got %v..%v, want 1500..2499", dst[0], dst[999])
	}
	if tap.Snapshot(make([]float32, 3000), 500) {
		t.Fatal("snapshot larger than history must fail")
	}
}

func TestTapWalksThroughBigReads(t *testing.T) {
	tap := NewTap()
	clock := time.Unix(0, 0)
	tap.now = func() time.Time { return clock }
	r := tap.Wrap(readerFunc(func(buf []float32) (int, error) {
		for i := range buf {
			buf[i] = float32(i / 2)
		}
		return len(buf), nil
	}))
	// One second pulled at once, as PulseAudio under WSLg does.
	if _, err := r.Read(make([]float32, 2*SampleRate)); err != nil {
		t.Fatal(err)
	}
	dst := make([]float32, 100)
	clock = clock.Add(250 * time.Millisecond)
	if !tap.Snapshot(dst, 0) || dst[99] != float32(SampleRate/4-1) {
		t.Fatalf("after 250 ms the window ends at %v, want %d", dst[99], SampleRate/4-1)
	}
	clock = clock.Add(500 * time.Millisecond)
	if !tap.Snapshot(dst, 0) || dst[99] != float32(3*SampleRate/4-1) {
		t.Fatalf("after 750 ms the window ends at %v", dst[99])
	}
	if !tap.Active() {
		t.Fatal("still inside its read: active")
	}
	clock = clock.Add(3 * time.Second)
	if tap.Active() {
		t.Fatal("no read for 3 s: not active")
	}
}

func TestRenderShape(t *testing.T) {
	out := Render([]float64{0, 0.5, 1}, []float64{0, 0.6, 1}, 2, 4, ParseHex("#1DB954"), ParseHex("#ffffff"))
	if rows := strings.Count(out, "\n") + 1; rows != 4 {
		t.Fatalf("got %d rows", rows)
	}
}

type readerFunc func([]float32) (int, error)

func (f readerFunc) Read(b []float32) (int, error) { return f(b) }

func argmax(v []float64) int {
	best := 0
	for i := range v {
		if v[i] > v[best] {
			best = i
		}
	}
	return best
}

func TestRenderWave(t *testing.T) {
	out := RenderWave(sine(441)[:441], 1, 20, 4, ParseHex("#1DB954"), ParseHex("#ffffff"))
	if rows := strings.Count(out, "\n") + 1; rows != 4 {
		t.Fatalf("got %d rows", rows)
	}
	if !strings.ContainsRune(out, 0x2800+0x01) && !strings.ContainsAny(out, "⠁⠂⠄⡀⠈⠐⠠⢀") {
		// any braille dot will do; check at least one cell was drawn
		drawn := false
		for _, r := range out {
			if r > 0x2800 && r <= 0x28FF {
				drawn = true
			}
		}
		if !drawn {
			t.Fatal("nothing drawn")
		}
	}
	silent := RenderWave(nil, 1, 10, 3, ParseHex("#1DB954"), ParseHex("#ffffff"))
	if !strings.ContainsRune(silent, '⠤') && !strings.ContainsRune(silent, '⠒') && !strings.ContainsRune(silent, '⠉') && !strings.ContainsRune(silent, '⣀') {
		// silence is a flat line through the middle; some braille must exist
		found := false
		for _, r := range silent {
			if r > 0x2800 && r <= 0x28FF {
				found = true
			}
		}
		if !found {
			t.Fatal("silence should draw a flat line")
		}
	}
}
