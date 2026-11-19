package ui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Nikkoro/spotui/internal/engine"
)

const djNewSet = "dj:new-set"

type djPanel struct {
	input   textinput.Model
	list    listModel
	loading bool
	err     error
}

func newDjPanel() djPanel {
	in := textinput.New()
	in.Prompt = "  "
	in.Placeholder = "Ask DJ for something… e.g. “90s hip-hop for a night drive”"
	in.CharLimit = 200
	return djPanel{input: in}
}

type djSuggestionsMsg struct {
	prompts []string
	err     error
}

func (m Model) playingDj() bool { return m.contextURI == engine.DjContextURI }

func (m Model) loadDjSuggestions() tea.Cmd {
	eng := m.eng
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		p, err := eng.DjSuggestions(ctx)
		return djSuggestionsMsg{prompts: p, err: err}
	}
}

// startDj plays DJ, or moves it on to a new set when it already plays.
func (m *Model) startDj() tea.Cmd {
	eng := m.eng
	if m.playingDj() {
		m.setFlash("DJ is picking something different…", false)
	} else {
		m.setFlash("Starting DJ…", false)
	}
	return call("DJ", eng.StartDj)
}

// djKey is D: start DJ, or open the request panel while it plays.
func (m *Model) djKey() tea.Cmd {
	if !m.playingDj() {
		return m.startDj()
	}
	m.overlay = overlayDj
	m.dj.loading, m.dj.err = true, nil
	m.dj.input.SetValue("")
	m.dj.list.setItems([]listItem{{kind: kindTrack, title: "✦ Pick something different", subtitle: "new picks", uri: djNewSet}})
	return tea.Batch(m.dj.input.Focus(), m.loadDjSuggestions())
}

func (m *Model) setDjSuggestions(msg djSuggestionsMsg) {
	m.dj.loading = false
	m.dj.err = msg.err
	items := []listItem{{kind: kindTrack, title: "✦ Pick something different", subtitle: "new picks", uri: djNewSet}}
	for _, p := range msg.prompts {
		items = append(items, listItem{kind: kindTrack, title: p, uri: "dj:prompt:" + p})
	}
	m.dj.list.setItems(items)
}

func (m Model) updateDjKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.overlay = overlayNone
		m.dj.input.Blur()
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "up":
		m.dj.list.move(-1)
		return m, nil
	case "down", "tab":
		m.dj.list.move(1)
		return m, nil
	case "enter":
		prompt := strings.TrimSpace(m.dj.input.Value())
		suggested := false
		if prompt == "" {
			it := m.dj.list.selected()
			if it == nil {
				return m, nil
			}
			if it.uri == djNewSet {
				m.overlay = overlayNone
				m.dj.input.Blur()
				return m, m.startDj()
			}
			prompt, suggested = it.title, true
		}
		m.overlay = overlayNone
		m.dj.input.Blur()
		m.setFlash("Asked DJ: “"+prompt+"” — it'll answer in a moment", false)
		eng := m.eng
		return m, call("Ask DJ", func(ctx context.Context) error { return eng.DjAsk(ctx, prompt, suggested) })
	}
	var cmd tea.Cmd
	m.dj.input, cmd = m.dj.input.Update(msg)
	return m, cmd
}

func (m Model) viewDj(w, h int) string {
	cw := clamp(w-16, 44, 72)
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(m.theme.accent).Width(cw - 2)
	in := m.dj.input
	in.Width = cw - 8

	lines := []string{
		m.theme.pill.Render("DJ") + " " + m.theme.title.Render("What do you want to hear?"),
		"",
		box.Render(m.theme.accentTxt.Render("›") + in.View()),
		"",
	}
	switch {
	case m.dj.loading:
		lines = append(lines, m.dj.list.view(m.theme, cw, 1, ""), m.loadingLine("Getting suggestions…"))
	default:
		lines = append(lines, m.dj.list.view(m.theme, cw, min(len(m.dj.list.items), max(h-14, 3)), ""))
		if m.dj.err != nil {
			lines = append(lines, m.theme.dim.Render("No suggestions right now; type your own."))
		}
	}
	lines = append(lines, "", m.theme.dim.Render("type + enter to ask · ↑↓ + enter for a suggestion · esc close"))
	return m.theme.card.Render(strings.Join(lines, "\n"))
}
