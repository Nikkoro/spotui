package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (m Model) viewPage(w, h int) string {
	switch m.page {
	case pageLibrary:
		return m.viewLibrary(w, h)
	case pageSearch:
		return m.viewSearch(w, h)
	case pageQueue:
		return m.viewQueue(w, h)
	case pageContext:
		return m.viewContext(w, h)
	case pageLyrics:
		return m.viewLyrics(w, h)
	}
	return ""
}

// tabBar renders section tabs, the active one underlined in the accent.
func (m Model) tabBar(names []string, active int, counts []int) string {
	parts := make([]string, len(names))
	for i, n := range names {
		label := n
		if counts != nil && counts[i] > 0 {
			label += " " + itoa(counts[i])
		}
		if i == active {
			parts[i] = m.theme.title.Underline(true).Render(label)
		} else {
			parts[i] = m.theme.dim.Render(label)
		}
	}
	return strings.Join(parts, "    ")
}

// listBlock renders an optional filter line plus the list itself. above is
// how many lines of the page precede it, for mouse hit-testing.
func (m Model) listBlock(l *listModel, w, h, above int) string {
	var lines []string
	if m.filtering {
		lines = append(lines, m.filterIn.View())
	} else if l.filter != "" {
		lines = append(lines, m.theme.dim.Render("/ ")+m.theme.accentTxt.Render(l.filter)+
			m.theme.dim.Render("   esc clears"))
	}
	listH := h - len(lines)
	if m.hits != nil {
		m.hits.listTop = m.hits.bodyTop + above + len(lines)
		m.hits.listRows = max(listH, 0)
	}
	l.liked = m.liked
	lines = append(lines, l.view(m.theme, w, listH, m.currentURI()))
	return strings.Join(lines, "\n")
}

func (m Model) pageTitle(title, right string, w int) string {
	t := m.theme.title.Render(title)
	r := m.theme.dim.Render(right)
	gap := w - lipgloss.Width(t) - lipgloss.Width(r)
	if gap < 1 {
		return t
	}
	return t + strings.Repeat(" ", gap) + r
}

func (m Model) loadingLine(text string) string {
	return m.spinner.View() + m.theme.dim.Render(" "+text)
}

func (m Model) errorLine(err error) string {
	return m.theme.errorTxt.Render("✕ " + err.Error())
}

// --- library ---

func (m Model) viewLibrary(w, h int) string {
	l := &m.lib.lists[m.lib.tab]
	right := l.position()
	if m.lib.loading[m.lib.tab] && len(l.items) > 0 {
		right = m.spinner.View() + " refreshing   " + right
	}
	head := m.pageTitle("Your Library", right, w)
	tabs := m.tabBar(libTabs, m.lib.tab, nil)

	var body string
	switch {
	case m.webAuthURL != "" && m.pendingLibTab == m.lib.tab:
		body = m.webAuthCard(w)
	case m.lib.err[m.lib.tab] != nil && len(l.items) > 0:
		// Keep showing the cached copy; just say it may be stale.
		body = m.listBlock(l, w, h-5, 3) + "\n" + m.theme.dim.Render("offline copy · "+m.lib.err[m.lib.tab].Error()+" · R retries")
	case m.lib.err[m.lib.tab] != nil:
		body = m.errorLine(m.lib.err[m.lib.tab]) + "\n" + m.theme.dim.Render("press R to retry")
	case m.lib.loading[m.lib.tab] && len(l.items) == 0:
		body = m.loadingLine("Loading " + strings.ToLower(libTabs[m.lib.tab]) + "…")
	default:
		body = m.listBlock(l, w, h-4, 3)
	}
	return lipgloss.JoinVertical(lipgloss.Left, head, tabs, "", body)
}

// --- search ---

func (m Model) viewSearch(w, h int) string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colTrack).
		Width(min(w-2, 70))
	if m.search.input.Focused() {
		box = box.BorderForeground(m.theme.accent)
	}
	input := m.search.input
	input.Width = min(w-8, 64)
	searchBox := box.Render(m.theme.accentTxt.Render("⌕") + input.View())

	lines := []string{searchBox}
	rest := h - lipgloss.Height(searchBox)

	switch {
	case m.webAuthURL != "":
		lines = append(lines, "", m.webAuthCard(w))
	case m.search.loading:
		lines = append(lines, "", m.loadingLine("Searching for “"+m.search.query+"”…"))
	case m.search.err != nil:
		lines = append(lines, "", m.errorLine(m.search.err))
		if m.pkce == nil {
			lines = append(lines, m.theme.dim.Render(
				"Tip: set client_id in config.toml to use your own Spotify app and avoid shared rate limits."))
		}
	case m.search.query == "":
		lines = append(lines, "", m.theme.dim.Render("Type and press enter. / focuses the search box again."))
	default:
		counts := make([]int, len(searchTabs))
		for i := range m.search.lists {
			counts[i] = len(m.search.lists[i].items)
		}
		l := &m.search.lists[m.search.tab]
		lines = append(lines, "", m.tabBar(searchTabs, m.search.tab, counts), "",
			m.listBlock(l, w, rest-3, lipgloss.Height(searchBox)+3))
	}
	return strings.Join(lines, "\n")
}

// --- queue ---

func (m Model) viewQueue(w, h int) string {
	l := &m.queue.list
	head := m.pageTitle("Queue", l.position(), w)

	var body string
	switch {
	case m.queue.err != nil:
		body = m.errorLine(m.queue.err)
	case m.queue.loading && len(l.items) == 0:
		body = m.loadingLine("Loading queue…")
	case len(l.items) == 0:
		body = m.theme.dim.Render("The queue is empty. Press a on any track to add it.")
	default:
		body = m.listBlock(l, w, h-2, 2)
	}
	return lipgloss.JoinVertical(lipgloss.Left, head, "", body)
}

// --- context (playlist / album / artist) ---

func (m Model) viewContext(w, h int) string {
	cp := m.ctxPage
	if cp == nil {
		return ""
	}

	pill := m.theme.pill.Render(strings.ToUpper(cp.kind))
	art := ""
	if cp.cover != nil {
		art = cp.cover.Render()
	}
	// A real image where the terminal shows them: as tall as the details
	// beside it.
	if m.imagesOn() && cp.img != nil {
		const rows = 6
		cols := max(rows*max(m.caps.CellH, 1)/max(m.caps.CellW, 1), 4)
		if img := m.imageOf(cp.imgURL, cp.img, cols, rows); img != nil {
			lines := make([]string, rows)
			blank := strings.Repeat(" ", cols)
			for i := range lines {
				lines[i] = blank
			}
			lines[0] = m.placeImage(img) + blank
			art = strings.Join(lines, "\n")
		}
	}
	textW := w - lipgloss.Width(art) - 3
	title := m.theme.title.Render(ansi.Truncate(cp.title, max(textW-12, 10), "…"))

	info := ""
	switch {
	case cp.length > 0 && isShow(cp.uri):
		// Episodes come a page at a time, as the cursor gets to the end.
		info = plural(cp.length, "episode")
		if n := len(cp.list.items); n < cp.length {
			info = itoa(n) + " of " + info
		}
		if cp.loading {
			info = m.spinner.View() + " " + info
		}
	case cp.length > 0 && cp.uri == yourEpisodesURI(m.username):
		info = plural(cp.length, "episode")
		if cp.loading && cp.cached < cp.length {
			info = m.spinner.View() + " " + itoa(cp.cached) + "/" + info
		}
	case cp.length > 0:
		info = plural(cp.length, "track")
		if cp.loading && cp.cached < cp.length {
			info = m.spinner.View() + " " + itoa(cp.cached) + "/" + info
		}
	case cp.loading:
		info = m.spinner.View() + " loading"
	}
	if pos := cp.list.position(); pos != "" {
		info += "   " + pos
	}
	playing := ""
	if m.contextURI == cp.uri {
		playing = m.theme.accentTxt.Render("♪ playing from here")
	}

	var header string
	if art != "" {
		// Cover on the left, details stacked beside it.
		lines := []string{pill, title}
		if cp.subtitle != "" {
			lines = append(lines, m.theme.album.Render(ansi.Truncate(cp.subtitle, textW, "…")))
		}
		if d := plainDescription(cp.desc); d != "" {
			lines = append(lines, m.theme.dim.Render(ansi.Truncate(d, textW, "…")))
		}
		lines = append(lines, "", m.theme.dim.Render(info), playing)
		header = lipgloss.JoinHorizontal(lipgloss.Top, art, "   ", lipgloss.JoinVertical(lipgloss.Left, lines...))
	} else {
		head := pill + " " + title
		gap := w - lipgloss.Width(head) - lipgloss.Width(info)
		if gap > 0 {
			head += strings.Repeat(" ", gap) + m.theme.dim.Render(info)
		}
		header = head + "\n" + playing
		if d := plainDescription(cp.desc); d != "" {
			header = head + "\n" + m.theme.dim.Render(ansi.Truncate(d, w, "…")) + "\n" + playing
		}
	}
	above := lipgloss.Height(header) + 1

	var body string
	switch {
	case cp.err != nil:
		body = m.errorLine(cp.err) + "\n" + m.theme.dim.Render("press R to retry, esc to go back")
	case len(cp.list.items) == 0 && cp.loading:
		body = m.loadingLine("Loading tracks…")
	default:
		body = m.listBlock(&cp.list, w, h-above, above)
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, "", body)
}

// webAuthCard asks the user to approve spotui's Web API access.
func (m Model) webAuthCard(w int) string {
	return strings.Join([]string{
		m.theme.title.Render("Connect the Web API"),
		m.theme.text.Render("Approve spotui in the browser window that just opened."),
		m.theme.dim.Render("Nothing opened? Press o, or open:"),
		m.theme.link.Render(ansi.Truncate(m.webAuthURL, w, "…")),
		"",
		m.loadingLine("Waiting for approval…"),
	}, "\n")
}
