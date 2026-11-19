package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const wheelStep = 3

// handleMouse supports the wheel in lists and lyrics, and clicking the
// page tabs in the header.
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.phase != phaseReady || m.inputting || m.filtering {
		return m, nil
	}

	switch msg.Button {
	case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
		d := wheelStep
		if msg.Button == tea.MouseButtonWheelUp {
			d = -d
		}
		switch {
		case m.overlay == overlayDevices:
			m.devices.list.move(d)
		case m.overlay != overlayNone:
		case m.page == pageLyrics:
			m.lyricsScroll = max(m.lyricsScroll+d, 0)
		default:
			if l := m.activeList(); l != nil {
				l.move(d)
			}
		}
		return m, nil

	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}
		if p, ok := m.tabAt(msg.X, msg.Y); ok {
			m.overlay = overlayNone
			return m, m.switchPage(p)
		}
	}
	return m, nil
}

// tabAt maps a click to a header tab, mirroring viewHeader's layout.
func (m Model) tabAt(x, y int) (page, bool) {
	const top, left = 1, 2 // app padding
	if y != top {
		return 0, false
	}
	pos := left + lipgloss.Width(m.theme.pill.Render("♫ spotui")) + 2
	for i, name := range pageNames {
		w := len(itoa(i+1)) + 1 + lipgloss.Width(name)
		if x >= pos && x < pos+w {
			return page(i), true
		}
		pos += w + 3
	}
	return 0, false
}
