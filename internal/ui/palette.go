package ui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The command palette (ctrl+k or :) runs any action by name and jumps
// straight to playlists, albums and artists from the library.

type paletteCmd struct {
	title string
	group string
	key   string // shown as a hint; also how it runs, when run is nil
	run   func(m Model) (tea.Model, tea.Cmd)
}

type cmdPalette struct {
	input textinput.Model
	list  listModel
	cmds  []paletteCmd
}

func newPalette() cmdPalette {
	in := textinput.New()
	in.Prompt = "  "
	in.Placeholder = "Type a command, playlist, album or artist…"
	in.CharLimit = 120
	p := cmdPalette{input: in}
	p.list.ranked = true
	return p
}

func pressKey(k string) func(m Model) (tea.Model, tea.Cmd) {
	return func(m Model) (tea.Model, tea.Cmd) { return m.handleKey(keyMsg(k)) }
}

func (m Model) paletteCommands() []paletteCmd {
	cmds := []paletteCmd{
		{title: "Now Playing", group: "go to", key: "1"},
		{title: "Library", group: "go to", key: "2"},
		{title: "Search", group: "go to", key: "3"},
		{title: "Queue", group: "go to", key: "4"},
		{title: "Lyrics", group: "go to", key: "5"},
		{title: "Album of this track", group: "go to", key: "A"},
		{title: "Artist of this track", group: "go to", key: "I"},

		{title: "Play / pause", group: "playback", key: "space", run: pressKey(" ")},
		{title: "Next track", group: "playback", key: "n"},
		{title: "Previous track", group: "playback", key: "p"},
		{title: "Shuffle on / off", group: "playback", key: "s"},
		{title: "Repeat: off → all → track", group: "playback", key: "r"},
		{title: "Like / unlike this track", group: "playback", key: "f"},
		{title: "Add to playlist…", group: "playback", key: "P"},
		{title: "Play a Spotify link…", group: "playback", key: "u"},
		{title: "Devices: move playback…", group: "playback", key: "d"},
		{title: "Start DJ / ask DJ", group: "playback", key: "D"},
		{title: "Sleep timer: next step", group: "playback", key: "z"},
		{title: "Sleep timer: off", group: "playback", run: func(m Model) (tea.Model, tea.Cmd) {
			m.sleepStep = len(sleepPresets) + 1 // the next step wraps to off
			return m, m.cycleSleep()
		}},

		{title: "Now Playing layout: next", group: "look", key: "L"},
		{title: "Visualizer on / off", group: "look", key: "v"},
		{title: "Visualizer style: bars / mirror", group: "look", key: "V"},
		{title: "Cover: real image / pixel blocks", group: "look", key: "c"},
		{title: "Cover: more pixels", group: "look", key: "]"},
		{title: "Cover: fewer pixels", group: "look", key: "["},
		{title: "Cover: more colours", group: "look", key: "}"},
		{title: "Cover: fewer colours", group: "look", key: "{"},
		{title: "Accent from cover on / off", group: "look", run: func(m Model) (tea.Model, tea.Cmd) {
			m.dynamicAccent = !m.dynamicAccent
			m.applyAccent()
			return m, nil
		}},

		{title: "Settings", group: "app", key: ","},
		{title: "Copy link to this track", group: "app", key: "y"},
		{title: "Reload this list", group: "app", key: "R"},
		{title: "Help: all keys", group: "app", key: "?"},
		{title: "Quit", group: "app", key: "q"},
	}

	// Whatever page is open can be played as a whole.
	if cp := m.ctxPage; m.page == pageContext && cp != nil {
		uri, title := cp.uri, cp.title
		cmds = append([]paletteCmd{
			{title: "Play " + title, group: "this page", key: "ctrl+p", run: func(m Model) (tea.Model, tea.Cmd) {
				return m, m.playWhole(uri, title, false)
			}},
			{title: "Shuffle " + title, group: "this page", run: func(m Model) (tea.Model, tea.Cmd) {
				return m, m.playWhole(uri, title, true)
			}},
		}, cmds...)
	}
	cmds = append(cmds,
		paletteCmd{title: "Notifications on / off", group: "app", run: func(m Model) (tea.Model, tea.Cmd) {
			m.notifyOff = !m.notifyOff
			if m.notifyOff {
				m.setFlash("Notifications off", false)
			} else {
				m.setFlash("Notifications on", false)
			}
			return m, nil
		}},
		paletteCmd{title: "Lyrics beat effects: off → subtle → party → nightcore", group: "look", run: func(m Model) (tea.Model, tea.Cmd) {
			next := fxLevels[0]
			for i, l := range fxLevels {
				if l == m.fxLevel() {
					next = fxLevels[(i+1)%len(fxLevels)]
				}
			}
			m.cfg.Lyrics.Effects = next
			_ = m.cfg.Set("lyrics", "effects", next)
			m.setFlash("Lyrics beat effects: "+next, false)
			return m, nil
		}},
		paletteCmd{title: "Lyrics line on Now Playing on / off", group: "look", run: func(m Model) (tea.Model, tea.Cmd) {
			m.cfg.Lyrics.NowPlayingLine = !m.cfg.Lyrics.NowPlayingLine
			return m, nil
		}},
	)

	for _, l := range npLayouts {
		l := l
		title := "Now Playing layout: " + l
		if l == m.npChoice {
			title += "  ✓"
		}
		cmds = append(cmds, paletteCmd{title: title, group: "look", run: func(m Model) (tea.Model, tea.Cmd) {
			m.setNpLayout(l)
			return m, m.loadQueueForNp()
		}})
	}

	names := make([]string, 0, len(palettes))
	for name := range palettes {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		name := name
		title := "Theme: " + name
		if name == m.themeName {
			title += "  ✓"
		}
		cmds = append(cmds, paletteCmd{title: title, group: "look", run: func(m Model) (tea.Model, tea.Cmd) {
			m.setTheme(name)
			return m, nil
		}})
	}

	// Jump straight to library entries already loaded.
	for tab, kind := range []string{"playlist", "album", "artist"} {
		for _, it := range m.lib.lists[tab].items {
			it := it
			if it.kind == kindHeader || it.uri == "" {
				continue
			}
			cmds = append(cmds, paletteCmd{title: it.title, group: kind, run: func(m Model) (tea.Model, tea.Cmd) {
				if m.page != pageLibrary && m.page != pageContext {
					m.page = pageLibrary
				}
				return m, m.activate(&it)
			}})
		}
	}
	return cmds
}

func (m *Model) openPalette() tea.Cmd {
	m.closeFilter()
	m.overlay = overlayPalette
	m.pal.cmds = m.paletteCommands()
	m.pal.input.SetValue("")
	m.setPaletteItems("")
	return m.pal.input.Focus()
}

func (m *Model) setPaletteItems(query string) {
	items := make([]listItem, 0, len(m.pal.cmds)+1)
	q := strings.TrimSpace(query)
	if q != "" {
		// Free text can always become a search.
		items = append(items, listItem{kind: kindTrack, title: "Search Spotify for “" + q + "”", subtitle: "search", uri: "palette:search"})
	}
	for i, c := range m.pal.cmds {
		right := ""
		if c.key != "" {
			right = c.key
		}
		items = append(items, listItem{kind: kindTrack, title: c.title, subtitle: c.group, right: right, uri: "palette:" + itoa(i)})
	}
	m.pal.list.setItems(items)
	m.pal.list.setFilter(q)
	if q != "" {
		// Keep the search entry visible at the top regardless of the filter.
		rest := m.pal.list.visible[:0:0]
		for _, v := range m.pal.list.visible {
			if v != 0 {
				rest = append(rest, v)
			}
		}
		m.pal.list.visible = append([]int{0}, rest...)
		m.pal.list.cursor = 0
		if len(m.pal.list.visible) > 1 {
			m.pal.list.cursor = 1 // best match first, search second
		}
	}
}

func (m Model) updatePaletteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+k":
		m.overlay = overlayNone
		m.pal.input.Blur()
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "up", "ctrl+p":
		m.pal.list.move(-1)
		return m, nil
	case "down", "ctrl+n", "tab":
		m.pal.list.move(1)
		return m, nil
	case "enter":
		it := m.pal.list.selected()
		m.overlay = overlayNone
		m.pal.input.Blur()
		if it == nil {
			return m, nil
		}
		if it.uri == "palette:search" {
			q := strings.TrimSpace(m.pal.input.Value())
			m.page = pageSearch
			m.search.input.SetValue(q)
			m.search.query = q
			m.search.loading = true
			m.search.err = nil
			m.rememberSearch(q)
			return m, m.runSearch(q)
		}
		idx := 0
		for _, r := range strings.TrimPrefix(it.uri, "palette:") {
			idx = idx*10 + int(r-'0')
		}
		if idx < 0 || idx >= len(m.pal.cmds) {
			return m, nil
		}
		c := m.pal.cmds[idx]
		if c.run != nil {
			return c.run(m)
		}
		return m.handleKey(keyMsg(c.key))
	}
	var cmd tea.Cmd
	m.pal.input, cmd = m.pal.input.Update(msg)
	m.setPaletteItems(m.pal.input.Value())
	return m, cmd
}

func (m Model) viewPalette(w, h int) string {
	cw := clamp(w-16, 44, 80)
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(m.theme.accent).Width(cw - 2)
	in := m.pal.input
	in.Width = cw - 8
	listH := clamp(h-10, 4, 16)
	lines := []string{
		box.Render(m.theme.accentTxt.Render("⌘") + in.View()),
		"",
		m.pal.list.view(m.theme, cw, listH, ""),
		"",
		m.theme.dim.Render("↑↓ choose · enter run · esc close"),
	}
	return m.theme.card.Padding(1, 2).Render(strings.Join(lines, "\n"))
}
