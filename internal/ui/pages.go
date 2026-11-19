package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/devgianlu/go-librespot/daemon"

	"github.com/Nikkoro/spotui/internal/cover"
	"github.com/Nikkoro/spotui/internal/engine"
	"github.com/Nikkoro/spotui/internal/platform"
	"github.com/Nikkoro/spotui/internal/webapi"
)

type page int

const (
	pageNowPlaying page = iota
	pageLibrary
	pageSearch
	pageQueue
	pageLyrics
	pageContext
)

// pageNames are the numbered tabs; index i is page(i).
var pageNames = []string{"Now Playing", "Library", "Search", "Queue", "Lyrics"}

// --- page state ---

const (
	libPlaylists = iota
	libAlbums
	libArtists
)

var libTabs = []string{"Playlists", "Albums", "Artists"}

type libraryPage struct {
	tab     int
	lists   [3]listModel
	loaded  [3]bool
	loading [3]bool
	err     [3]error
}

func newLibraryPage() libraryPage { return libraryPage{} }

var searchTabs = []string{"Tracks", "Albums", "Artists", "Playlists"}

type searchPage struct {
	input   textinput.Model
	query   string
	tab     int
	lists   [4]listModel
	loading bool
	err     error
	// pending is set while waiting for the user to authorize the Web API;
	// the query runs again once that finishes.
	pending string
}

func newSearchPage() searchPage {
	in := textinput.New()
	in.Prompt = "  "
	in.Placeholder = "Search songs, albums, artists, playlists…"
	in.CharLimit = 120
	return searchPage{input: in}
}

type queuePage struct {
	list    listModel
	loading bool
	err     error
}

func newQueuePage() queuePage { return queuePage{} }

// ctxCoverPx is the size of a playlist / album cover in its page header.
const ctxCoverPx = 12

type ctxCoverMsg struct {
	uri string
	pix *cover.Pixels
}

type contextPage struct {
	uri      string
	title    string
	subtitle string
	cover    *cover.Pixels
	// likedLoaded: the hearts for this page have been asked for.
	likedLoaded bool
	kind        string
	list        listModel
	loading     bool
	ready       bool
	length      int
	cached      int
	err         error
	back        page
	backCtx     *contextPage
	openedAt    time.Time
}

// activeList returns the list the cursor keys drive on the current page.
func (m *Model) activeList() *listModel {
	switch m.page {
	case pageLibrary:
		return &m.lib.lists[m.lib.tab]
	case pageSearch:
		return &m.search.lists[m.search.tab]
	case pageQueue:
		return &m.queue.list
	case pageContext:
		if m.ctxPage != nil {
			return &m.ctxPage.list
		}
	}
	return nil
}

// --- messages ---

type libLoadedMsg struct {
	tab   int
	items []listItem
	err   error
}

type searchMsg struct {
	query string
	res   *webapi.SearchResult
	err   error
}

type queueMsg struct {
	items []listItem
	err   error
}

type ctxTracksMsg struct {
	uri string
	res *daemon.ApiContextTracks
	err error
}

type ctxPollMsg struct{ uri string }

type webAuthStartedMsg struct {
	url  string
	done <-chan error
	err  error
}

type webAuthDoneMsg struct{ err error }

// --- loaders ---

func (m Model) loadLibrary(tab int) tea.Cmd {
	eng, username := m.eng, m.username
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		var items []listItem
		switch tab {
		case libPlaylists:
			pls, err := eng.Playlists(ctx)
			if err != nil {
				return libLoadedMsg{tab: tab, err: err}
			}
			items = append(items, listItem{
				kind: kindDj, title: "◉ DJ", subtitle: "Spotify's DJ, talking between tracks",
				uri: engine.DjContextURI,
			})
			if username != "" {
				items = append(items, listItem{
					kind: kindPlaylist, title: "♥ Liked Songs", subtitle: "your saved tracks",
					uri: "spotify:user:" + username + ":collection",
				})
			}
			for _, p := range pls {
				sub := "by " + p.OwnerUsername
				if p.OwnerUsername == username {
					sub = "yours"
				}
				if len(p.Folder) > 0 {
					sub = strings.Join(p.Folder, " / ") + " · " + sub
				}
				items = append(items, listItem{
					kind: kindPlaylist, title: p.Name, subtitle: sub,
					right: plural(int(p.Length), "track"), uri: p.Uri, imageURL: deref(p.ImageUrl),
					editable: p.CanEdit || p.OwnerUsername == username,
				})
			}
		case libAlbums:
			albums, err := eng.Albums(ctx)
			if err != nil {
				return libLoadedMsg{tab: tab, err: err}
			}
			for _, a := range albums {
				right := ""
				if a.Year > 0 {
					right = itoa(a.Year)
				}
				items = append(items, listItem{
					kind: kindAlbum, title: a.Name, subtitle: strings.Join(a.ArtistNames, ", "),
					right: right, uri: a.Uri, imageURL: deref(a.ImageUrl),
				})
			}
		case libArtists:
			artists, err := eng.Artists(ctx)
			if err != nil {
				return libLoadedMsg{tab: tab, err: err}
			}
			for _, a := range artists {
				items = append(items, listItem{kind: kindArtist, title: a.Name, uri: a.Uri, imageURL: deref(a.ImageUrl)})
			}
		}
		return libLoadedMsg{tab: tab, items: items}
	}
}

func (m Model) runSearch(query string) tea.Cmd {
	api := m.api
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		res, err := api.Search(ctx, query)
		return searchMsg{query: query, res: res, err: err}
	}
}

func (m Model) startWebAuth() tea.Cmd {
	pkce := m.pkce
	return func() tea.Msg {
		url, done, err := pkce.Authorize(context.Background())
		return webAuthStartedMsg{url: url, done: done, err: err}
	}
}

func waitWebAuth(done <-chan error) tea.Cmd {
	return func() tea.Msg { return webAuthDoneMsg{err: <-done} }
}

func (m Model) loadQueue() tea.Cmd {
	eng := m.eng
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		q, err := eng.Queue(ctx)
		if err != nil {
			return queueMsg{err: err}
		}

		var items []listItem
		if q.Track != nil {
			items = append(items, listItem{kind: kindHeader, title: "Now playing"})
			items = append(items, queueItem(*q.Track))
		}
		if len(q.NextTracks) > 0 {
			if len(items) > 0 {
				items = append(items, listItem{kind: kindHeader})
			}
			items = append(items, listItem{kind: kindHeader, title: "Next up"})
			for _, e := range q.NextTracks {
				items = append(items, queueItem(e))
			}
		}
		if len(q.PrevTracks) > 0 {
			if len(items) > 0 {
				items = append(items, listItem{kind: kindHeader})
			}
			items = append(items, listItem{kind: kindHeader, title: "Played"})
			for i := len(q.PrevTracks) - 1; i >= 0; i-- {
				items = append(items, queueItem(q.PrevTracks[i]))
			}
		}
		return queueMsg{items: items}
	}
}

func queueItem(e daemon.ApiQueueEntry) listItem {
	it := listItem{kind: kindTrack, uri: e.Uri, title: e.Uri}
	if e.Track != nil {
		it = trackItem(*e.Track, "")
	}
	if e.Provider == "queue" {
		it.right = "queued  " + it.right
	}
	return it
}

func trackItem(t daemon.ApiTrack, contextURI string) listItem {
	return listItem{
		kind:     kindTrack,
		title:    t.Name,
		subtitle: strings.Join(t.ArtistNames, ", "),
		right:    fmtDuration(int64(t.Duration)),
		uri:      t.Uri,
		context:  contextURI,
	}
}

func (m Model) loadContext(uri string) tea.Cmd {
	eng := m.eng
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		res, err := eng.ContextTracks(ctx, uri)
		return ctxTracksMsg{uri: uri, res: res, err: err}
	}
}

// openContext pushes a context page (playlist, album, artist).
func (m *Model) openContext(it listItem) tea.Cmd {
	kind := map[itemKind]string{kindAlbum: "Album", kindArtist: "Artist", kindPlaylist: "Playlist"}[it.kind]
	cp := &contextPage{
		uri:      it.uri,
		title:    it.title,
		subtitle: it.subtitle,
		kind:     kind,
		loading:  true,
		back:     m.page,
		backCtx:  m.ctxPage,
		openedAt: time.Now(),
	}
	cp.list.numbered = true
	m.ctxPage = cp
	m.page = pageContext
	m.closeFilter()
	cmds := []tea.Cmd{m.loadContext(it.uri)}
	if it.imageURL != "" && m.cfg.Cover.Enabled && m.covers != nil {
		covers, url, uri := m.covers, it.imageURL, it.uri
		cmds = append(cmds, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			img, err := covers.Get(ctx, url)
			if err != nil {
				return nil
			}
			return ctxCoverMsg{uri: uri, pix: cover.Pixelate(img, ctxCoverPx, 0)}
		})
	}
	return tea.Batch(cmds...)
}

func (m *Model) goBack() {
	if m.page != pageContext || m.ctxPage == nil {
		return
	}
	back := m.ctxPage.back
	m.ctxPage = m.ctxPage.backCtx
	if back == pageContext && m.ctxPage == nil {
		back = pageLibrary
	}
	m.page = back
	m.closeFilter()
}

func (m *Model) switchPage(p page) tea.Cmd {
	if m.page == p {
		return nil
	}
	m.closeFilter()
	m.page = p
	switch p {
	case pageLibrary:
		return m.ensureLibrary()
	case pageSearch:
		if m.search.query == "" {
			return m.search.input.Focus()
		}
	case pageQueue:
		m.queue.loading = true
		return m.loadQueue()
	}
	return nil
}

func (m *Model) ensureLibrary() tea.Cmd {
	t := m.lib.tab
	if m.lib.loaded[t] || m.lib.loading[t] {
		return nil
	}
	if len(m.lib.lists[t].items) == 0 {
		if cached := m.readLibCache(t); len(cached) > 0 {
			m.lib.lists[t].setItems(cached)
		}
	}
	m.lib.loading[t] = true
	return m.loadLibrary(t)
}

func (m *Model) closeFilter() {
	if m.filtering {
		m.filtering = false
		m.filterIn.Blur()
	}
}

// --- page message handling ---

func (m Model) updatePages(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case libLoadedMsg:
		m.lib.loading[msg.tab] = false
		m.lib.err[msg.tab] = msg.err
		if msg.err == nil {
			m.lib.loaded[msg.tab] = true
			m.lib.lists[msg.tab].setItems(msg.items)
			go m.writeLibCache(msg.tab, msg.items)
			if msg.tab == libPlaylists && m.overlay == overlayPlaylists {
				var items []listItem
				for _, it := range msg.items {
					if it.kind == kindPlaylist && it.editable {
						items = append(items, it)
					}
				}
				m.picker.list.setItems(items)
				m.picker.list.setFilter(m.picker.filter.Value())
			}
		}
		return m, nil

	case searchMsg:
		if msg.query != m.search.query {
			return m, nil
		}
		m.search.loading = false
		if errors.Is(msg.err, webapi.ErrNotAuthorized) && m.pkce != nil {
			m.search.pending = msg.query
			return m, m.startWebAuth()
		}
		m.search.err = msg.err
		if msg.err == nil {
			m.setSearchResults(msg.res)
		}
		return m, nil

	case webAuthStartedMsg:
		if msg.err != nil {
			m.search.err = msg.err
			return m, nil
		}
		m.webAuthURL = msg.url
		_ = platform.OpenURL(msg.url)
		return m, waitWebAuth(msg.done)

	case webAuthDoneMsg:
		m.webAuthURL = ""
		if msg.err != nil {
			m.search.err = fmt.Errorf("authorization: %w", msg.err)
			return m, nil
		}
		m.setFlash("Web API connected", false)
		if q := m.search.pending; q != "" {
			m.search.pending = ""
			m.search.loading = true
			return m, m.runSearch(q)
		}
		return m, nil

	case queueMsg:
		m.queue.loading = false
		m.queue.err = msg.err
		if msg.err == nil {
			m.queue.list.setItems(msg.items)
		}
		return m, nil

	case ctxTracksMsg:
		cp := m.ctxPage
		if cp == nil || cp.uri != msg.uri {
			return m, nil
		}
		if msg.err != nil {
			cp.loading = false
			cp.err = msg.err
			return m, nil
		}
		res := msg.res
		cp.ready, cp.length, cp.cached = res.Ready, res.Length, res.Cached
		items := make([]listItem, 0, len(res.Tracks))
		for _, t := range res.Tracks {
			if t.Track != nil {
				items = append(items, trackItem(*t.Track, cp.uri))
			} else {
				items = append(items, listItem{kind: kindTrack, title: "…", uri: t.Uri, context: cp.uri})
			}
		}
		cp.list.setItems(items)
		done := res.Ready && res.Cached >= res.Length
		cp.loading = !done
		if done && !cp.likedLoaded {
			cp.likedLoaded = true
			uris := make([]string, 0, len(items))
			for _, it := range items {
				if strings.HasPrefix(it.uri, "spotify:track:") {
					uris = append(uris, it.uri)
				}
			}
			return m, m.fetchLikedMany(uris)
		}
		if !done && time.Since(cp.openedAt) < 3*time.Minute {
			uri := cp.uri
			return m, tea.Tick(800*time.Millisecond, func(time.Time) tea.Msg { return ctxPollMsg{uri: uri} })
		}
		return m, nil

	case ctxCoverMsg:
		if m.ctxPage != nil && m.ctxPage.uri == msg.uri {
			m.ctxPage.cover = msg.pix
		}
		return m, nil

	case ctxPollMsg:
		if m.ctxPage == nil || m.ctxPage.uri != msg.uri {
			return m, nil
		}
		return m, m.loadContext(msg.uri)
	}
	return m, nil
}

func (m *Model) setSearchResults(res *webapi.SearchResult) {
	var tracks, albums, artists, playlists []listItem
	for _, t := range res.Tracks {
		names := make([]string, len(t.Artists))
		for i, a := range t.Artists {
			names[i] = a.Name
		}
		// Playing a found track starts its album at that track, so music
		// keeps going afterwards.
		tracks = append(tracks, listItem{
			kind: kindTrack, title: t.Name, subtitle: strings.Join(names, ", ") + " · " + t.Album.Name,
			right: fmtDuration(int64(t.DurationMs)), uri: t.URI, context: t.Album.URI,
		})
	}
	for _, a := range res.Albums {
		names := make([]string, len(a.Artists))
		for i, ar := range a.Artists {
			names[i] = ar.Name
		}
		right := ""
		if len(a.ReleaseDate) >= 4 {
			right = a.ReleaseDate[:4]
		}
		albums = append(albums, listItem{kind: kindAlbum, title: a.Name, subtitle: strings.Join(names, ", "), right: right, uri: a.URI, imageURL: firstImage(a.Images)})
	}
	for _, a := range res.Artists {
		sub := ""
		if len(a.Genres) > 0 {
			sub = strings.Join(a.Genres[:min(2, len(a.Genres))], ", ")
		}
		artists = append(artists, listItem{kind: kindArtist, title: a.Name, subtitle: sub, uri: a.URI, imageURL: firstImage(a.Images)})
	}
	for _, p := range res.Playlists {
		playlists = append(playlists, listItem{
			kind: kindPlaylist, title: p.Name, subtitle: "by " + p.Owner.DisplayName,
			right: plural(p.Tracks.Total, "track"), uri: p.URI, imageURL: firstImage(p.Images),
		})
	}
	m.search.lists[0].setItems(tracks)
	m.search.lists[1].setItems(albums)
	m.search.lists[2].setItems(artists)
	m.search.lists[3].setItems(playlists)
	for i := range m.search.lists {
		m.search.lists[i].top()
	}
	// Land on the first tab that has something.
	m.search.tab = 0
	for i, l := range m.search.lists {
		if len(l.items) > 0 {
			m.search.tab = i
			break
		}
	}
}

// --- keys ---

// handlePageKey handles navigation; handled=false lets player keys run.
func (m Model) handlePageKey(key string) (bool, tea.Model, tea.Cmd) {
	switch key {
	case "1":
		return true, m, m.switchPage(pageNowPlaying)
	case "2":
		cmd := m.switchPage(pageLibrary)
		return true, m, cmd
	case "3":
		cmd := m.switchPage(pageSearch)
		return true, m, cmd
	case "4":
		cmd := m.switchPage(pageQueue)
		return true, m, cmd
	case "5":
		cmd := m.switchPage(pageLyrics)
		return true, m, cmd
	}

	if m.page == pageLyrics {
		return m.handleLyricsKey(key)
	}

	if m.page == pageSearch && m.webAuthURL != "" && key == "o" {
		_ = platform.OpenURL(m.webAuthURL)
		return true, m, nil
	}

	list := m.activeList()
	if list == nil {
		return false, m, nil
	}

	half := max((m.height-12)/2, 1)
	switch key {
	case "up", "k":
		list.move(-1)
	case "down", "j":
		list.move(1)
	case "pgup", "ctrl+u":
		list.move(-half)
	case "pgdown", "ctrl+d":
		list.move(half)
	case "home", "g":
		list.top()
	case "end", "G":
		list.bottom()
	case "tab", "shift+tab":
		dir := 1
		if key == "shift+tab" {
			dir = -1
		}
		switch m.page {
		case pageLibrary:
			m.closeFilter()
			m.lib.tab = (m.lib.tab + dir + len(libTabs)) % len(libTabs)
			return true, m, m.ensureLibrary()
		case pageSearch:
			m.closeFilter()
			m.search.tab = (m.search.tab + dir + len(searchTabs)) % len(searchTabs)
		default:
			return false, m, nil
		}
	case "/":
		if m.page == pageSearch {
			m.search.input.SetValue(m.search.query)
			m.search.input.CursorEnd()
			return true, m, m.search.input.Focus()
		}
		m.filtering = true
		m.filterIn.SetValue(list.filter)
		return true, m, m.filterIn.Focus()
	case "esc", "backspace":
		if list.filter != "" {
			list.setFilter("")
			return true, m, nil
		}
		if m.page == pageContext {
			m.goBack()
			return true, m, nil
		}
		return false, m, nil
	case "enter":
		return true, m, m.activate(list.selected())
	case "f":
		// Like the selected track; without one, f acts on the playing track.
		it := list.selected()
		if it == nil || it.kind != kindTrack || !strings.HasPrefix(it.uri, "spotify:track:") {
			return false, m, nil
		}
		on := !m.liked[it.uri]
		m.liked[it.uri] = on
		if on {
			m.setFlash("Added “"+it.title+"” to Liked Songs", false)
		} else {
			m.setFlash("Removed “"+it.title+"” from Liked Songs", false)
		}
		eng, uri := m.eng, it.uri
		return true, m, call("Like", func(ctx context.Context) error { return eng.SetLiked(ctx, []string{uri}, on) })
	case "a":
		it := list.selected()
		if it == nil || it.kind != kindTrack {
			return true, m, nil
		}
		eng, uri := m.eng, it.uri
		return true, m, call("Add to queue", func(ctx context.Context) error { return eng.AddToQueue(ctx, uri) })
	case "R":
		switch m.page {
		case pageLibrary:
			m.lib.loaded[m.lib.tab] = false
			return true, m, m.ensureLibrary()
		case pageQueue:
			return true, m, m.loadQueue()
		case pageContext:
			m.ctxPage.loading = true
			return true, m, m.loadContext(m.ctxPage.uri)
		}
	default:
		return false, m, nil
	}
	return true, m, nil
}

// activate is Enter on a list row.
func (m *Model) activate(it *listItem) tea.Cmd {
	if it == nil {
		return nil
	}
	eng := m.eng
	switch it.kind {
	case kindTrack:
		// In the queue, jump ahead to the entry instead of replacing what
		// plays, so the rest of the queue survives.
		if m.page == pageQueue {
			if it.uri == m.currentURI() {
				return nil
			}
			uri := it.uri
			m.setFlash("Skipping to "+it.title, false)
			return call("Skip", func(ctx context.Context) error { return eng.SkipTo(ctx, uri) })
		}
		ctxURI, trackURI := it.context, it.uri
		if ctxURI == "" {
			ctxURI, trackURI = it.uri, ""
		}
		m.setFlash("Playing "+it.title, false)
		return call("Play", func(ctx context.Context) error { return eng.PlayContext(ctx, ctxURI, trackURI) })
	case kindAlbum, kindPlaylist, kindArtist:
		return m.openContext(*it)
	case kindDj:
		return m.startDj()
	}
	return nil
}

func (m Model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	list := m.activeList()
	switch msg.String() {
	case "esc":
		m.closeFilter()
		if list != nil {
			list.setFilter("")
		}
		return m, nil
	case "enter", "down", "up":
		// Keep the filter, return to the list.
		m.closeFilter()
		return m, nil
	}
	var cmd tea.Cmd
	m.filterIn, cmd = m.filterIn.Update(msg)
	if list != nil {
		list.setFilter(m.filterIn.Value())
	}
	return m, cmd
}

func (m Model) updateSearchInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.search.input.Blur()
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "enter":
		q := strings.TrimSpace(m.search.input.Value())
		m.search.input.Blur()
		if q == "" {
			return m, nil
		}
		m.search.query = q
		m.search.loading = true
		m.search.err = nil
		m.rememberSearch(q)
		return m, m.runSearch(q)
	case "up":
		// Walk back through earlier searches.
		if m.historyIdx+1 < len(m.searchHistory) {
			m.historyIdx++
			m.search.input.SetValue(m.searchHistory[m.historyIdx])
			m.search.input.CursorEnd()
		}
		return m, nil
	case "down":
		if m.historyIdx > 0 {
			m.historyIdx--
			m.search.input.SetValue(m.searchHistory[m.historyIdx])
			m.search.input.CursorEnd()
			return m, nil
		}
		if m.historyIdx == 0 {
			m.historyIdx = -1
			m.search.input.SetValue("")
			return m, nil
		}
		m.search.input.Blur()
		return m, nil
	case "tab":
		m.search.input.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	m.search.input, cmd = m.search.input.Update(msg)
	return m, cmd
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return itoa(n) + " " + word + "s"
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// firstImage picks a mid-sized image: the list is largest first, and the
// header only needs a small one.
func firstImage(images []webapi.Image) string {
	if len(images) == 0 {
		return ""
	}
	return images[len(images)/2].URL
}
