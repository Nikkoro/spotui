package ui

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/devgianlu/go-librespot/daemon"

	"github.com/Nikkoro/spotui/internal/lyrics"
)

// Version is set by main for the LRCLIB user agent.
var Version = "dev"

const lyricsOffsetStep = 250 * time.Millisecond

type lyricsMsg struct {
	uri string
	l   *lyrics.Lyrics
	err error
}

// lyricsDebounce waits before asking LRCLIB, so skipping through a playlist
// does not fire a request per track.
const lyricsDebounce = 1200 * time.Millisecond

type lyricsDueMsg struct{ uri string }

func (m *Model) fetchLyrics(t *daemon.ApiTrack) tea.Cmd {
	m.lyrics, m.lyricsErr, m.lyricsScroll = nil, nil, 0
	m.lyricsURI = t.Uri
	m.lyricsTrack = t
	if m.lyr == nil || t.Uri == "" || strings.HasPrefix(t.Uri, "spotify:episode:") {
		m.lyricsLoading = false
		return nil
	}
	m.lyricsLoading = true
	uri := t.Uri
	return tea.Tick(lyricsDebounce, func(time.Time) tea.Msg { return lyricsDueMsg{uri: uri} })
}

// startLyricsFetch runs the lookup for the still-current track.
func (m Model) startLyricsFetch() tea.Cmd {
	t := m.lyricsTrack
	if t == nil || m.lyr == nil {
		return nil
	}
	client := m.lyr
	q := lyrics.Query{
		URI:      t.Uri,
		Track:    t.Name,
		Album:    t.AlbumName,
		Duration: time.Duration(t.Duration) * time.Millisecond,
	}
	if len(t.ArtistNames) > 0 {
		q.Artist = t.ArtistNames[0]
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		l, err := client.Get(ctx, q)
		return lyricsMsg{uri: q.URI, l: l, err: err}
	}
}

func (m *Model) handleLyricsMsg(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case lyricsDueMsg:
		if msg.uri != m.lyricsURI {
			return nil // skipped on before the lookup was due
		}
		return m.startLyricsFetch()
	case lyricsMsg:
		if msg.uri != m.lyricsURI {
			return nil
		}
		m.lyrics, m.lyricsErr = msg.l, msg.err
		m.lyricsLoading = false
		// Backed off by LRCLIB: try again once allowed, if still playing.
		var rl *lyrics.RateLimitedError
		if errors.As(msg.err, &rl) {
			uri := msg.uri
			return tea.Tick(time.Until(rl.Until)+time.Second, func(time.Time) tea.Msg { return lyricsDueMsg{uri: uri} })
		}
	}
	return nil
}

// lyricsPos is the playback position adjusted by the user's sync offset.
func (m Model) lyricsPos() time.Duration {
	return time.Duration(m.position())*time.Millisecond + m.lyricsOffset
}

// currentLyric returns the line being sung, if synced lyrics are known.
func (m Model) currentLyric() string {
	if m.lyrics == nil || len(m.lyrics.Synced) == 0 {
		return ""
	}
	i := lyrics.Index(m.lyrics.Synced, m.lyricsPos())
	if i < 0 {
		return ""
	}
	return m.lyrics.Synced[i].Text
}

func (m Model) handleLyricsKey(key string) (bool, tea.Model, tea.Cmd) {
	switch key {
	case ">", ".":
		m.lyricsOffset += lyricsOffsetStep
		m.setFlash("Lyrics offset "+fmtOffset(m.lyricsOffset), false)
	case "<", ",":
		m.lyricsOffset -= lyricsOffsetStep
		m.setFlash("Lyrics offset "+fmtOffset(m.lyricsOffset), false)
	case "0":
		m.lyricsOffset = 0
		m.setFlash("Lyrics offset reset", false)
	case "down", "j":
		m.lyricsScroll++
	case "up", "k":
		m.lyricsScroll = max(m.lyricsScroll-1, 0)
	default:
		return false, m, nil
	}
	return true, m, nil
}

func fmtOffset(d time.Duration) string {
	s := d.Round(10 * time.Millisecond).String()
	if d > 0 {
		s = "+" + s
	}
	return s
}

func (m Model) viewLyrics(w, h int) string {
	if m.track == nil {
		return m.theme.dim.Render("Nothing playing.")
	}

	right := ""
	switch {
	case m.lyrics != nil && len(m.lyrics.Synced) > 0:
		right = "synced"
	case m.lyrics != nil && len(m.lyrics.Plain) > 0:
		right = "not synced"
	}
	if m.lyrics != nil && m.lyrics.Source != "" {
		right += " · " + m.lyrics.Source
	}
	if m.lyricsOffset != 0 {
		right += " · " + fmtOffset(m.lyricsOffset)
	}
	head := m.pageTitle(m.track.Name+m.theme.dim.Render("  "+strings.Join(m.track.ArtistNames, ", ")), right, w)

	bodyH := h - 2
	var body string
	switch {
	case m.lyr == nil:
		body = m.centreText(w, bodyH, m.theme.dim.Render("Lyrics are disabled in config.toml."))
	case m.lyricsLoading:
		body = m.centreText(w, bodyH, m.loadingLine("Looking for lyrics…"))
	case errors.Is(m.lyricsErr, lyrics.ErrNotFound) || (m.lyricsErr == nil && m.lyrics.Empty()):
		body = m.centreText(w, bodyH, m.theme.dim.Render("No lyrics found for this track."))
	case m.lyricsErr != nil:
		body = m.centreText(w, bodyH, m.errorLine(m.lyricsErr))
	case m.lyrics.Instrumental && len(m.lyrics.Synced) == 0:
		body = m.centreText(w, bodyH, m.theme.accentTxt.Render("♪  Instrumental  ♪"))
	case len(m.lyrics.Synced) > 0:
		body = m.viewSyncedLyrics(w, bodyH)
	default:
		body = m.viewPlainLyrics(w, bodyH)
	}
	return lipgloss.JoinVertical(lipgloss.Left, head, "", body)
}

func (m Model) centreText(w, h int, s string) string {
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, s)
}

// viewSyncedLyrics keeps the current line about 40% down the page, with
// sung lines fading out above and upcoming ones below.
func (m Model) viewSyncedLyrics(w, h int) string {
	lines := m.lyrics.Synced
	cur := lyrics.Index(lines, m.lyricsPos())
	anchor := h * 2 / 5

	next := m.theme.text
	past := m.theme.dim
	far := lipgloss.NewStyle().Foreground(colTrack)

	rows := make([]string, h)
	for r := 0; r < h; r++ {
		i := cur + (r - anchor)
		if cur < 0 {
			i = r - anchor // before the first line: show it below the anchor
		}
		if i < 0 || i >= len(lines) {
			continue
		}
		text := lines[i].Text
		if text == "" {
			text = "♪"
		}
		text = ansi.Truncate(text, w-4, "…")

		var st lipgloss.Style
		switch d := i - cur; {
		case d == 0:
			// Karaoke: the line fills in as it is sung, up to the next one.
			rows[r] = lipgloss.PlaceHorizontal(w, lipgloss.Center, m.karaokeLine(text, i))
			continue
		case d < 0 && d >= -2:
			st = past
		case d < 0:
			st = far
		case d <= 3:
			st = next
		default:
			st = past
		}
		rows[r] = lipgloss.PlaceHorizontal(w, lipgloss.Center, st.Render(text))
	}
	return strings.Join(rows, "\n")
}

// karaokeLine renders the current line, the sung part in the accent.
func (m Model) karaokeLine(text string, i int) string {
	lines := m.lyrics.Synced
	start := lines[i].At
	end := start + 4*time.Second
	if i+1 < len(lines) {
		end = lines[i+1].At
	}
	progress := 1.0
	if span := end - start; span > 0 {
		progress = float64(m.lyricsPos()-start) / float64(span)
	}
	progress = math.Max(0, math.Min(1, progress))

	runes := []rune(text)
	n := int(math.Round(progress * float64(len(runes))))
	sung := m.theme.title.Bold(true).Render(string(runes[:n]))
	rest := m.theme.text.Bold(true).Render(string(runes[n:]))
	return m.theme.accentTxt.Render("› ") + sung + rest + m.theme.accentTxt.Render(" ‹")
}

func (m Model) viewPlainLyrics(w, h int) string {
	lines := m.lyrics.Plain
	start := min(m.lyricsScroll, max(len(lines)-h, 0))
	rows := make([]string, 0, h)
	for i := start; i < len(lines) && len(rows) < h; i++ {
		rows = append(rows, lipgloss.PlaceHorizontal(w, lipgloss.Center,
			m.theme.text.Render(ansi.Truncate(lines[i], w-4, "…"))))
	}
	return strings.Join(rows, "\n")
}
