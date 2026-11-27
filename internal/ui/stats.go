package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Nikkoro/spotui/internal/history"
)

// Stats: what you listened to, from the local history (internal/history),
// shown as Library → Stats. The recorder is fed after every update from
// the same state `spotui now` reads.

// statsTop is how many tracks and artists the Stats tab lists.
const statsTop = 50

// WithHistory records plays with r (nil: no history at all).
func (m Model) WithHistory(r *history.Recorder) Model {
	m.history = r
	return m
}

// recordHistory hands the recorder what plays now.
func (m Model) recordHistory() {
	if m.history == nil || m.cfg == nil {
		return
	}
	var n history.Now
	if m.cfg.Stats.History {
		st := m.remoteState()
		n = history.Now{URI: st.URI, Title: st.Title, Artists: st.Artists, Album: st.Album,
			DurationMs: st.DurationMs, Playing: st.Status == "playing"}
	}
	m.history.Observe(n, m.username, time.Now())
}

func (m Model) loadStats() tea.Cmd {
	dir, username := "", m.username
	if m.cfg != nil {
		dir = m.cfg.ConfigDir
	}
	return func() tea.Msg {
		plays, err := history.Load(history.Path(dir, username))
		if err != nil {
			return libLoadedMsg{tab: libStats, err: err}
		}
		st := history.Summarize(plays, time.Now(), statsTop)
		items := make([]listItem, 0, len(st.TopTracks))
		for _, c := range st.TopTracks {
			sub := strings.Join(c.Artists, ", ")
			if c.Album != "" {
				sub += " · " + c.Album
			}
			items = append(items, listItem{kind: kindTrack, title: c.Title, subtitle: sub,
				right: fmt.Sprintf("%d× · %s", c.Plays, fmtListened(c.Ms)), uri: c.Key})
		}
		return libLoadedMsg{tab: libStats, items: items, stats: &st}
	}
}

// viewStats is the Stats tab: totals, streak and top artists above the
// most played tracks of the last 30 days.
func (m Model) viewStats(l *listModel, w, h int) string {
	st := m.lib.stats
	off := m.cfg != nil && !m.cfg.Stats.History
	switch {
	case m.lib.err[libStats] != nil:
		return m.errorLine(m.lib.err[libStats])
	case st == nil:
		return m.loadingLine("Reading your history…")
	case st.All.Plays == 0 && off:
		return m.theme.dim.Render("The listening history is off: turn it on in Settings → Listening history.")
	case st.All.Plays == 0:
		return m.theme.dim.Render("Nothing recorded yet. Whatever plays for 30 s (or half a short track) counts;\nit stays on this computer.")
	}
	dim, acc := m.theme.dim, m.theme.accentTxt
	total := func(label string, t history.Totals) string {
		return dim.Render(label+" ") + acc.Render(fmtListened(t.Ms)) + dim.Render(fmt.Sprintf(" · %d plays", t.Plays))
	}
	lines := []string{
		total("Today", st.Today) + "   " + total("7 days", st.Week) + "   " + total("30 days", st.Month),
		total("All time", st.All) + dim.Render(fmt.Sprintf(" · %d tracks", st.All.Tracks)),
	}
	extra := dim.Render("Streak ") + acc.Render(plural(st.Streak, "day"))
	if st.PeakHour >= 0 {
		extra += dim.Render("   Most listened ") + acc.Render(fmt.Sprintf("%02d:00–%02d:00", st.PeakHour, (st.PeakHour+1)%24))
	}
	if off {
		extra += dim.Render("   (history off: nothing new is recorded)")
	}
	lines = append(lines, extra)
	if len(st.TopArtists) > 0 {
		var names []string
		for _, a := range st.TopArtists[:min(5, len(st.TopArtists))] {
			names = append(names, a.Title+dim.Render(" "+fmtListened(a.Ms)))
		}
		lines = append(lines, dim.Render("Top artists, 30 days  ")+strings.Join(names, dim.Render(" · ")))
	}
	lines = append(lines, "", dim.Render("Most played, 30 days"))
	head := strings.Join(lines, "\n")
	return head + "\n" + m.listBlock(l, w, h-len(lines), 3)
}

// fmtListened is a listening time: 42m, 3h 05m, 12 s under a minute.
func fmtListened(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d s", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
}
