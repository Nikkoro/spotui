package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Nikkoro/spotui/internal/platform"
)

// Small touches, each switchable in [ui]: the progress bar's colours, the
// start-up animation, the heart's sparkle and copying links.

var progressStyles = []string{"plain", "gradient", "shimmer"}

func (m Model) progressStyle() string {
	if m.cfg != nil {
		for _, s := range progressStyles {
			if s == m.cfg.UI.ProgressStyle {
				return s
			}
		}
	}
	return "gradient"
}

type rgb struct{ r, g, b float64 }

func parseRGB(hex string) (rgb, bool) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return rgb{}, false
	}
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return rgb{}, false
	}
	return rgb{float64(v >> 16 & 0xff), float64(v >> 8 & 0xff), float64(v & 0xff)}, true
}

func (c rgb) mix(o rgb, t float64) rgb {
	t = math.Max(0, math.Min(1, t))
	return rgb{c.r + (o.r-c.r)*t, c.g + (o.g-c.g)*t, c.b + (o.b-c.b)*t}
}

func (c rgb) color() lipgloss.Color {
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", int(c.r), int(c.g), int(c.b)))
}

// fancyProgress draws the played part from a deep shade of the accent to
// a light one, with a light running along it for "shimmer".
func (m Model) fancyProgress(filled int) (string, bool) {
	base, ok := parseRGB(m.theme.accentHex)
	if !ok || filled <= 0 {
		return "", false
	}
	dark := base.mix(rgb{0, 0, 0}, 0.45)
	light := base.mix(rgb{255, 255, 255}, 0.35)
	shimmerAt := -100
	if m.progressStyle() == "shimmer" {
		period := filled + 12
		shimmerAt = int(time.Now().UnixMilli()/45)%period - 6
	}

	var sb strings.Builder
	for i := 0; i < filled; i++ {
		t := float64(i) / float64(max(filled-1, 1))
		var c rgb
		if t < 0.5 {
			c = dark.mix(base, t*2)
		} else {
			c = base.mix(light, (t-0.5)*2)
		}
		if d := math.Abs(float64(i - shimmerAt)); d < 3 {
			c = c.mix(rgb{255, 255, 255}, 0.55*(1-d/3))
		}
		sb.WriteString(lipgloss.NewStyle().Foreground(c.color()).Render("━"))
	}
	sb.WriteString(lipgloss.NewStyle().Foreground(light.color()).Bold(true).Render("●"))
	return sb.String(), true
}

// --- start-up

// splash is the start-up screen: an equalizer dancing over the name.
func (m Model) splash(text string) string {
	if m.cfg == nil || !m.cfg.UI.Splash {
		return m.theme.card.Render(m.spinner.View() + m.theme.text.Render(" "+text))
	}
	base, ok := parseRGB(m.theme.accentHex)
	if !ok {
		base = rgb{29, 185, 84}
	}
	const bars, rows = 15, 4
	levels := []rune(" ▁▂▃▄▅▆▇█")
	t := float64(time.Now().UnixMilli()) / 1000
	heights := make([]float64, bars)
	for i := range heights {
		x := float64(i)
		h := 0.55 + 0.3*math.Sin(t*5.1+x*0.7) + 0.15*math.Sin(t*8.3+x*1.9)
		heights[i] = math.Max(0.08, math.Min(1, h)) * rows
	}
	var lines []string
	for r := rows - 1; r >= 0; r-- {
		var sb strings.Builder
		c := base.mix(rgb{255, 255, 255}, float64(r)/rows*0.5)
		style := lipgloss.NewStyle().Foreground(c.color())
		for _, h := range heights {
			fill := h - float64(r)
			idx := int(math.Round(math.Max(0, math.Min(1, fill)) * 8))
			sb.WriteString(style.Render(strings.Repeat(string(levels[idx]), 2)) + " ")
		}
		lines = append(lines, sb.String())
	}
	lines = append(lines, "",
		lipgloss.PlaceHorizontal(bars*3, lipgloss.Center, m.theme.title.Render("♫ spotui")),
		lipgloss.PlaceHorizontal(bars*3, lipgloss.Center, m.theme.dim.Render(text)))
	return m.theme.card.Padding(1, 3).Render(strings.Join(lines, "\n"))
}

// --- the heart

const likeFxLen = 1100 * time.Millisecond

var likeFrames = []string{"♥", "♥ ✦", "♥ ✧ ·", "♥  ⋆ ·", "♥   ·", "♥"}

// heart is the liked mark for uri: sparkling for a moment after a like.
func (m Model) heart(uri string, lead string) string {
	if !m.liked[uri] {
		return ""
	}
	if m.cfg != nil && m.cfg.UI.LikeAnimation && uri == m.likedNowURI {
		if age := time.Since(m.likedNowAt); age >= 0 && age < likeFxLen {
			f := likeFrames[int(age*time.Duration(len(likeFrames))/likeFxLen)]
			return m.theme.accentTxt.Bold(true).Render(lead + f)
		}
	}
	return m.theme.accentTxt.Render(lead + "♥")
}

// --- copying links

// spotifyURL turns spotify:track:ID into its open.spotify.com link.
func spotifyURL(uri string) string {
	parts := strings.Split(uri, ":")
	if len(parts) == 3 && parts[0] == "spotify" {
		return "https://open.spotify.com/" + parts[1] + "/" + parts[2]
	}
	return ""
}

// copyLink copies the selected list entry's link, or the playing track's.
func (m *Model) copyLink() tea.Cmd {
	uri, what := m.currentURI(), "this track"
	if l := m.activeList(); l != nil && m.page != pageNowPlaying {
		if it := l.selected(); it != nil && strings.HasPrefix(it.uri, "spotify:") {
			uri, what = it.uri, it.title
		}
	}
	if uri == "" {
		return nil
	}
	text := uri
	if m.cfg.UI.CopyAs != "uri" {
		if u := spotifyURL(uri); u != "" {
			text = u
		}
	}
	m.setFlash("Copied the link to "+what, false)
	return func() tea.Msg {
		if err := platform.Copy(text); err != nil {
			_ = platform.CopyOSC52(text) // the terminal's own clipboard
		}
		return nil
	}
}
