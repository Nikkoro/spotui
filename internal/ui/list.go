package ui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type itemKind int

const (
	kindTrack itemKind = iota
	kindAlbum
	kindArtist
	kindPlaylist
	kindDj
	kindHeader // non-selectable section label
)

// listItem is one row of a list.
type listItem struct {
	kind     itemKind
	title    string
	subtitle string
	right    string
	uri      string
	// context is the context to play a track in, empty for a lone track.
	context string
	// extra info used by specific pages.
	imageURL string
	editable bool   // a playlist the user can change the tracks of
	owned    bool   // a playlist the user owns: rename, describe, delete
	desc     string // a playlist's description
}

// listModel is a scrollable, filterable list with a cursor.
type listModel struct {
	items    []listItem
	visible  []int // indices into items after filtering
	cursor   int   // index into visible
	scroll   *int  // first visible row; shared so View (on a copy) can keep it
	filter   string
	numbered bool
	// ranked orders filter matches by quality instead of list order.
	ranked bool
	// liked marks tracks in Liked Songs with a heart; set while rendering.
	liked map[string]bool
}

func (l *listModel) setItems(items []listItem) {
	if l.scroll == nil {
		l.scroll = new(int)
	}
	sel := ""
	if it := l.selected(); it != nil {
		sel = it.uri
	}
	l.items = items
	l.applyFilter()
	// Keep the cursor on the same entry when the list is refreshed.
	if sel != "" {
		for i, idx := range l.visible {
			if l.items[idx].uri == sel {
				l.cursor = i
				break
			}
		}
	}
	l.clampCursor()
}

func (l *listModel) setFilter(f string) {
	l.filter = f
	l.applyFilter()
	l.cursor = 0
	if l.scroll != nil {
		*l.scroll = 0
	}
	l.clampCursor()
}

func (l *listModel) applyFilter() {
	l.visible = l.visible[:0]
	needle := strings.ToLower(strings.TrimSpace(l.filter))
	for i, it := range l.items {
		if needle != "" {
			if it.kind == kindHeader {
				continue
			}
			if !fuzzyMatch(strings.ToLower(it.title+" "+it.subtitle), needle) {
				continue
			}
		}
		l.visible = append(l.visible, i)
	}
	if l.ranked && needle != "" {
		sort.SliceStable(l.visible, func(a, b int) bool {
			return matchRank(l.items[l.visible[a]].title, needle) < matchRank(l.items[l.visible[b]].title, needle)
		})
	}
}

// matchRank scores how well title matches needle: 0 prefix, 1 word prefix,
// 2 substring, 3 anything looser (subtitle or subsequence).
func matchRank(title, needle string) int {
	t := strings.ToLower(title)
	switch {
	case strings.HasPrefix(t, needle):
		return 0
	case strings.Contains(" "+t, " "+needle):
		return 1
	case strings.Contains(t, needle):
		return 2
	}
	return 3
}

// fuzzyMatch reports whether every word of needle appears in hay as a
// subsequence, so "dft punk" finds "Daft Punk".
func fuzzyMatch(hay, needle string) bool {
	for _, word := range strings.Fields(needle) {
		if strings.Contains(hay, word) {
			continue
		}
		i := 0
		for _, r := range hay {
			if i < len(word) && r == rune(word[i]) {
				i++
			}
		}
		if i < len(word) {
			return false
		}
	}
	return true
}

func (l *listModel) selected() *listItem {
	if l.cursor < 0 || l.cursor >= len(l.visible) {
		return nil
	}
	it := &l.items[l.visible[l.cursor]]
	if it.kind == kindHeader {
		return nil
	}
	return it
}

// selectedIndex is the index in items of the selected entry, or -1.
func (l *listModel) selectedIndex() int {
	if l.selected() == nil {
		return -1
	}
	return l.visible[l.cursor]
}

func (l *listModel) move(delta int) {
	if len(l.visible) == 0 {
		return
	}
	l.cursor += delta
	l.clampCursor()
	// Skip section headers in the direction of travel.
	dir := 1
	if delta < 0 {
		dir = -1
	}
	for l.cursor >= 0 && l.cursor < len(l.visible) && l.items[l.visible[l.cursor]].kind == kindHeader {
		l.cursor += dir
	}
	if l.cursor < 0 || l.cursor >= len(l.visible) {
		l.cursor -= dir
		for l.cursor >= 0 && l.cursor < len(l.visible) && l.items[l.visible[l.cursor]].kind == kindHeader {
			l.cursor -= dir
		}
	}
	l.clampCursor()
}

func (l *listModel) top()    { l.cursor = 0; l.move(0) }
func (l *listModel) bottom() { l.cursor = len(l.visible) - 1; l.move(0) }

func (l *listModel) clampCursor() {
	if l.cursor >= len(l.visible) {
		l.cursor = len(l.visible) - 1
	}
	if l.cursor < 0 {
		l.cursor = 0
	}
}

// view renders height rows of width columns. playing is the URI of the
// current track, highlighted wherever it appears.
func (l *listModel) view(t theme, width, height int, playing string) string {
	if height < 1 {
		return ""
	}
	if len(l.visible) == 0 {
		msg := "Nothing here"
		if l.filter != "" {
			msg = "No matches for “" + l.filter + "”"
		}
		return t.dim.Render(msg)
	}

	// Keep the cursor inside the window with a little context around it.
	offset := 0
	if l.scroll != nil {
		offset = *l.scroll
	}
	margin := min(3, height/4)
	if l.cursor < offset+margin {
		offset = l.cursor - margin
	}
	if l.cursor >= offset+height-margin {
		offset = l.cursor - height + margin + 1
	}
	offset = max(0, min(offset, len(l.visible)-height))
	if l.scroll != nil {
		*l.scroll = offset
	}

	numW := 0
	if l.numbered {
		numW = len(itoa(len(l.items))) + 2
	}

	rows := make([]string, 0, height)
	for i := offset; i < len(l.visible) && len(rows) < height; i++ {
		it := l.items[l.visible[i]]
		rows = append(rows, l.row(t, it, i == l.cursor, it.uri != "" && it.uri == playing, width, numW, l.visible[i]+1))
	}
	return strings.Join(rows, "\n")
}

// position describes the cursor for page headers, e.g. "12/340".
func (l *listModel) position() string {
	if len(l.visible) == 0 {
		return ""
	}
	return itoa(l.cursor+1) + "/" + itoa(len(l.visible))
}

func (l *listModel) row(t theme, it listItem, selected, playing bool, width, numW, num int) string {
	if it.kind == kindHeader {
		if it.title == "" {
			return "" // spacer
		}
		label := " " + strings.ToUpper(it.title) + " "
		line := t.accentTxt.Bold(true).Render(label)
		rest := width - lipgloss.Width(line)
		if rest > 0 {
			line += t.dim.Render(strings.Repeat("─", rest))
		}
		return line
	}

	// Every segment gets the row background itself: an outer style cannot
	// paint across the resets of already-styled inner text.
	bg := func(s lipgloss.Style) lipgloss.Style {
		if selected {
			return s.Background(colSelBg)
		}
		return s
	}
	dim, accent := bg(t.dim), bg(t.accentTxt)

	marker := bg(lipgloss.NewStyle()).Render("  ")
	switch {
	case selected && playing:
		marker = accent.Render("▌♪")
	case selected:
		marker = accent.Render("▌ ")
	case playing:
		marker = accent.Render(" ♪")
	}

	num_ := ""
	if numW > 0 {
		num_ = dim.Render(padLeft(itoa(num), numW-1) + " ")
	}

	right := ""
	if it.right != "" {
		right = " " + it.right + " "
	}
	avail := max(width-2-lipgloss.Width(num_)-ansi.StringWidth(right)-1, 4)

	titleStyle := bg(t.text)
	if playing {
		titleStyle = accent
	}
	if selected {
		titleStyle = titleStyle.Bold(true)
	}

	heart := ""
	if it.kind == kindTrack && l.liked[it.uri] {
		heart = " ♥"
		avail -= 2
	}
	title := ansi.Truncate(" "+it.title, avail, "…")
	titleW := ansi.StringWidth(title) + ansi.StringWidth(heart)
	sub := ""
	if it.subtitle != "" && avail-titleW > 6 {
		sub = ansi.Truncate("  "+it.subtitle, avail-titleW, "…")
	}
	pad := max(avail-titleW-ansi.StringWidth(sub), 0)

	return marker + num_ + titleStyle.Render(title) + accent.Render(heart) + dim.Render(sub+strings.Repeat(" ", pad)) + dim.Render(right)
}

func padLeft(s string, w int) string {
	if len(s) >= w {
		return s
	}
	return strings.Repeat(" ", w-len(s)) + s
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		p--
		b[p] = '-'
	}
	return string(b[p:])
}
