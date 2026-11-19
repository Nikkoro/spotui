package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Nikkoro/spotui/internal/cover"
)

// View renders the whole screen.
func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	w := m.width - 4 // app padding

	header := m.viewHeader(w)
	footer := m.viewFooter(w)

	// Where things land on screen, for the mouse. View works on a copy, so
	// this is written through a pointer the real model shares.
	if m.hits != nil {
		*m.hits = hitMap{bodyTop: 1 + lipgloss.Height(header) + 1}
	}

	var body string
	switch m.phase {
	case phaseConnecting:
		body = m.centre(w, m.bodyHeight(header, footer, ""), m.viewConnecting())
	case phaseAuth:
		body = m.centre(w, m.bodyHeight(header, footer, ""), m.viewAuth())
	case phaseFatal:
		body = m.centre(w, m.bodyHeight(header, footer, ""), m.viewFatal())
	default:
		if m.overlay != overlayNone {
			body = m.viewOverlay(w, m.bodyHeight(header, footer, ""))
		} else if m.page == pageNowPlaying {
			body = m.centre(w, m.bodyHeight(header, footer, ""), m.viewNowPlaying(w))
		} else {
			bar := m.viewPlayerBar(w)
			h := m.bodyHeight(header, footer, bar)
			if m.hits != nil && m.hits.progressW > 0 {
				// rule, title, album, then the progress line
				m.hits.progressY = m.hits.bodyTop + h + 3
			}
			body = lipgloss.NewStyle().Height(h).MaxHeight(h).Render(m.viewPage(w, h)) + "\n" + bar
		}
	}

	return m.theme.app.Render(lipgloss.JoinVertical(lipgloss.Left, header, "", body, footer))
}

func (m Model) bodyHeight(header, footer, bar string) int {
	h := m.height - 2 /* padding */ - lipgloss.Height(header) - 1 /* gap */ - lipgloss.Height(footer)
	if bar != "" {
		h -= lipgloss.Height(bar) + 1
	}
	return max(h, 1)
}

func (m Model) centre(w, h int, s string) string {
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, s)
}

// --- chrome ---

func (m Model) viewHeader(w int) string {
	left := m.theme.pill.Render("♫ spotui")

	if m.phase == phaseReady {
		var tabs []string
		for i, name := range pageNames {
			p := page(i)
			active := m.page == p || (p == pageLibrary && m.page == pageContext)
			label := m.theme.dim.Render(itoa(i+1)+" ") + name
			if active {
				tabs = append(tabs, m.theme.title.Render(itoa(i+1)+" "+name))
			} else {
				tabs = append(tabs, m.theme.text.Faint(true).Render(label))
			}
		}
		left += "  " + strings.Join(tabs, m.theme.dim.Render("   "))
	} else {
		left += m.theme.dim.Render("  " + m.cfg.DeviceName)
	}

	var state string
	switch m.phase {
	case phaseReady:
		state = m.theme.accentTxt.Render("●") + m.theme.dim.Render(" "+m.cfg.DeviceName)
		if s := m.sleepLabel(); s != "" {
			state = m.theme.accentTxt.Render(s) + "   " + state
		}
	case phaseAuth:
		state = m.theme.dim.Render("○ waiting for login")
	case phaseFatal:
		state = m.theme.errorTxt.Render("✕ stopped")
	default:
		state = m.theme.dim.Render("○ connecting")
	}

	gap := w - lipgloss.Width(left) - lipgloss.Width(state)
	if gap < 2 && m.phase == phaseReady {
		state = m.theme.accentTxt.Render("●")
		gap = w - lipgloss.Width(left) - 1
	}
	if gap < 1 {
		return left
	}
	return left + strings.Repeat(" ", gap) + state
}

func (m Model) viewFooter(w int) string {
	if m.inputting {
		return m.theme.dim.Render("Paste a Spotify link or URI · ") +
			m.theme.key.Render("enter") + m.theme.dim.Render(" play · ") +
			m.theme.key.Render("esc") + m.theme.dim.Render(" cancel") + "\n" +
			m.input.View()
	}

	if m.flash != "" && time.Now().Before(m.flashUntil) {
		st := m.theme.accentTxt
		if m.flashErr {
			st = m.theme.errorTxt
		}
		return ansi.Truncate(st.Render(m.flash), w, "…")
	}

	var keys [][2]string
	switch {
	case m.overlay == overlayDevices:
		keys = [][2]string{{"↑↓", "move"}, {"enter", "move playback"}, {"esc", "close"}}
	case m.overlay == overlayPalette:
		keys = [][2]string{{"type", "find"}, {"↑↓", "choose"}, {"enter", "run"}, {"esc", "close"}}
	case m.overlay == overlayPlaylists:
		keys = [][2]string{{"type", "filter"}, {"enter", "add"}, {"esc", "close"}}
	case m.overlay == overlayDj:
		keys = [][2]string{{"enter", "ask"}, {"↑↓", "suggestions"}, {"esc", "close"}}
	case m.overlay == overlayHelp:
		keys = [][2]string{{"esc", "close"}}
	case m.phase == phaseAuth:
		keys = [][2]string{{"o", "open link"}, {"q", "quit"}}
	case m.phase != phaseReady:
		keys = [][2]string{{"q", "quit"}}
	case m.page == pageNowPlaying:
		keys = [][2]string{
			{"space", "play"}, {"n/p", "next/prev"}, {"←→", "seek"}, {"+/-", "vol"},
			{"s", "shuffle"}, {"r", "repeat"}, {"f", "like"}, {"D", "DJ"}, {"ctrl+k", "commands"}, {"?", "help"}, {"q", "quit"},
		}
	case m.page == pageLyrics:
		keys = [][2]string{{"< >", "sync offset"}, {"0", "reset"}, {"↑↓", "scroll (plain)"},
			{"space", "play"}, {"n/p", "next/prev"}, {"q", "quit"}}
	case m.page == pageSearch && m.search.input.Focused():
		keys = [][2]string{{"enter", "search"}, {"esc", "results"}}
	default:
		keys = [][2]string{
			{"↑↓", "move"}, {"enter", "open/play"}, {"a", "queue"}, {"/", "filter"},
		}
		if m.page == pageLibrary || m.page == pageSearch {
			keys = append(keys, [2]string{"tab", "section"})
		}
		if m.page == pageContext {
			keys = append(keys, [2]string{"esc", "back"})
		}
		keys = append(keys, [][2]string{{"space", "play"}, {"ctrl+k", "commands"}, {"?", "help"}, {"q", "quit"}}...)
	}

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, m.theme.key.Render(k[0])+" "+m.theme.dim.Render(k[1]))
	}
	return ansi.Truncate(strings.Join(parts, m.theme.dim.Render("  ")), w, "…")
}

// viewPlayerBar is the compact player under list pages.
func (m Model) viewPlayerBar(w int) string {
	rule := lipgloss.NewStyle().Foreground(colTrack).Render(strings.Repeat("─", w))

	art := ""
	if m.coverMini != nil {
		art = m.coverMini.Render() + "  "
	} else {
		art = m.theme.dim.Render(strings.Repeat("░", miniCoverPx)+"\n"+strings.Repeat("░", miniCoverPx)+"\n"+
			strings.Repeat("░", miniCoverPx)+"\n"+strings.Repeat("░", miniCoverPx)) + "  "
	}
	infoW := max(w-lipgloss.Width(art), 20)

	if m.track == nil {
		return rule + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, art,
			m.theme.dim.Render("Nothing playing · pick a track or select \""+m.cfg.DeviceName+"\" in Spotify Connect")) + "\n"
	}
	t := m.track

	heart := ""
	if m.liked[t.Uri] {
		heart = m.theme.accentTxt.Render(" ♥")
	}
	dj := ""
	if m.playingDj() {
		dj = m.theme.pill.Render("DJ") + " "
	}
	line1 := ansi.Truncate(dj+m.theme.title.Render(t.Name)+heart+m.theme.dim.Render("  ")+
		m.theme.artist.UnsetBold().Render(strings.Join(t.ArtistNames, ", ")), infoW, "…")

	pos, dur := m.position(), int64(t.Duration)
	times := fmtDuration(pos) + " / " + fmtDuration(dur)
	icon := m.stateIcon()
	barW := max(infoW-lipgloss.Width(icon)-len(times)-4, 5)
	if m.hits != nil {
		m.hits.progressX = 2 + lipgloss.Width(art) + lipgloss.Width(icon) + 2
		m.hits.progressW = barW
	}
	line2 := icon + "  " + m.progressBar(barW, pos, dur) + "  " + m.theme.dim.Render(times)

	album := ansi.Truncate(m.theme.album.Render(t.AlbumName), infoW, "…")
	line4 := m.modeBadges() + m.theme.dim.Render("   ") + m.volumeBar(min(infoW-24, 30))

	return rule + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, art,
		lipgloss.JoinVertical(lipgloss.Left, line1, album, line2, line4)) + "\n"
}

func (m Model) stateIcon() string {
	switch {
	case m.buffering:
		return m.spinner.View()
	case m.stopped:
		return m.theme.accentTxt.Render("■")
	case m.paused:
		return m.theme.accentTxt.Render("‖")
	}
	return m.theme.accentTxt.Render("▶")
}

// --- connecting / auth / errors ---

func (m Model) viewConnecting() string {
	return m.theme.card.Render(m.spinner.View() + m.theme.text.Render(" Connecting to Spotify…"))
}

func (m Model) viewFatal() string {
	msg := "unknown error"
	if m.fatal != nil {
		msg = m.fatal.Error()
	}
	return m.theme.card.BorderForeground(colError).Render(
		m.theme.errorTxt.Bold(true).Render("The player stopped") + "\n\n" +
			lipgloss.NewStyle().Width(min(60, m.width-12)).Foreground(colSubtle).Render(msg) + "\n\n" +
			m.theme.dim.Render("Details are in spotui.log in your config folder.") + "\n\n" +
			m.spinner.View() + " " + m.theme.text.Render(m.restartLabel()),
	)
}

func (m Model) viewAuth() string {
	if m.auth == nil {
		return m.viewConnecting()
	}

	lines := []string{
		m.theme.title.Render("Link your Spotify account"),
		"",
		m.theme.text.Render("Open this page on any device and approve:"),
		m.theme.link.Render(m.auth.Url),
	}
	if m.auth.Code != "" {
		lines = append(lines, "",
			m.theme.text.Render("If asked, enter the code:"),
			m.theme.code.Render(m.auth.Code))
	}
	if !m.auth.ExpiresAt.IsZero() {
		left := time.Until(m.auth.ExpiresAt).Round(time.Second)
		if left > 0 {
			lines = append(lines, "", m.theme.dim.Render(fmt.Sprintf("Code expires in %s", left)))
		}
	}
	lines = append(lines, "", m.spinner.View()+m.theme.dim.Render(" Waiting for approval…"))

	return m.theme.card.Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// --- now playing ---

func (m Model) viewNowPlaying(w int) string {
	// Room inside the card: header, footer, paddings and borders.
	availH := m.height - 12
	availW := w - 8

	vizH := 0
	if m.vizVisible() && availH >= 14 {
		vizH = clamp(availH/4, 4, 8)
		availH -= vizH + 1
	}

	// Prefer the cover beside the info, shrinking both (cover never below
	// 12 px) rather than dropping the cover on smaller terminals.
	coverSize := 0
	if m.cfg.Cover.Enabled && m.track != nil {
		coverSize = min(m.pixSize, availH*2)
	}
	infoWidth := clamp(availW-4-coverSize, 30, 56)
	info := m.viewTrackInfo(infoWidth)

	var content string
	if coverSize > 0 {
		size := min(coverSize, availW-4-infoWidth)
		size -= size % 2
		if size >= 12 {
			content = lipgloss.JoinHorizontal(lipgloss.Center, m.coverView(size), "    ", info)
		}
	}
	if content == "" {
		content = info
	}
	if vizH > 0 {
		content = lipgloss.JoinVertical(lipgloss.Left, content, "",
			m.viewVisualizer(lipgloss.Width(content), vizH))
	}

	return m.theme.card.Render(content)
}

// coverView returns the cover at size, cached, or a placeholder while it loads.
func (m Model) coverView(size int) string {
	if m.coverImg == nil {
		return m.coverPlaceholder(size)
	}
	if size == m.pixSize && m.coverPix != nil {
		return m.coverPix.Render()
	}
	if m.coverFit != nil {
		if p, ok := m.coverFit[size]; ok {
			return p.Render()
		}
		p := cover.Pixelate(m.coverImg, size, m.levels)
		m.coverFit[size] = p
		return p.Render()
	}
	return m.coverPlaceholder(size)
}

func (m Model) coverPlaceholder(size int) string {
	row := m.theme.dim.Render(strings.Repeat("░", size))
	rows := make([]string, size/2)
	for i := range rows {
		rows[i] = row
	}
	return strings.Join(rows, "\n")
}

func (m Model) viewTrackInfo(w int) string {
	t := m.track
	if t == nil {
		return lipgloss.JoinVertical(lipgloss.Left,
			m.theme.title.Render("Nothing playing"),
			"",
			lipgloss.NewStyle().Width(w).Foreground(colSubtle).Render(
				"Pick \""+m.cfg.DeviceName+"\" in Spotify Connect on your phone or computer, "+
					"browse your library with 2, search with 3, or press u to play a link."),
		)
	}

	trunc := lipgloss.NewStyle().MaxWidth(w)

	heart := ""
	if m.liked[t.Uri] {
		heart = m.theme.accentTxt.Render("  ♥")
	}
	title := trunc.Render(m.theme.title.Render(t.Name) + heart)
	artist := trunc.Render(m.theme.artist.Render(strings.Join(t.ArtistNames, ", ")))
	album := t.AlbumName
	if len(t.ReleaseDate) >= 4 {
		album += " · " + t.ReleaseDate[:4]
	}
	albumLine := trunc.Render(m.theme.album.Render(album))

	pos := m.position()
	dur := int64(t.Duration)

	stateText := "Playing"
	switch {
	case m.buffering:
		stateText = "Loading"
	case m.stopped:
		stateText = "Stopped"
	case m.paused:
		stateText = "Paused"
	}
	state := m.stateIcon() + " " + m.theme.text.Render(stateText)
	if m.playingDj() {
		state += "  " + m.theme.pill.Render("DJ")
	}

	posStr, durStr := fmtDuration(pos), fmtDuration(dur)
	times := m.theme.dim.Render(posStr) +
		strings.Repeat(" ", max(w-len(posStr)-len(durStr), 1)) +
		m.theme.dim.Render(durStr)

	modes := m.modeBadges()
	status := state + strings.Repeat(" ", max(w-lipgloss.Width(state)-lipgloss.Width(modes), 1)) + modes

	parts := []string{
		title,
		artist,
		albumLine,
		"",
		status,
		m.progressBar(w, pos, dur),
		times,
		"",
		m.volumeBar(w),
	}
	if m.cfg.Lyrics.NowPlayingLine && m.lyrics != nil && len(m.lyrics.Synced) > 0 {
		line := m.currentLyric()
		if line == "" {
			line = "♪"
		}
		parts = append(parts, "", m.theme.album.Foreground(m.theme.accent).Render(ansi.Truncate("“"+line+"”", w, "…")))
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m Model) modeBadges() string {
	on := m.theme.accentTxt
	off := m.theme.dim

	shuffle := off.Render("⇄ shuffle")
	if m.shuffle {
		shuffle = on.Render("⇄ shuffle")
	}

	repeat := off.Render("↻ repeat")
	switch {
	case m.repeatTrack:
		repeat = on.Render("↻ track")
	case m.repeatCtx:
		repeat = on.Render("↻ all")
	}
	return shuffle + "  " + repeat
}

func (m Model) progressBar(w int, pos, dur int64) string {
	if w < 3 {
		return ""
	}
	ratio := 0.0
	if dur > 0 {
		ratio = float64(pos) / float64(dur)
	}
	filled := clamp(int(ratio*float64(w-1)), 0, w-1)
	return m.theme.accentTxt.Render(strings.Repeat("━", filled)+"●") +
		lipgloss.NewStyle().Foreground(colTrack).Render(strings.Repeat("━", w-1-filled))
}

func (m Model) volumeBar(w int) string {
	pct := 0
	if m.volumeMax > 0 {
		pct = int(m.volume * 100 / m.volumeMax)
	}
	// Plain text instead of speaker emoji: emoji widths differ between
	// terminals (notably on Windows) and would break the alignment.
	icon := m.theme.dim.Render("vol")
	label := fmt.Sprintf("%3d%%", pct)
	barW := max(min(w-lipgloss.Width(icon)-len(label)-2, 24), 4)
	filled := barW * pct / 100
	bar := m.theme.accentTxt.Render(strings.Repeat("■", filled)) +
		lipgloss.NewStyle().Foreground(colTrack).Render(strings.Repeat("■", barW-filled))
	return icon + " " + bar + " " + m.theme.dim.Render(label)
}

func fmtDuration(ms int64) string {
	s := ms / 1000
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}
