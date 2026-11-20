package viz

import "math"

// BeatDetector finds beats in the audio: sudden rises of bass energy over
// its recent average. A drop is a very strong beat after a quieter stretch,
// where a track's energy comes back in.
type BeatDetector struct {
	lp      float64   // one-pole low-pass state, keeps the kick and the bass
	hist    []float64 // recent frame energies, about a second
	pos     int
	filled  int
	long    float64 // slow average energy, several seconds
	sinceMs float64 // since the last beat
}

// Beat is what one frame found.
type Beat struct {
	Hit      bool    // a beat starts in this frame
	Strength float64 // how far above the recent average, 1 = average
	Drop     bool    // a big comeback after a quieter stretch
}

// NewBeatDetector keeps about a second of history at 30 frames a second.
func NewBeatDetector() *BeatDetector {
	return &BeatDetector{hist: make([]float64, 32)}
}

const (
	beatCutoffHz = 160.0
	beatMinGapMs = 220.0 // faster than ~270 bpm is not a beat
	beatRatio    = 1.45  // above the recent average by this much
	beatFloor    = 1e-5  // ignore near silence
	dropRatio    = 2.4   // against the slow average
)

// Feed analyses the newest samples (mono, SampleRate) covering dtMs.
func (b *BeatDetector) Feed(samples []float32, dtMs float64) Beat {
	b.sinceMs += dtMs
	if len(samples) == 0 {
		return Beat{}
	}
	alpha := 1 - math.Exp(-2*math.Pi*beatCutoffHz/SampleRate)
	var e float64
	for _, s := range samples {
		b.lp += alpha * (float64(s) - b.lp)
		e += b.lp * b.lp
	}
	e /= float64(len(samples))

	var avg float64
	for i := 0; i < b.filled; i++ {
		avg += b.hist[i]
	}
	if b.filled > 0 {
		avg /= float64(b.filled)
	}
	quietBefore := b.long > 0 && avg < 0.7*b.long

	b.hist[b.pos] = e
	b.pos = (b.pos + 1) % len(b.hist)
	b.filled = min(b.filled+1, len(b.hist))
	if b.long == 0 {
		b.long = e
	} else {
		b.long += (e - b.long) * 0.01 // ~3 s at 30 fps
	}

	if b.filled < len(b.hist)/2 || e < beatFloor || avg <= 0 {
		return Beat{}
	}
	strength := e / avg
	if strength < beatRatio || b.sinceMs < beatMinGapMs {
		return Beat{Strength: strength}
	}
	b.sinceMs = 0
	return Beat{Hit: true, Strength: strength, Drop: quietBefore && e > dropRatio*b.long}
}
