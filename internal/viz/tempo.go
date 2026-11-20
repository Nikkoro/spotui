package viz

import "math"

// TempoTracker follows the beat of the music the way a DJ program does,
// rather than taking every loud moment for a beat: it builds an onset
// envelope (sudden rises in loudness, in the bass and across the band),
// finds the tempo from how that envelope repeats over the last few seconds
// (autocorrelation), and keeps a grid of beats locked to it. A kick buried
// under a steady bass line, or one missing for a bar, does not lose the
// beat.
//
// Feed it each analysis frame (about 30 a second). Frames are taken as
// evenly spaced; the frame rate is measured from dt.
type TempoTracker struct {
	env    []float64 // onset envelope, a ring of the last tempoWindow seconds
	pos    int
	filled int
	fps    float64 // frames a second, smoothed

	lpLow, lpHigh       float64 // one-pole filter states
	prevLow, prevAll    float64 // last frame's log energies
	prevHigh, highAvg   float64 // for snares
	highPeak            float64 // the highs' recent peak energy
	sinceSnare, sinceUp float64 // seconds

	period    float64 // frames a beat (0: no tempo yet)
	candidate float64 // a different period seen at the last updates…
	seen      int     // …this many in a row
	conf      float64 // how clearly the envelope repeats at period, 0…1
	sincePeak float64 // frames since the grid's last beat
}

// Tempo is what one frame found.
type Tempo struct {
	Hit        bool    // the grid has a beat in this frame
	Strength   float64 // how hard: the onset near it against the usual, 1 = usual
	Phase      float64 // 0 on a beat, 1 at the next
	BPM        float64 // 0 while unknown
	Confidence float64 // 0…1; below about 0.15 the grid is a guess
	Snare      bool    // a sudden rise in the highs: snare, clap, hat

	// This frame's log energies, for tuning against a recording
	// (SPOTUI_TEMPO_DUMP).
	LogLow, LogHigh, LogAll float64
}

const (
	tempoWindow  = 8.0  // seconds of envelope the tempo is found in
	tempoSpan    = 3.5  // seconds of whole beats a tempo is judged over
	tempoEvery   = 0.5  // seconds between tempo updates
	tempoMinBPM  = 70.0 // tempos outside are folded in by octaves
	tempoMaxBPM  = 180.0
	tempoLowHz   = 150.0
	tempoHighHz  = 2500.0
	snareRise    = 0.9  // log rise of the highs that counts as a hit…
	snareOverAvg = 2.2  // …and against their usual rise…
	snareLoud    = 0.25 // …and loud against the highs' recent peak (not a hat)
	snareGap     = 0.12 // seconds
)

// NewTempoTracker starts with no tempo.
func NewTempoTracker() *TempoTracker {
	return &TempoTracker{fps: 30}
}

// Feed analyses the newest samples, covering dt seconds.
func (t *TempoTracker) Feed(samples []float32, dt float64) Tempo {
	// Energies: the bass, the highs, everything.
	aLow := 1 - math.Exp(-2*math.Pi*tempoLowHz/SampleRate)
	aHigh := 1 - math.Exp(-2*math.Pi*tempoHighHz/SampleRate)
	var eLow, eHigh, eAll float64
	for _, s := range samples {
		v := float64(s)
		t.lpLow += aLow * (v - t.lpLow)
		t.lpHigh += aHigh * (v - t.lpHigh)
		h := v - t.lpHigh
		eLow += t.lpLow * t.lpLow
		eHigh += h * h
		eAll += v * v
	}
	n := float64(max(len(samples), 1))
	const floor = 1e-7
	return t.FeedEnergies(math.Log(eLow/n+floor), math.Log(eHigh/n+floor), math.Log(eAll/n+floor), dt)
}

// FeedEnergies takes a frame as its log energies (bass, highs, all) instead
// of samples: Feed works them out, and a recording of them
// (SPOTUI_TEMPO_DUMP) can be replayed through it while tuning.
func (t *TempoTracker) FeedEnergies(lLow, lHigh, lAll, dt float64) Tempo {
	if dt > 0 && dt < 0.2 {
		t.fps += (1/dt - t.fps) * 0.05
	}
	if t.env == nil {
		t.env = make([]float64, int(tempoWindow*t.fps))
	}
	t.sinceSnare += dt
	t.sinceUp += dt
	eHigh, eAll := math.Exp(lHigh), math.Exp(lAll)

	// The onset envelope: how much louder than a frame ago, rises only.
	onset := math.Max(0, lLow-t.prevLow) + 0.6*math.Max(0, lAll-t.prevAll) + 0.5*math.Max(0, lHigh-t.prevHigh)
	riseHigh := math.Max(0, lHigh-t.prevHigh)
	silent := eAll < 1e-6
	if silent || t.prevAll == 0 {
		onset, riseHigh = 0, 0
	}
	t.prevLow, t.prevHigh, t.prevAll = lLow, lHigh, lAll

	t.env[t.pos] = onset
	t.pos = (t.pos + 1) % len(t.env)
	t.filled = min(t.filled+1, len(t.env))

	out := Tempo{LogLow: lLow, LogHigh: lHigh, LogAll: lAll}
	if eh := eHigh; eh > t.highPeak {
		t.highPeak = eh
	} else {
		t.highPeak += (eh - t.highPeak) * math.Min(1, dt*0.3)
	}
	snare := riseHigh > snareRise && riseHigh > snareOverAvg*t.highAvg &&
		eHigh > snareLoud*t.highPeak && t.sinceSnare > snareGap
	t.highAvg += (riseHigh - t.highAvg) * math.Min(1, dt*3)
	if snare {
		t.sinceSnare = 0
		out.Snare = true
	}

	if t.sinceUp >= tempoEvery && t.filled >= len(t.env)*2/3 {
		t.sinceUp = 0
		t.update()
	}

	if t.period == 0 {
		return out
	}
	// Run the grid, and beat when it comes round.
	t.sincePeak++
	if t.sincePeak >= t.period {
		t.sincePeak -= t.period
		out.Hit = true
		out.Strength = t.strengthNow()
	}
	out.Phase = math.Min(1, t.sincePeak/t.period)
	out.BPM = 60 * t.fps / t.period
	out.Confidence = t.conf
	return out
}

// at is the envelope k frames ago (0 = this frame), linearly between
// frames for a fractional k.
func (t *TempoTracker) at(k float64) float64 {
	if k < 0 || k > float64(t.filled-1) {
		return 0
	}
	i := int(k)
	f := k - float64(i)
	get := func(j int) float64 {
		if j > t.filled-1 {
			return 0
		}
		return t.env[((t.pos-1-j)%len(t.env)+len(t.env))%len(t.env)]
	}
	return get(i)*(1-f) + get(i+1)*f
}

// update finds the tempo and moves the grid onto the beats.
//
// Onsets are sharp, a frame or so, and a beat seldom lasts a whole number
// of frames (148 bpm at 29 frames a second is 11.7), so the envelope is
// smoothed and the autocorrelation taken at fractional lags; otherwise
// neither 11 nor 12 lines up and two or three beats win. A tempo is scored
// by how well the envelope repeats at every whole number of its beats
// within tempoSpan: the real beat repeats at all of them, half of it or
// two thirds (a syncopated kick) at only some. Where kick and snare take
// turns, the half tempo still scores about as well, so tempos are weighed
// towards 120 bpm, evenly by octave either side: 148 wins over 74, and 90
// over 180. Tuned on a recording of a 148 bpm rock song (see
// TestTempoOnRecording): from 19% of the time right to 89%.
func (t *TempoTracker) update() {
	n := t.filled
	raw := make([]float64, n) // newest first
	var mean float64
	for k := range raw {
		raw[k] = t.at(float64(k))
		mean += raw[k]
	}
	mean /= float64(n)
	x := make([]float64, n) // smoothed, mean taken out
	var power float64
	for k := range x {
		var s, w float64
		for d, c := range [5]float64{1, 4, 6, 4, 1} {
			if j := k + d - 2; j >= 0 && j < n {
				s += c * raw[j]
				w += c
			}
		}
		x[k] = s/w - mean
		power += x[k] * x[k]
	}
	if power < 1e-9 {
		t.conf *= 0.5
		return
	}
	power /= float64(n)

	// Autocorrelation at a fractional lag, normalised: 1 = repeats exactly.
	ac := func(lag float64) float64 {
		i := int(lag)
		f := lag - float64(i)
		var s float64
		cnt := 0
		for k := 0; k+i+1 < n; k++ {
			s += x[k] * (x[k+i]*(1-f) + x[k+i+1]*f)
			cnt++
		}
		if cnt < n/4 {
			return 0
		}
		return s / float64(cnt) / power
	}
	span := tempoSpan * t.fps
	score := func(lag float64) float64 {
		var s, w float64
		for m := 1.0; lag*m <= span; m++ {
			s += ac(lag * m)
			w++
		}
		if w < 2 {
			return math.Inf(-1)
		}
		return s / w
	}
	minLag := 60 * t.fps / tempoMaxBPM
	maxLag := 60 * t.fps / tempoMinBPM
	period, best, bestFit := 0.0, math.Inf(-1), 0.0
	for lag := minLag; lag <= maxLag; lag += 0.25 {
		bpm := 60 * t.fps / lag
		w := math.Exp(-0.5 * math.Pow(math.Log2(bpm/120)/0.6, 2))
		fit := score(lag)
		if s := fit * (0.4 + 0.6*w); s > best {
			period, best, bestFit = lag, s, fit
		}
	}
	if period == 0 || math.IsInf(best, -1) {
		return
	}
	// How sure: how well it repeats over its whole beats, the same measure
	// it was chosen by. (Over one beat alone, a song whose kick and snare
	// take turns looks unsure of a tempo it has right.)
	conf := math.Max(0, math.Min(1, score(period)))

	// A nearby tempo is eased towards. A new one has to show up twice
	// running before it replaces the one in use, and one related to it the
	// way mistakes are (half, double, two thirds from a syncopated kick,
	// three halves) for four seconds.
	near := func(a, b float64) bool { return math.Abs(a-b)/b < 0.06 }
	// And none replaces it unless it fits the music clearly better: the one
	// in use keeps its place while it still fits nearly as well.
	keep := t.period > 0 && !near(period, t.period) && score(t.period) > 0.8*bestFit
	switch {
	case keep:
		t.candidate, t.seen = 0, 0
	case t.period == 0:
		t.period = period
	case near(period, t.period):
		t.period += (period - t.period) * 0.3
		t.candidate, t.seen = 0, 0
	case t.candidate != 0 && near(period, t.candidate):
		t.seen++
		need := 1
		for _, r := range [4]float64{2, 0.5, 1.5, 2.0 / 3} {
			if near(period, r*t.period) {
				need = int(4 / tempoEvery)
			}
		}
		if t.seen >= need {
			t.period, t.candidate, t.seen = period, 0, 0
		}
	default:
		t.candidate, t.seen = period, 0
	}
	t.conf += (conf - t.conf) * 0.5

	// The grid's phase: the offset whose comb of beats, back through the
	// window, lands on the most onset.
	bestOff, bestSum := 0.0, -1.0
	for off := 0.0; off < t.period; off += 0.5 {
		var s float64
		for k := off; k < float64(n-1); k += t.period {
			s += t.at(k)
		}
		if s > bestSum {
			bestOff, bestSum = off, s
		}
	}
	// Ease the grid there, by the shorter way round.
	err := bestOff - t.sincePeak
	err -= t.period * math.Round(err/t.period)
	t.sincePeak += 0.5 * err
	if t.sincePeak < 0 {
		t.sincePeak += t.period
	}
}

// strengthNow is the onset around now against the envelope's mean.
func (t *TempoTracker) strengthNow() float64 {
	var mean float64
	for k := 0; k < t.filled; k++ {
		mean += t.at(float64(k))
	}
	mean /= math.Max(float64(t.filled), 1)
	peak := math.Max(t.at(0), math.Max(t.at(1), t.at(2)))
	if mean <= 0 {
		return 1
	}
	return peak / mean
}
