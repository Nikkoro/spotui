package ui

import (
	"context"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/devgianlu/go-librespot/daemon"
)

// notifyDelay lets the cover arrive and swallows quick skips.
const notifyDelay = 1500 * time.Millisecond

type notifyDueMsg struct{ uri string }

// notifyTrack schedules a notification and the track change hook if t is
// a new track. It must run before setTrack updates m.track.
func (m *Model) notifyTrack(t *daemon.ApiTrack) tea.Cmd {
	notifying := m.notifier != nil && !m.notifyOff
	hook := strings.TrimSpace(m.cfg.Hooks.OnTrackChange) != ""
	if (!notifying && !hook) || t.Uri == "" || (m.track != nil && m.track.Uri == t.Uri) {
		return nil
	}
	uri := t.Uri
	return tea.Tick(notifyDelay, func(time.Time) tea.Msg { return notifyDueMsg{uri: uri} })
}

func (m Model) sendNotification(uri string) tea.Cmd {
	t := m.track
	if m.notifier == nil || m.notifyOff || t == nil || t.Uri != uri {
		return nil
	}

	icon := ""
	if m.coverImg != nil {
		path := filepath.Join(m.cfg.CacheDir, "notify-cover.png")
		if f, err := os.Create(path); err == nil {
			if png.Encode(f, m.coverImg) == nil {
				icon = path
			}
			_ = f.Close()
		}
	}

	title := t.Name
	body := strings.Join(t.ArtistNames, ", ")
	if t.AlbumName != "" {
		body += " — " + t.AlbumName
	}
	n := m.notifier
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = n.Notify(ctx, title, body, icon)
		return nil
	}
}
