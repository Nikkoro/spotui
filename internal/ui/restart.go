package ui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type restartDueMsg struct{}

// WithRestartAttempt tells the interface how many restarts preceded it, for
// the back-off and the message shown.
func (m Model) WithRestartAttempt(n int) Model {
	m.restartAttempt = n
	return m
}

// WithWarnings queues configuration problems to show once logged in.
func (m Model) WithWarnings(w []string) Model {
	m.warnings = w
	return m
}

// WantsRestart reports whether the interface quit to restart the player.
func (m Model) WantsRestart() bool { return m.restart }

// restartDelay backs off 5 s, 10 s, 20 s … up to a minute.
func (m Model) restartDelay() time.Duration {
	d := 5 * time.Second << min(m.restartAttempt, 4)
	return min(d, time.Minute)
}

func (m *Model) scheduleRestart() tea.Cmd {
	d := m.restartDelay()
	m.restartAt = time.Now().Add(d)
	return tea.Tick(d, func(time.Time) tea.Msg { return restartDueMsg{} })
}

func (m Model) restartNow() (tea.Model, tea.Cmd) {
	m.restart = true
	return m, tea.Quit
}

func (m Model) restartLabel() string {
	left := time.Until(m.restartAt).Round(time.Second)
	if left < 0 {
		left = 0
	}
	return fmt.Sprintf("Reconnecting in %s · R now · q quits", left)
}
