package viz

import (
	"math"
	"math/cmplx"
)

const fftSize = 2048

// Analyzer converts sample windows into smoothed, log-spaced band levels.
type Analyzer struct {
	window []float64
	buf    []complex128
	edges  []int // FFT bin boundaries per band, len = bands+1

	levels []float64 // smoothed 0..1
	peaks  []float64 // falling peak markers 0..1
	gain   float64   // automatic gain (running loudness reference)
}

// NewAnalyzer splits 40 Hz – 16 kHz into bands log-spaced bands.
func NewAnalyzer(bands int) *Analyzer {
	a := &Analyzer{
		window: make([]float64, fftSize),
		buf:    make([]complex128, fftSize),
		gain:   1e-3,
	}
	for i := range a.window {
		a.window[i] = 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(fftSize-1))) // Hann
	}
	a.SetBands(bands)
	return a
}

// SetBands changes the number of bands, resetting their state.
func (a *Analyzer) SetBands(bands int) {
	bands = max(bands, 1)
	if len(a.levels) == bands {
		return
	}
	a.levels = make([]float64, bands)
	a.peaks = make([]float64, bands)
	a.edges = bandEdges(bands)
}

// Bands returns the current band count.
func (a *Analyzer) Bands() int { return len(a.levels) }

func bandEdges(bands int) []int {
	const lo, hi = 40.0, 16000.0
	binHz := float64(SampleRate) / fftSize
	edges := make([]int, bands+1)
	for i := 0; i <= bands; i++ {
		f := lo * math.Pow(hi/lo, float64(i)/float64(bands))
		edges[i] = int(math.Round(f / binHz))
	}
	// Every band needs at least one bin of its own.
	for i := 1; i <= bands; i++ {
		if edges[i] <= edges[i-1] {
			edges[i] = edges[i-1] + 1
		}
	}
	if last := edges[bands]; last > fftSize/2 {
		edges[bands] = fftSize / 2
	}
	return edges
}

// Update analyses samples (len fftSize, may be nil for silence) and
// advances the smoothing by dt seconds. It returns levels and peaks, both
// owned by the analyzer.
func (a *Analyzer) Update(samples []float32, dt float64) (levels, peaks []float64) {
	target := make([]float64, len(a.levels))

	if len(samples) == fftSize {
		for i, s := range samples {
			a.buf[i] = complex(float64(s)*a.window[i], 0)
		}
		fft(a.buf)

		loudest := 0.0
		for b := range target {
			lo, hi := a.edges[b], a.edges[b+1]
			if lo >= fftSize/2 {
				lo = fftSize/2 - 1
			}
			hi = min(max(hi, lo+1), fftSize/2)
			// Peak magnitude in the band keeps narrow tones visible in
			// wide high bands.
			m := 0.0
			for k := lo; k < hi; k++ {
				m = max(m, cmplx.Abs(a.buf[k]))
			}
			// Gentle tilt so highs, naturally quieter, still move.
			m *= 1 + 1.5*float64(b)/float64(len(target))
			target[b] = m
			loudest = max(loudest, m)
		}

		// Automatic gain: chase the loudest band quickly upwards, relax
		// slowly so quiet passages fill the bars again.
		if loudest > a.gain {
			a.gain += (loudest - a.gain) * 0.5
		} else {
			a.gain += (loudest - a.gain) * math.Min(1, dt*0.4)
		}
		ref := math.Max(a.gain, 1e-3)

		for b, m := range target {
			// Map to a 45 dB range under the reference.
			db := 20 * math.Log10(math.Max(m/ref, 1e-9))
			target[b] = clamp01((db + 45) / 45)
		}
	}

	attack := math.Min(1, dt*25)
	decay := math.Min(1, dt*6)
	peakFall := dt * 0.6
	for b := range a.levels {
		if target[b] > a.levels[b] {
			a.levels[b] += (target[b] - a.levels[b]) * attack
		} else {
			a.levels[b] += (target[b] - a.levels[b]) * decay
		}
		if a.levels[b] >= a.peaks[b] {
			a.peaks[b] = a.levels[b]
		} else {
			a.peaks[b] = math.Max(a.levels[b], a.peaks[b]-peakFall)
		}
	}
	return a.levels, a.peaks
}

// Idle reports whether every bar has fallen to (almost) zero.
func (a *Analyzer) Idle() bool {
	for b := range a.levels {
		if a.levels[b] > 0.01 || a.peaks[b] > 0.01 {
			return false
		}
	}
	return true
}

func clamp01(v float64) float64 {
	return math.Max(0, math.Min(1, v))
}

// fft is an in-place iterative radix-2 Cooley–Tukey transform.
func fft(x []complex128) {
	n := len(x)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		step := cmplx.Exp(complex(0, -2*math.Pi/float64(size)))
		for start := 0; start < n; start += size {
			w := complex(1, 0)
			for k := 0; k < size/2; k++ {
				u := x[start+k]
				v := x[start+k+size/2] * w
				x[start+k] = u + v
				x[start+k+size/2] = u - v
				w *= step
			}
		}
	}
}
