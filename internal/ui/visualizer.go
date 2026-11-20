package ui

import (
	"os"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Nikkoro/spotui/internal/viz"
)

const (
	vizFrameInterval = time.Second / 30
	// shaderFrameInterval is how often the Windows Terminal shader is sent
	// new values while it is on.
	shaderFrameInterval = time.Second / 60
	vizIdleInterval     = 250 * time.Millisecond
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
	if !m.vizVisible() && !m.fxActive() && !m.shaderActive() {
		m.vizAt = time.Time{}
		m.stepFx(nil, vizIdleInterval.Seconds())
		return vizTick(vizIdleInterval)
	}

	dt := vizFrameInterval.Seconds()
	if !m.vizAt.IsZero() {
		dt = min(now.Sub(m.vizAt).Seconds(), 0.25)
	}
	m.vizAt = now

	// The shader is fed at twice the rate, so what it is sent changes in
	// smaller steps; the analysis below keeps 30 frames a second, which
	// the beat detector is tuned for.
	next := vizFrameInterval
	if m.shaderActive() {
		next = shaderFrameInterval
		m.frames.tick(now, m.eng)
		m.fx.shader.step(dt)
		m.fx.shader.logValues(dt, m.lyricsPos(), m.eng)
		m.vizAcc += dt
		if m.vizAcc < vizFrameInterval.Seconds()*0.9 {
			return vizTick(next)
		}
		dt, m.vizAcc = min(m.vizAcc, 0.25), 0
	}

	// Only analyse while the device is actually pulling audio; otherwise
	// the last buffer would freeze on screen when paused.
	var samples []float32
	if !m.paused && !m.stopped && m.vizTap.Active() {
		delay := m.vizDelayMs() * viz.SampleRate / 1000
		if m.vizTap.Snapshot(m.vizBuf, delay) {
			samples = m.vizBuf
		}
	}

	m.stepFx(samples, dt)
	if !m.vizVisible() {
		return vizTick(next) // effects only
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
	return vizTick(next)
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

// vizDelayMs is how far the analysed audio is held back to match what is
// heard: the configured delay, or with -1 (auto) the output's buffer.
func (m Model) vizDelayMs() int {
	if d := m.cfg.Visualizer.DelayMs; d >= 0 {
		return d
	}
	switch m.cfg.AudioBackend {
	case "pulseaudio":
		if v, err := strconv.Atoi(os.Getenv("PULSE_LATENCY_MSEC")); err == nil && v > 0 {
			return min(max(v, 20), 2000)
		}
		return 200 // what the PulseAudio output asks for
	case "wasapi":
		return 100
	}
	return 120
}
