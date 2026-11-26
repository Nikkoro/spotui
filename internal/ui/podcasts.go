package ui

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Nikkoro/spotui/internal/engine"
	"github.com/Nikkoro/spotui/internal/webapi"
)

// Podcasts: the saved shows in the library, shows and episodes in search,
// a show's page with each episode's progress, the episode's notes where
// lyrics would be, and marking episodes as played.

func isEpisode(uri string) bool { return strings.HasPrefix(uri, "spotify:episode:") }
func isShow(uri string) bool    { return strings.HasPrefix(uri, "spotify:show:") }

// yourEpisodesURI is the context of the episodes saved with f.
func yourEpisodesURI(username string) string {
	return "spotify:user:" + username + ":collection:your-episodes"
}

func showItem(s webapi.Show) listItem {
	right := ""
	if s.TotalEpisodes > 0 {
		right = plural(s.TotalEpisodes, "episode")
	}
	return listItem{kind: kindShow, title: s.Name, subtitle: s.Publisher, right: right,
		uri: s.URI, imageURL: firstImage(s.Images), desc: s.Description}
}

// episodeItem is an episode in a list, played in context (its show).
func episodeItem(e webapi.Episode, context string) listItem {
	sub := e.ReleaseDate
	if e.Show != nil && e.Show.Name != "" {
		sub = e.Show.Name + " · " + sub
		if context == "" {
			context = e.Show.URI
		}
	}
	return listItem{kind: kindTrack, title: e.Name, subtitle: sub, right: fmtDuration(int64(e.DurationMs)),
		uri: e.URI, context: context, durMs: int64(e.DurationMs), desc: e.Description}
}

// episodeRight is what the right column says of an episode: its length,
// how much is left, or that it was played.
func episodeRight(durMs int64, st engine.EpisodeState, known bool) string {
	switch {
	case known && st.Finished:
		return "✓ played"
	case known && st.PositionMs > 0 && durMs > st.PositionMs:
		left := (durMs - st.PositionMs + 59_999) / 60_000
		return fmt.Sprintf("%d min left", left)
	}
	return fmtDuration(durMs)
}

// --- the show page ---

type showPageMsg struct {
	uri      string
	offset   int
	show     *webapi.Show
	episodes []webapi.Episode
	total    int
	err      error
}

type episodeStatesMsg struct {
	states map[string]engine.EpisodeState
}

// loadShow reads a page of a show's episodes (and, first, the show).
func (m Model) loadShow(uri string, offset int) tea.Cmd {
	api := m.api
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		msg := showPageMsg{uri: uri, offset: offset}
		if offset == 0 {
			if msg.show, msg.err = api.ShowDetails(ctx, uri); msg.err != nil {
				msg.err = webErr(msg.err)
				return msg
			}
		}
		msg.episodes, msg.total, msg.err = api.ShowEpisodes(ctx, uri, offset)
		msg.err = webErr(msg.err)
		return msg
	}
}

// fetchEpisodeStates asks the player how far each episode was listened to.
func (m Model) fetchEpisodeStates(uris []string) tea.Cmd {
	if len(uris) == 0 {
		return nil
	}
	eng := m.eng
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		states, err := eng.EpisodeStates(ctx, uris)
		if err != nil {
			eng.Debugf("ui: episode states: %v", err)
			return nil
		}
		return episodeStatesMsg{states: states}
	}
}

func (m *Model) handleShowPage(msg showPageMsg) tea.Cmd {
	cp := m.ctxPage
	if cp == nil || cp.uri != msg.uri {
		return nil
	}
	cp.loading = false
	if msg.err != nil {
		cp.err = msg.err
		return nil
	}
	if msg.show != nil {
		cp.title = msg.show.Name
		cp.subtitle = msg.show.Publisher
		cp.desc = msg.show.Description
	}
	items := cp.list.items
	if msg.offset == 0 {
		items = nil
	}
	uris := make([]string, 0, len(msg.episodes))
	for _, e := range msg.episodes {
		it := episodeItem(e, cp.uri)
		it.subtitle = e.ReleaseDate
		if st, ok := m.episodes[e.URI]; ok {
			it.right = episodeRight(it.durMs, st, true)
		}
		items = append(items, it)
		uris = append(uris, e.URI)
	}
	cp.list.setItems(items)
	cp.length, cp.cached, cp.ready = msg.total, len(items), true
	return m.fetchEpisodeStates(uris)
}

func (m *Model) applyEpisodeStates(states map[string]engine.EpisodeState) {
	for uri, st := range states {
		m.episodes[uri] = st
	}
	refresh := func(l *listModel) {
		for i := range l.items {
			it := &l.items[i]
			if st, ok := states[it.uri]; ok && isEpisode(it.uri) {
				it.right = episodeRight(it.durMs, st, true)
			}
		}
	}
	if m.ctxPage != nil {
		refresh(&m.ctxPage.list)
	}
	for i := range m.search.lists {
		refresh(&m.search.lists[i])
	}
}

// moreEpisodes loads the next page when the cursor reaches the end of a
// show's episodes.
func (m *Model) moreEpisodes() tea.Cmd {
	cp := m.ctxPage
	if m.page != pageContext || cp == nil || !isShow(cp.uri) || cp.loading || cp.list.filter != "" {
		return nil
	}
	n := len(cp.list.items)
	if n == 0 || n >= cp.length || cp.list.cursor < len(cp.list.visible)-1 {
		return nil
	}
	cp.loading = true
	return m.loadShow(cp.uri, n)
}

// --- played / not played ---

// toggleEpisodePlayed marks the selected episode (or the one playing) as
// played, or takes the mark off.
func (m *Model) toggleEpisodePlayed() tea.Cmd {
	uri, title := "", ""
	if l := m.activeList(); l != nil && m.page != pageNowPlaying {
		if it := l.selected(); it != nil && isEpisode(it.uri) {
			uri, title = it.uri, it.title
		}
	}
	if uri == "" && m.track != nil && isEpisode(m.track.Uri) {
		uri, title = m.track.Uri, m.track.Name
	}
	if uri == "" {
		return nil
	}
	played := !m.episodes[uri].Finished
	st := engine.EpisodeState{Uri: uri, Finished: played}
	m.applyEpisodeStates(map[string]engine.EpisodeState{uri: st})
	flash := "Marked “" + title + "” as played"
	if !played {
		flash = "Marked “" + title + "” as not played"
	}
	m.setFlash(flash, false)
	eng := m.eng
	return call("Mark as played", func(ctx context.Context) error { return eng.SetEpisodePlayed(ctx, uri, played) })
}

// --- the episode's notes on Now Playing and the lyrics page ---

type episodeNotesMsg struct {
	uri, notes string
}

func (m Model) fetchEpisodeNotes(uri string) tea.Cmd {
	if !isEpisode(uri) || m.api == nil {
		return nil
	}
	if _, ok := m.episodeNotes[uri]; ok {
		return nil
	}
	api := m.api
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		e, err := api.EpisodeDetails(ctx, uri)
		if err != nil {
			return episodeNotesMsg{uri: uri}
		}
		notes := htmlToText(e.HTMLDescription)
		if notes == "" {
			notes = e.Description
		}
		return episodeNotesMsg{uri: uri, notes: notes}
	}
}

// viewEpisodeNotes shows an episode's description, wrapped, scrolled by
// the lyrics keys.
func (m Model) viewEpisodeNotes(w, h int) string {
	notes, ok := m.episodeNotes[m.currentURI()]
	switch {
	case !ok:
		return m.centreText(w, h, m.loadingLine("Reading the episode notes…"))
	case strings.TrimSpace(notes) == "":
		return m.centreText(w, h, m.theme.dim.Render("This episode has no notes."))
	}
	tw := max(min(w-4, 90), 10)
	place := func(s string) string {
		return lipgloss.PlaceHorizontal(w, lipgloss.Center, lipgloss.PlaceHorizontal(tw, lipgloss.Left, s))
	}

	// Chapters first, the one playing lit, then the rest of the notes.
	var lines []string
	chapters, rest := parseChapters(notes)
	if len(chapters) > 0 {
		cur := currentChapter(chapters, m.position())
		lines = append(lines, m.theme.dim.Bold(true).Render("CHAPTERS")+m.theme.dim.Render("  < > previous / next"))
		for i, c := range chapters {
			row := fmt.Sprintf("%8s  %s", fmtDuration(c.startMs), c.title)
			row = ansi.Truncate(row, tw, "…")
			if i == cur {
				lines = append(lines, m.theme.accentTxt.Bold(true).Render("▶"+strings.TrimPrefix(row, " ")))
			} else {
				lines = append(lines, m.theme.dim.Render(row))
			}
		}
		lines = append(lines, "")
		notes = rest
	}
	// Words wrap at spaces; a long link breaks where it must.
	for _, line := range strings.Split(ansi.Wrap(strings.TrimSpace(notes), tw, ""), "\n") {
		line = strings.TrimRight(line, " ")
		st := m.theme.text
		if strings.Contains(line, "://") {
			st = m.theme.dim // links step back
		}
		lines = append(lines, st.Render(line))
	}

	start := min(m.lyricsScroll, max(len(lines)-h, 0))
	rows := make([]string, 0, h)
	for i := start; i < len(lines) && len(rows) < h; i++ {
		rows = append(rows, place(lines[i]))
	}
	return strings.Join(rows, "\n")
}

// chapter is a part of an episode its notes name with a time.
type chapter struct {
	startMs int64
	title   string
}

// chapterLead is a line of notes that starts with a time: "03:35 News",
// "(1:02:10) - Questions".
var chapterLead = regexp.MustCompile(`^[\[(]?((?:\d{1,2}:)?\d{1,2}:\d{2})[\])]?\s*[-–—:|.]?\s*(.+)$`)

// parseChapters finds the chapters in an episode's notes, the way the
// Spotify apps and YouTube read them: at least two lines with times, in
// order. rest is the notes without those lines.
func parseChapters(notes string) (chapters []chapter, rest string) {
	var kept []string
	for _, line := range strings.Split(notes, "\n") {
		l := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "• "))
		var ts, title string
		if m := chapterLead.FindStringSubmatch(l); m != nil {
			ts, title = m[1], m[2]
		}
		if ms, ok := parseClock(ts); ok && strings.TrimSpace(title) != "" {
			chapters = append(chapters, chapter{startMs: ms, title: strings.TrimSpace(title)})
			continue
		}
		kept = append(kept, line)
	}
	for i := 1; i < len(chapters); i++ {
		if chapters[i].startMs <= chapters[i-1].startMs {
			return nil, notes // not in order: not a chapter list
		}
	}
	if len(chapters) < 2 {
		return nil, notes
	}
	return chapters, strings.TrimSpace(manyBlankRows.ReplaceAllString(strings.Join(kept, "\n"), "\n\n"))
}

// parseClock reads "3:35", "03:35" or "1:02:10".
func parseClock(s string) (int64, bool) {
	if s == "" {
		return 0, false
	}
	var total int64
	parts := strings.Split(s, ":")
	for i, p := range parts {
		n := int64(atoiOr(p, -1))
		if n < 0 || (i > 0 && n >= 60) {
			return 0, false
		}
		total = total*60 + n
	}
	return total * 1000, true
}

// currentChapter is the index of the chapter playing at posMs.
func currentChapter(chapters []chapter, posMs int64) int {
	cur := 0
	for i, c := range chapters {
		if c.startMs <= posMs {
			cur = i
		}
	}
	return cur
}

// jumpChapter seeks to the next (+1) or previous (-1) chapter of the
// playing episode; back goes to the start of the current one first, when
// it has played a few seconds, like a player's previous button.
func (m *Model) jumpChapter(dir int) tea.Cmd {
	uri := m.currentURI()
	chapters, _ := parseChapters(m.episodeNotes[uri])
	if !isEpisode(uri) || len(chapters) == 0 {
		return nil
	}
	pos := m.position()
	cur := currentChapter(chapters, pos)
	target := cur + dir
	if dir < 0 && pos-chapters[cur].startMs > 3000 {
		target = cur
	}
	if target < 0 || target >= len(chapters) {
		return nil
	}
	c := chapters[target]
	m.setPosition(c.startMs)
	m.setFlash("Chapter: "+c.title, false)
	eng, at := m.eng, c.startMs
	return call("Seek", func(ctx context.Context) error { return eng.Seek(ctx, at, false) })
}

// likeFlash says what f did: songs go to Liked Songs, episodes to Your
// Episodes.
func likeFlash(title, uri string, on bool) string {
	what, where := "", "Liked Songs"
	if title != "" {
		what = "“" + title + "” "
	}
	if isEpisode(uri) {
		where = "Your Episodes"
		if on {
			return "Saved " + what + "to " + where
		}
	}
	if on {
		return "Added " + what + "to " + where
	}
	return "Removed " + what + "from " + where
}

var (
	htmlBreak     = regexp.MustCompile(`(?i)<br\s*/?>`)
	htmlParaEnd   = regexp.MustCompile(`(?i)</(p|div|h[1-6]|ul|ol)>`)
	htmlListItem  = regexp.MustCompile(`(?i)<li[^>]*>`)
	htmlListEnd   = regexp.MustCompile(`(?i)</li>`)
	htmlLink      = regexp.MustCompile(`(?is)<a\s[^>]*href="([^"]*)"[^>]*>(.*?)</a>`)
	manyBlankRows = regexp.MustCompile(`\n{3,}`)
)

// htmlToText turns an episode's HTML notes into text that keeps their
// paragraphs, line breaks and lists. A link keeps its text, and its
// address when the text does not show it.
func htmlToText(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	s = htmlLink.ReplaceAllStringFunc(s, func(a string) string {
		m := htmlLink.FindStringSubmatch(a)
		href, text := html.UnescapeString(m[1]), strings.TrimSpace(htmlTag.ReplaceAllString(m[2], ""))
		switch {
		case text == "":
			return href
		case strings.Contains(href, "://") && !strings.Contains(html.UnescapeString(text), strings.TrimPrefix(strings.TrimPrefix(href, "https://"), "http://")):
			return text + " (" + href + ")"
		}
		return text
	})
	s = htmlBreak.ReplaceAllString(s, "\n")
	s = htmlParaEnd.ReplaceAllString(s, "\n\n")
	s = htmlListItem.ReplaceAllString(s, "• ")
	s = htmlListEnd.ReplaceAllString(s, "\n")
	s = html.UnescapeString(htmlTag.ReplaceAllString(s, ""))
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.Join(strings.Fields(l), " ")
	}
	return strings.TrimSpace(manyBlankRows.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
}
