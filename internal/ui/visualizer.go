package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Nikkoro/spotui/internal/viz"
)

const (
	vizFrameInterval = time.Second / 30
	vizIdleInterval  = 250 * time.Millisecond
)

type vizTickMsg time.Time

func vizTick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return vizTickMsg(t) })
}

func (m Model) vizVisible() bool {
	return m.vizOn && m.phase == phaseReady && m.page == pageNowPlaying && m.track != nil
}

// stepVisualizer advances the analyzer one frame and schedules the next.
// It runs at full rate only while the visualizer is on screen.
func (m *Model) stepVisualizer(now time.Time) tea.Cmd {
	if !m.vizVisible() {
		m.vizAt = time.Time{}
		return vizTick(vizIdleInterval)
	}

	dt := vizFrameInterval.Seconds()
	if !m.vizAt.IsZero() {
		dt = min(now.Sub(m.vizAt).Seconds(), 0.25)
	}
	m.vizAt = now

	// Only analyse while the device is actually pulling audio; otherwise
	// the last buffer would freeze on screen when paused.
	var samples []float32
	total := m.vizTap.Total()
	if total != m.vizSeen {
		m.vizSeen = total
		m.vizStalled = 0
	} else {
		m.vizStalled++
	}
	playing := !m.paused && !m.stopped && m.vizStalled < 6
	if playing {
		delay := m.cfg.Visualizer.DelayMs * viz.SampleRate / 1000
		if m.vizTap.Snapshot(m.vizBuf, delay) {
			samples = m.vizBuf
		}
	}

	m.vizLevels, m.vizPeaks = m.vizAn.Update(samples, dt)

	// The oscilloscope shows ~20 ms of the same window, gain eased.
	if m.vizStyle == "wave" {
		if samples != nil {
			win := samples[len(samples)-882:]
			m.vizWave = append(m.vizWave[:0], win...)
			target := viz.WaveGain(win)
			m.vizWaveGain += (target - m.vizWaveGain) * min(1, dt*4)
		} else {
			m.vizWave = nil
		}
	}
	return vizTick(vizFrameInterval)
}

// viewVisualizer renders the spectrum width columns wide.
func (m Model) viewVisualizer(width, height int) string {
	barW := max(m.cfg.Visualizer.BarWidth, 1)
	bands := viz.BarLayout(width, barW)
	levels, peaks := m.vizLevels, m.vizPeaks
	if m.vizAn.Bands() != bands || len(levels) != bands {
		// Resized: start the new layout from silence, keeping the space.
		m.vizAn.SetBands(bands)
		levels, peaks = make([]float64, bands), make([]float64, bands)
	}
	bottom, top := viz.Gradient(viz.ParseHex(m.theme.accentHex))
	if m.vizStyle == "wave" {
		var win []float32
		if m.vizWave != nil {
			win = m.vizWave
		}
		return viz.RenderWave(win, m.vizWaveGain, width, height, top, bottom)
	}
	if m.vizStyle == "mirror" {
		return viz.RenderMirror(levels, peaks, barW, height, bottom, top)
	}
	return viz.Render(levels, peaks, barW, height, bottom, top)
}
