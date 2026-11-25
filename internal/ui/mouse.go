package ui

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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

	// barText is the last progress bar drawn, as plain text. Now Playing
	// places its bar differently in each layout (and centres the classic
	// card), so View finds it on the finished screen instead (locateBar).
	barText string

	// Synced lyrics on screen: rows from lyrTop, columns lyrX0…lyrX1, and
	// for each row the line it shows (-1: none).
	lyrTop, lyrX0, lyrX1 int
	lyrIdx               []int

	// Up next on Now Playing: two rows per entry from nextTop.
	nextTop, nextX0, nextX1, nextFirst, nextCount int
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
		if m.overlay == overlayNone && m.hits != nil {
			if cmd, ok := m.clickLyricsOrNext(msg.X, msg.Y); ok {
				return m, cmd
			}
		}
		if m.overlay != overlayNone || m.hits == nil {
			return m, nil
		}
		h := *m.hits

		// The progress bar, in the player bar or on Now Playing: click to
		// seek.
		if h.progressW > 0 && msg.Y == h.progressY &&
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

// locateBar finds the progress bar drawn last (hits.barText) on the
// finished screen and records where it is, for clicks to seek. It is how
// Now Playing's bar is found, wherever its layout put it.
func (h *hitMap) locateBar(screen string) {
	if h.barText == "" {
		return
	}
	for y, line := range strings.Split(ansi.Strip(screen), "\n") {
		if i := strings.Index(line, h.barText); i >= 0 {
			h.progressY = y
			h.progressX = ansi.StringWidth(line[:i])
			h.progressW = ansi.StringWidth(h.barText)
			return
		}
	}
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

// clickLyricsOrNext seeks to a clicked lyric line, or plays a clicked Up
// next entry.
func (m *Model) clickLyricsOrNext(x, y int) (tea.Cmd, bool) {
	h := m.hits
	if r := y - h.lyrTop; r >= 0 && r < len(h.lyrIdx) && x >= h.lyrX0 && x < h.lyrX1 && m.lyrics != nil {
		i := h.lyrIdx[r]
		if i < 0 || i >= len(m.lyrics.Synced) {
			return nil, false
		}
		// The line shows at At on the lyrics clock, which runs offset ahead.
		pos := max(int64((m.lyrics.Synced[i].At-m.lyricsOffset)/time.Millisecond), 0)
		m.setPosition(pos)
		eng := m.eng
		return call("Seek", func(ctx context.Context) error { return eng.Seek(ctx, pos, false) }), true
	}
	if r := y - h.nextTop; h.nextCount > 0 && r >= 0 && r < 2*h.nextCount && x >= h.nextX0 && x < h.nextX1 {
		next := m.upNext()
		i := h.nextFirst + r/2
		if i >= len(next) {
			return nil, false
		}
		it := next[i]
		m.setFlash("Playing "+it.title, false)
		eng, uri := m.eng, it.uri
		return call("Skip to track", func(ctx context.Context) error { return eng.SkipTo(ctx, uri) }), true
	}
	return nil, false
}
