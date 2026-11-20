package ui

import (
	"slices"
	"time"

	"github.com/Nikkoro/spotui/internal/engine"
)

// frameStats measures how evenly frames go out while the Windows Terminal
// shader is fed: the gaps between effect ticks and how long View takes.
// Every few seconds it writes a line to the debug log (spotui --debug).
type frameStats struct {
	last  time.Time
	since time.Time
	gaps  []time.Duration
	views []time.Duration
}

const frameStatsEvery = 5 * time.Second

func (s *frameStats) tick(now time.Time, eng *engine.Engine) {
	if s == nil {
		return
	}
	if !s.last.IsZero() {
		s.gaps = append(s.gaps, now.Sub(s.last))
	}
	s.last = now
	if s.since.IsZero() {
		s.since = now
	}
	if now.Sub(s.since) < frameStatsEvery || len(s.gaps) == 0 {
		return
	}
	g, v := summary(s.gaps), summary(s.views)
	eng.Debugf("ui: shader frames: %d in %.1fs, gap avg %.1f p95 %.1f max %.1f ms; view avg %.1f p95 %.1f max %.1f ms",
		len(s.gaps), now.Sub(s.since).Seconds(), g[0], g[1], g[2], v[0], v[1], v[2])
	s.gaps, s.views, s.since = s.gaps[:0], s.views[:0], now
}

func (s *frameStats) viewTook(d time.Duration) {
	if s != nil {
		s.views = append(s.views, d)
	}
}

// summary is the average, 95th percentile and maximum, in milliseconds.
func summary(ds []time.Duration) [3]float64 {
	if len(ds) == 0 {
		return [3]float64{}
	}
	sorted := slices.Clone(ds)
	slices.Sort(sorted)
	var sum time.Duration
	for _, d := range sorted {
		sum += d
	}
	ms := func(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
	return [3]float64{ms(sum / time.Duration(len(sorted))), ms(sorted[len(sorted)*95/100]), ms(sorted[len(sorted)-1])}
}
