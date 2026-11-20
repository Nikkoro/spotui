package ui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const wheelStep = 3

// hitMap records where View put the clickable things.
type hitMap struct {
	bodyTop   int
	listTop   int
	listRows  int
	progressY int
	progressX int
	progressW int
	barRow    int // progress line's row within the player bar, after the rule
}

const doubleClick = 400 * time.Millisecond

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
		case m.page == pageNowPlaying:
			// The wheel is the volume knob on Now Playing.
			step := m.volumeStep()
			if d > 0 {
				step = -step
			}
			eng := m.eng
			return m, call("Volume", func(ctx context.Context) error { return eng.SetVolume(ctx, step, true) })
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
		if m.overlay != overlayNone || m.hits == nil {
			return m, nil
		}
		h := *m.hits

		// Progress bar in the player bar: click to seek.
		if m.page != pageNowPlaying && h.progressW > 0 && msg.Y == h.progressY &&
			msg.X >= h.progressX && msg.X < h.progressX+h.progressW && m.track != nil {
			ratio := float64(msg.X-h.progressX) / float64(max(h.progressW-1, 1))
			pos := int64(ratio * float64(m.track.Duration))
			m.setPosition(pos)
			eng := m.eng
			return m, call("Seek", func(ctx context.Context) error { return eng.Seek(ctx, pos, false) })
		}

		// Lists: click selects, double click opens or plays.
		if l := m.activeList(); l != nil && msg.Y >= h.listTop && msg.Y < h.listTop+h.listRows {
			offset := 0
			if l.scroll != nil {
				offset = *l.scroll
			}
			idx := offset + msg.Y - h.listTop
			if idx >= len(l.visible) || l.items[l.visible[idx]].kind == kindHeader {
				return m, nil
			}
			l.cursor = idx
			if m.lastClick.idx == idx && time.Since(m.lastClick.at) < doubleClick {
				m.lastClick.at = time.Time{}
				return m, m.activate(l.selected())
			}
			m.lastClick.idx, m.lastClick.at = idx, time.Now()
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
