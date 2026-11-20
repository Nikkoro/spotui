package ui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Nikkoro/spotui/internal/update"
)

type updateMsg struct {
	tag string
	pre bool
}

// checkUpdate looks for a newer release a little after start: once a day at
// most (the answer is cached), never for a development build.
func (m Model) checkUpdate() tea.Cmd {
	if m.cfg == nil || !m.cfg.Update.Check || m.cfg.CacheDir == "" || !update.Newer(Version, "") {
		return nil
	}
	dir, pre := m.cfg.CacheDir, m.cfg.Update.Prerelease
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		rels, err := update.NewChecker(dir, Version).Releases(ctx, false)
		if err != nil {
			return nil
		}
		if r := update.Pick(rels, pre); r != nil && update.Newer(r.Tag, Version) {
			return updateMsg{tag: r.Tag, pre: r.Prerelease}
		}
		return nil
	})
}

func (m *Model) handleUpdateMsg(msg updateMsg) {
	m.updateTag = msg.tag
	how := "spotui update"
	if msg.pre && !m.cfg.Update.Prerelease {
		how += " --pre"
	}
	m.setFlash("spotui "+msg.tag+" is out · run `"+how+"` to install it", false)
}
