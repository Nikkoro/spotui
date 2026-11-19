package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// View renders the whole screen.
func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	var body string
	switch m.phase {
	case phaseConnecting:
		body = m.viewConnecting()
	case phaseAuth:
		body = m.viewAuth()
	case phaseFatal:
		body = m.viewFatal()
	default:
		body = m.viewPlayer()
	}

	header := m.viewHeader()
	footer := m.viewFooter()

	avail := m.height - lipgloss.Height(header) - lipgloss.Height(footer) - 2
	centred := lipgloss.Place(m.width-4, max(avail, 1), lipgloss.Center, lipgloss.Center, body)

	return m.theme.app.Render(lipgloss.JoinVertical(lipgloss.Left, header, centred, footer))
}

func (m Model) viewHeader() string {
	left := m.theme.pill.Render("♫ spotui")
	device := m.theme.dim.Render("  " + m.cfg.DeviceName)

	var state string
	switch m.phase {
	case phaseReady:
		state = m.theme.accentTxt.Render("● connected")
		if m.username != "" {
			state += m.theme.dim.Render(" as " + m.username)
		}
	case phaseAuth:
		state = m.theme.dim.Render("○ waiting for login")
	case phaseFatal:
		state = m.theme.errorTxt.Render("✕ stopped")
	default:
		state = m.theme.dim.Render("○ connecting")
	}

	l := left + device
	gap := max(m.width-4-lipgloss.Width(l)-lipgloss.Width(state), 1)
	return l + strings.Repeat(" ", gap) + state
}

func (m Model) viewFooter() string {
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
		return st.Render(m.flash)
	}

	var keys [][2]string
	switch m.phase {
	case phaseAuth:
		keys = [][2]string{{"o", "open link"}, {"q", "quit"}}
	case phaseReady:
		keys = [][2]string{
			{"space", "play/pause"}, {"n/p", "next/prev"}, {"←/→", "seek"},
			{"+/-", "volume"}, {"s", "shuffle"}, {"r", "repeat"},
			{"u", "play link"}, {"[ ]", "pixels"}, {"{ }", "colours"}, {"q", "quit"},
		}
	default:
		keys = [][2]string{{"q", "quit"}}
	}

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, m.theme.key.Render(k[0])+" "+m.theme.dim.Render(k[1]))
	}
	return lipgloss.NewStyle().Width(m.width - 4).Render(strings.Join(parts, m.theme.dim.Render("  ·  ")))
}

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
			m.theme.dim.Render("Details are in spotui.log in your config folder."),
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

func (m Model) viewPlayer() string {
	infoWidth := clamp(m.width-12, 30, 56)

	info := m.viewTrackInfo(infoWidth)

	coverView := ""
	if m.coverPix != nil {
		coverView = m.coverPix.Render()
	} else if m.cfg.Cover.Enabled && m.track != nil {
		coverView = m.coverPlaceholder()
	}

	// Side by side when there is room, cover on top otherwise, no cover on
	// tiny terminals.
	coverW := lipgloss.Width(coverView)
	coverH := lipgloss.Height(coverView)
	infoH := lipgloss.Height(info)
	availH := m.height - 8
	var content string
	switch {
	case coverView == "":
		content = info
	case m.width-12 >= coverW+4+infoWidth:
		content = lipgloss.JoinHorizontal(lipgloss.Center, coverView, "    ", info)
	case availH >= coverH+1+infoH:
		content = lipgloss.JoinVertical(lipgloss.Center, coverView, "", info)
	default:
		content = info
	}

	return m.theme.card.Render(content)
}

func (m Model) coverPlaceholder() string {
	size := m.pixSize
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
				"Pick \""+m.cfg.DeviceName+"\" in Spotify Connect on your phone or computer, or press u to play a link."),
		)
	}

	trunc := lipgloss.NewStyle().MaxWidth(w)

	title := trunc.Render(m.theme.title.Render(t.Name))
	artist := trunc.Render(m.theme.artist.Render(strings.Join(t.ArtistNames, ", ")))
	album := t.AlbumName
	if len(t.ReleaseDate) >= 4 {
		album += " · " + t.ReleaseDate[:4]
	}
	albumLine := trunc.Render(m.theme.album.Render(album))

	pos := m.position()
	dur := int64(t.Duration)

	stateIcon, stateText := "▶", "Playing"
	switch {
	case m.buffering:
		stateIcon, stateText = m.spinner.View(), "Loading"
	case m.stopped:
		stateIcon, stateText = "■", "Stopped"
	case m.paused:
		stateIcon, stateText = "‖", "Paused"
	}
	state := m.theme.accentTxt.Render(stateIcon) + " " + m.theme.text.Render(stateText)

	posStr, durStr := fmtDuration(pos), fmtDuration(dur)
	times := m.theme.dim.Render(posStr) +
		strings.Repeat(" ", max(w-len(posStr)-len(durStr), 1)) +
		m.theme.dim.Render(durStr)

	modes := m.modeBadges()
	status := state + strings.Repeat(" ", max(w-lipgloss.Width(state)-lipgloss.Width(modes), 1)) + modes

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		artist,
		albumLine,
		"",
		status,
		m.progressBar(w, pos, dur),
		times,
		"",
		m.volumeBar(w),
	)
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
