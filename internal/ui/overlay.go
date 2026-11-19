package ui

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type overlay int

const (
	overlayNone overlay = iota
	overlayHelp
	overlayDevices
)

type devicesMsg struct {
	items []listItem
	err   error
}

func (m Model) loadDevices() tea.Cmd {
	eng := m.eng
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		res, err := eng.Devices(ctx)
		if err != nil {
			return devicesMsg{err: err}
		}
		items := make([]listItem, 0, len(res.Devices))
		for _, d := range res.Devices {
			sub := strings.ToLower(d.Type)
			switch {
			case d.IsSelf && d.IsActive:
				sub += " · this terminal · playing"
			case d.IsSelf:
				sub += " · this terminal"
			case d.IsActive:
				sub += " · playing"
			case d.IsOffline:
				sub += " · offline"
			}
			right := ""
			if d.Volume >= 0 && !d.IsOffline {
				right = itoa(int(d.Volume)) + "%"
			}
			name := d.Name
			if d.IsActive {
				name = "♪ " + name
			}
			items = append(items, listItem{kind: kindTrack, title: name, subtitle: sub, right: right, uri: d.Id})
		}
		return devicesMsg{items: items}
	}
}

func (m *Model) openOverlay(o overlay) tea.Cmd {
	m.closeFilter()
	m.overlay = o
	if o == overlayDevices {
		m.devices.loading = true
		m.devices.err = nil
		return m.loadDevices()
	}
	return nil
}

func (m Model) updateOverlayKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "q", "?", "d":
		if key == "d" && m.overlay != overlayDevices {
			break
		}
		if key == "?" && m.overlay != overlayHelp {
			break
		}
		m.overlay = overlayNone
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	}

	if m.overlay != overlayDevices {
		return m, nil
	}
	l := &m.devices.list
	switch key {
	case "up", "k":
		l.move(-1)
	case "down", "j":
		l.move(1)
	case "R":
		m.devices.loading = true
		return m, m.loadDevices()
	case "enter":
		it := l.selected()
		if it == nil {
			return m, nil
		}
		eng, id, name := m.eng, it.uri, strings.TrimPrefix(it.title, "♪ ")
		m.overlay = overlayNone
		m.setFlash("Moving playback to "+name+"…", false)
		return m, call("Transfer", func(ctx context.Context) error { return eng.Transfer(ctx, id) })
	}
	return m, nil
}

func (m Model) viewOverlay(w, h int) string {
	var card string
	switch m.overlay {
	case overlayHelp:
		card = m.viewHelp(w)
	case overlayDevices:
		card = m.viewDevices(w, h)
	}
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, card)
}

var helpSections = []struct {
	title string
	keys  [][2]string
}{
	{"Pages", [][2]string{
		{"1 … 5", "now playing · library · search · queue · lyrics"},
		{"d", "devices: move playback"},
		{"?", "this help"},
		{"q", "quit"},
	}},
	{"Playback", [][2]string{
		{"space", "play / pause"},
		{"n  p", "next / previous"},
		{"← →  h l", "seek 5 s"},
		{"+ -", "volume"},
		{"s  r", "shuffle · repeat (off → all → track)"},
		{"f", "like / unlike"},
		{"u", "play a Spotify link"},
	}},
	{"Lists", [][2]string{
		{"↑ ↓  j k", "move · g G top/bottom · ctrl+u/d page"},
		{"enter", "open / play"},
		{"a", "add track to queue"},
		{"/", "filter (search: edit query)"},
		{"tab", "next section"},
		{"esc", "back / clear filter"},
		{"R", "reload"},
	}},
	{"Look", [][2]string{
		{"[ ]", "cover pixel size"},
		{"{ }", "cover colours"},
		{"v", "visualizer on/off"},
		{"V", "visualizer style"},
		{"< > 0", "lyrics offset (lyrics page)"},
	}},
}

func (m Model) viewHelp(w int) string {
	keyW := 10
	var cols []string
	for _, sec := range helpSections {
		lines := []string{m.theme.title.Render(sec.title)}
		for _, k := range sec.keys {
			lines = append(lines, m.theme.key.Width(keyW).Render(k[0])+m.theme.text.Render(k[1]))
		}
		cols = append(cols, strings.Join(lines, "\n"))
	}

	// Two columns when wide enough, otherwise one.
	var body string
	if w >= 110 {
		left := lipgloss.JoinVertical(lipgloss.Left, cols[0], "", cols[1])
		right := lipgloss.JoinVertical(lipgloss.Left, cols[2], "", cols[3])
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, "      ", right)
	} else {
		body = strings.Join(cols, "\n\n")
	}
	return m.theme.card.Render(body + "\n\n" + m.theme.dim.Render("esc closes"))
}

func (m Model) viewDevices(w, h int) string {
	cw := clamp(w-16, 40, 70)
	lines := []string{m.theme.title.Render("Spotify Connect devices"), ""}
	switch {
	case m.devices.err != nil:
		lines = append(lines, m.errorLine(m.devices.err))
	case m.devices.loading && len(m.devices.list.items) == 0:
		lines = append(lines, m.loadingLine("Looking for devices…"))
	case len(m.devices.list.items) == 0:
		lines = append(lines, m.theme.dim.Render("No devices found."))
	default:
		lines = append(lines, m.devices.list.view(m.theme, cw, min(len(m.devices.list.items), h-10), ""))
	}
	lines = append(lines, "", m.theme.dim.Render("enter moves playback there · R reload · esc close"))
	return m.theme.card.Render(strings.Join(lines, "\n"))
}
