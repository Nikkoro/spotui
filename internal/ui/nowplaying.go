package ui

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Nikkoro/spotui/internal/lyrics"
	"github.com/Nikkoro/spotui/internal/termimg"
)

// Now Playing layouts. All of them are built from the same pieces (cover,
// track info, progress, lyrics, visualizer, up next) and fill the whole
// body; "auto" picks one by the terminal's size. A layout that does not fit
// the window falls back to a smaller one until the window grows again.

const (
	npAuto    = "auto"
	npSplit   = "split"   // cover and info left, lyrics right
	npPanels  = "panels"  // cover and info, lyrics, up next
	npCompact = "compact" // small cover, three lyric lines, for small windows
	npClassic = "classic" // the centred card
)

// npLayouts is the order L cycles through.
var npLayouts = []string{npAuto, npSplit, npPanels, npCompact, npClassic}

func validNpLayout(s string) bool {
	for _, l := range npLayouts {
		if l == s {
			return true
		}
	}
	return false
}

// Smallest bodies (columns, rows) each layout needs.
const (
	panelsMinW, panelsMinH = 150, 28
	splitMinW, splitMinH   = 90, 22
)

// npLayout resolves the chosen layout for the current body size.
func (m Model) npLayout() string {
	w, h := m.width-4, m.height-6 // the body, roughly: padding, header, footer
	choice := m.npChoice
	if choice == "" {
		choice = npAuto
	}
	fits := func(l string) bool {
		switch l {
		case npPanels:
			return w >= panelsMinW && h >= panelsMinH
		case npSplit:
			return w >= splitMinW && h >= splitMinH
		}
		return true
	}
	switch choice {
	case npClassic:
		return npClassic
	case npAuto, npPanels:
		if fits(npPanels) {
			return npPanels
		}
		fallthrough
	case npSplit:
		if fits(npSplit) {
			return npSplit
		}
	}
	return npCompact
}

// cycleNpLayout moves to the next layout and says which is showing.
func (m *Model) cycleNpLayout() {
	cur := m.npChoice
	if cur == "" {
		cur = npAuto
	}
	next := npLayouts[0]
	for i, l := range npLayouts {
		if l == cur {
			next = npLayouts[(i+1)%len(npLayouts)]
			break
		}
	}
	m.setNpLayout(next)
}

func (m *Model) setNpLayout(l string) {
	m.npChoice = l
	msg := "Now Playing layout: " + l
	switch shown := m.npLayout(); {
	case l == npAuto:
		msg += " (" + shown + ")"
	case shown != l:
		msg += " · window too small, showing " + shown
	}
	m.setFlash(msg, false)
	if m.img != nil {
		m.img.Invalidate()
	}
}

func (m Model) viewNowPlayingLayout(w, h int) string {
	switch m.npLayout() {
	case npPanels:
		return m.npPanelsView(w, h)
	case npSplit:
		return m.npSplitView(w, h)
	}
	return m.npCompactView(w, h)
}

// npVizHeight is how many rows the visualizer gets out of h.
func (m Model) npVizHeight(h int) int {
	if !m.vizVisible() {
		return 0
	}
	return clamp(h/5, 3, 8)
}

// npCover renders the cover to fit maxCols×maxRows cells: the real image
// (placed by the terminal writer) or pixel blocks. Returns the block and
// its size in cells.
func (m Model) npCover(maxCols, maxRows int) (string, int, int) {
	if !m.cfg.Cover.Enabled || m.track == nil || maxCols < 6 || maxRows < 3 {
		return "", 0, 0
	}
	if size := m.cfg.Cover.Size; size > 0 {
		maxCols = min(maxCols, size)
	}
	if m.imagesOn() {
		cw, ch := max(m.caps.CellW, 1), max(m.caps.CellH, 1)
		cols := min(maxCols, maxRows*ch/cw)
		rows := max(1, (cols*cw+ch/2)/ch)
		if rows > maxRows {
			rows = maxRows
			cols = rows * ch / cw
		}
		if cols < 6 {
			return "", 0, 0
		}
		img := m.coverImage(cols, rows)
		if img == nil {
			block := lipgloss.NewStyle().Width(cols).Height(rows).Render(m.coverPlaceholder(rows * 2))
			return block, cols, rows
		}
		lines := make([]string, rows)
		blank := strings.Repeat(" ", cols)
		for i := range lines {
			lines[i] = blank
		}
		lines[0] = termimg.Marker + blank
		if m.frame != nil {
			m.frame.tmpl = img
		}
		return strings.Join(lines, "\n"), cols, rows
	}
	// Pixel blocks: one column and two pixels per cell, at most the chosen
	// pixelation so it stays as coarse as the user set it.
	size := min(maxCols, maxRows*2, m.pixSize)
	size -= size % 2
	if size < 8 {
		return "", 0, 0
	}
	return m.coverView(size), size, size / 2
}

// npInfoColumn is the cover over the track info and progress, w wide and h
// tall.
func (m Model) npInfoColumn(w, h int) string {
	static := m.viewTrackStatic(w, true)
	dynamic := m.trackDynamic(w, false)
	infoH := lipgloss.Height(static) + 1 + lipgloss.Height(dynamic)

	parts := []string{}
	if block, _, _ := m.npCover(w, h-infoH-1); block != "" {
		parts = append(parts, block, "")
	}
	parts = append(parts, static, "", dynamic)
	return lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).
		Render(lipgloss.JoinVertical(lipgloss.Left, parts...))
}

// npLyricsPanel is a small heading over the lyrics.
func (m Model) npLyricsPanel(w, h int) string {
	head := m.theme.dim.Bold(true).Render("LYRICS")
	if m.npFocused(npFocusLyrics) {
		head = m.theme.accentTxt.Bold(true).Render("▌LYRICS") + m.theme.dim.Render("  < > offset · 0 reset · esc")
	}
	if info := m.lyricsInfo(); info != "" {
		head += m.theme.dim.Render(" · " + info)
	}
	head = ansi.Truncate(head, w, "…")
	return lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).
		Render(lipgloss.JoinVertical(lipgloss.Left, head, "", m.lyricsBody(w, max(h-2, 1), true)))
}

// vrule is a vertical line h rows tall with a space either side.
func vrule(h int) string {
	line := lipgloss.NewStyle().Foreground(colTrack).Render("│")
	rows := make([]string, h)
	for i := range rows {
		rows[i] = " " + line + " "
	}
	return strings.Join(rows, "\n")
}

// withViz puts the visualizer under main when it is on.
func (m Model) withViz(main string, w, vizH int) string {
	if vizH <= 0 {
		return main
	}
	return lipgloss.JoinVertical(lipgloss.Left, main, "", m.viewVisualizer(w, vizH))
}

func (m Model) npSplitView(w, h int) string {
	vizH := m.npVizHeight(h)
	mainH := h
	if vizH > 0 {
		mainH -= vizH + 1
	}
	leftW := clamp(w*2/5, 28, 64)
	rightW := w - leftW - 3
	main := lipgloss.JoinHorizontal(lipgloss.Top,
		m.npInfoColumn(leftW, mainH), vrule(mainH), m.npLyricsPanel(rightW, mainH))
	return m.withViz(main, w, vizH)
}

func (m Model) npPanelsView(w, h int) string {
	vizH := m.npVizHeight(h)
	mainH := h
	if vizH > 0 {
		mainH -= vizH + 1
	}
	leftW := clamp(w/4, 30, 48)
	rightW := clamp(w/4, 30, 44)
	midW := w - leftW - rightW - 6
	main := lipgloss.JoinHorizontal(lipgloss.Top,
		m.npInfoColumn(leftW, mainH), vrule(mainH),
		m.npLyricsPanel(midW, mainH), vrule(mainH),
		m.npSidePanel(rightW, mainH))
	return m.withViz(main, w, vizH)
}

// npSidePanel is what plays next, then where it plays and DJ.
func (m Model) npSidePanel(w, h int) string {
	title := func(s string) string { return m.theme.dim.Bold(true).Render(s) }
	var lines []string
	focused := m.npFocused(npFocusNext)
	if focused {
		lines = append(lines, m.theme.accentTxt.Bold(true).Render("▌UP NEXT")+m.theme.dim.Render("  enter play · esc"), "")
	} else {
		lines = append(lines, title("UP NEXT"), "")
	}
	next := m.upNext()
	room := max((h-9)/2, 1)
	switch {
	case len(next) == 0 && m.queue.loading:
		lines = append(lines, m.theme.dim.Render("…"))
	case len(next) == 0:
		lines = append(lines, m.theme.dim.Render("Nothing queued"))
	}
	start := 0
	if focused {
		start = clamp(m.npNextCursor-room+1, 0, max(len(next)-room, 0))
	}
	for i := start; i < len(next); i++ {
		it := next[i]
		if i-start >= room {
			lines = append(lines, m.theme.dim.Render(itoa(len(next)-i)+" more · 4 queue"))
			break
		}
		name := m.theme.text.Render(it.title)
		if focused && i == m.npNextCursor {
			name = m.theme.accentTxt.Render("› ") + m.theme.title.Render(it.title)
		}
		lines = append(lines,
			ansi.Truncate(name, w, "…"),
			ansi.Truncate(m.theme.dim.Render(it.subtitle), w, "…"))
	}

	lines = append(lines, "", title("DEVICE"),
		ansi.Truncate(m.theme.text.Render(m.cfg.DeviceName), w, "…")+"  "+m.theme.dim.Render("d switch"))
	dj := "D start DJ"
	if m.playingDj() {
		dj = "D ask DJ for something"
	}
	lines = append(lines, "", title("DJ"), m.theme.dim.Render(dj))

	return lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).
		Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// upNext lists the queue's coming tracks, as last loaded.
func (m Model) upNext() []listItem {
	var out []listItem
	in := false
	for _, it := range m.queue.list.items {
		if it.kind == kindHeader {
			if it.title != "" {
				in = it.title == "Next up"
			}
			continue
		}
		if in {
			out = append(out, it)
		}
	}
	return out
}

// npCompactView suits small windows: a small cover beside the track, the
// progress under it, three lyric lines and the visualizer in what is left,
// which shrinks before it goes.
func (m Model) npCompactView(w, h int) string {
	dynamic := m.compactDynamic(w)
	dynH := lipgloss.Height(dynamic)

	lyricLines := m.lyrics != nil && len(m.lyrics.Synced) > 0 && m.lyr != nil
	coverRows := clamp(h/3, 3, 8)

	layout := func(coverRows int, withLyrics bool) (top string, vizH int) {
		block, cols, _ := m.npCover(max(w/3, 6), coverRows)
		infoW := w
		if block != "" {
			infoW = w - cols - 3
		}
		static := m.viewTrackStatic(infoW, true)
		if block != "" {
			top = lipgloss.JoinHorizontal(lipgloss.Top, block, "   ", static)
		} else {
			top = static
		}
		used := lipgloss.Height(top) + 1 + dynH
		if withLyrics {
			used += 4
		}
		return top, h - used - 1
	}

	// Short of room, the cover shrinks first, then the lyric lines go; the
	// visualizer keeps at least three rows while there is any room at all.
	top, vizH := layout(coverRows, lyricLines)
	for vizH < 3 && coverRows > 3 {
		coverRows--
		top, vizH = layout(coverRows, lyricLines)
	}
	if vizH < 3 && lyricLines {
		lyricLines = false
		top, vizH = layout(coverRows, false)
	}

	parts := []string{top, "", dynamic}
	if lyricLines {
		parts = append(parts, "", m.npLyricLines(w))
	}
	main := lipgloss.JoinVertical(lipgloss.Left, parts...)
	if !m.vizVisible() || vizH < 3 {
		return main
	}
	return lipgloss.JoinVertical(lipgloss.Left, main, "", m.viewVisualizer(w, min(vizH, 10)))
}

// compactDynamic is the progress on one line and the modes and volume on
// the next.
func (m Model) compactDynamic(w int) string {
	t := m.track
	pos, dur := m.position(), int64(t.Duration)
	times := fmtDuration(pos) + " / " + fmtDuration(dur)
	icon := m.stateIcon()
	barW := max(w-lipgloss.Width(icon)-len(times)-4, 5)
	line1 := icon + "  " + m.progressBar(barW, pos, dur) + "  " + m.theme.dim.Render(times)
	line2 := m.modeBadges() + m.theme.dim.Render("   ") + m.volumeBar(min(w-24, 30))
	return line1 + "\n" + ansi.Truncate(line2, w, "…")
}

// npLyricLines is the line before, the one being sung and the next.
func (m Model) npLyricLines(w int) string {
	lines := m.lyrics.Synced
	cur := lyrics.Index(lines, m.lyricsPos())
	get := func(i int) string {
		if i < 0 || i >= len(lines) {
			return ""
		}
		if lines[i].Text == "" {
			return "♪"
		}
		return lines[i].Text
	}
	prev := m.theme.dim.Render(ansi.Truncate(get(cur-1), w-2, "…"))
	now := get(cur)
	if now == "" {
		now = "♪"
	}
	curLine := m.theme.accentTxt.Render("› ") + m.theme.title.Bold(true).Render(ansi.Truncate(now, w-4, "…"))
	next := m.theme.text.Render(ansi.Truncate(get(cur+1), w-2, "…"))
	return lipgloss.JoinVertical(lipgloss.Left, "  "+prev, curLine, "  "+next)
}

// loadQueueForNp refreshes the queue when Now Playing shows what is next.
// It asks the embedded player, not Spotify.
func (m *Model) loadQueueForNp() tea.Cmd {
	if m.page != pageNowPlaying || m.npLayout() != npPanels || m.eng == nil {
		return nil
	}
	m.queue.loading = true
	return m.loadQueue()
}

// Sections of Now Playing that tab moves between.
const (
	npFocusNone = iota
	npFocusLyrics
	npFocusNext
)

// npFocusable lists the sections the current layout shows, after none.
func (m Model) npFocusable() []int {
	out := []int{npFocusNone}
	switch m.npLayout() {
	case npSplit, npCompact:
		out = append(out, npFocusLyrics)
	case npPanels:
		out = append(out, npFocusLyrics, npFocusNext)
	}
	return out
}

// focused reports whether section f has the focus in the current layout.
func (m Model) npFocused(f int) bool {
	if m.npFocus != f {
		return false
	}
	for _, x := range m.npFocusable() {
		if x == f {
			return true
		}
	}
	return false
}

// handleNpKey moves between Now Playing's sections and handles keys for the
// one in focus. The lyric offset keys work whatever has the focus.
func (m Model) handleNpKey(key string) (bool, tea.Model, tea.Cmd) {
	switch key {
	case "tab", "shift+tab":
		order := m.npFocusable()
		if len(order) < 2 {
			return false, m, nil
		}
		idx := 0
		for i, f := range order {
			if f == m.npFocus {
				idx = i
			}
		}
		step := 1
		if key == "shift+tab" {
			step = len(order) - 1
		}
		m.npFocus = order[(idx+step)%len(order)]
		m.npNextCursor = 0
		return true, m, nil
	case "<", ",", ">", ".", "0":
		if m.track != nil && m.npLayout() != npClassic {
			return m.handleLyricsKey(key)
		}
	case "esc":
		if m.npFocus != npFocusNone {
			m.npFocus = npFocusNone
			return true, m, nil
		}
	}

	switch {
	case m.npFocused(npFocusLyrics):
		switch key {
		case "up", "k", "down", "j":
			return m.handleLyricsKey(key) // scrolls lyrics that are not synced
		}
	case m.npFocused(npFocusNext):
		next := m.upNext()
		switch key {
		case "up", "k":
			m.npNextCursor = max(m.npNextCursor-1, 0)
			return true, m, nil
		case "down", "j":
			m.npNextCursor = clamp(m.npNextCursor+1, 0, max(len(next)-1, 0))
			return true, m, nil
		case "enter":
			if m.npNextCursor >= len(next) {
				return true, m, nil
			}
			it := next[m.npNextCursor]
			m.npNextCursor = 0
			m.setFlash("Playing "+it.title, false)
			eng, uri := m.eng, it.uri
			return true, m, call("Skip to track", func(ctx context.Context) error { return eng.SkipTo(ctx, uri) })
		}
	}
	return false, m, nil
}
