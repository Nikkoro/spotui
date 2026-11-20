package ui

import (
	"math"

	"github.com/Nikkoro/spotui/internal/viz"
)

// beatSource gives the beats every effect follows, the lyric line's and the
// shader's: the tempo tracker's grid once it is sure of the tempo, and the
// plain beat detector before that. Drops always come from the detector.
type beatSource struct {
	detector *viz.BeatDetector
	tempo    *viz.TempoTracker
	onGrid   bool // the grid is in use
}

// The grid is used once the tracker is this sure, and until it drops under
// the second: a song whose sureness wavers around one threshold would flip
// between the two, and the plain detector's tempo is often far off.
const (
	gridOn  = 0.15
	gridOff = 0.08
)

// feed analyses the newest samples, covering dt seconds. It returns the
// detector's verdict (raw), the beat to follow and the tracker's verdict.
func (s *beatSource) feed(newest []float32, dt float64) (raw, beat viz.Beat, t viz.Tempo) {
	if s.detector == nil {
		s.detector, s.tempo = viz.NewBeatDetector(), viz.NewTempoTracker()
	}
	raw = s.detector.Feed(newest, dt*1000)
	t = s.tempo.Feed(newest, dt)
	switch {
	case t.BPM > 0 && t.Confidence >= gridOn:
		s.onGrid = true
	case t.BPM == 0 || t.Confidence < gridOff:
		s.onGrid = false
	}
	if !s.onGrid {
		return raw, raw, t
	}
	// The grid's beat, its strength put on the detector's scale (1.45 just
	// a beat … about 3 a hard one).
	beat = viz.Beat{Hit: t.Hit, Drop: raw.Drop}
	if t.Hit {
		beat.Strength = 1.45 + 1.5*math.Max(0, math.Min(1, (t.Strength-2)/4))
	}
	if raw.Drop && !t.Hit {
		beat.Hit, beat.Strength = true, 3
	}
	return raw, beat, t
}
