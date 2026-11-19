package ui

import "github.com/charmbracelet/lipgloss"

const defaultAccent = "#1DB954" // Spotify green

// theme holds every style derived from the current accent colour, so the
// whole UI can be re-tinted when the album cover changes.
type theme struct {
	accent lipgloss.Color

	app       lipgloss.Style
	card      lipgloss.Style
	pill      lipgloss.Style
	title     lipgloss.Style
	artist    lipgloss.Style
	album     lipgloss.Style
	dim       lipgloss.Style
	text      lipgloss.Style
	accentTxt lipgloss.Style
	errorTxt  lipgloss.Style
	key       lipgloss.Style
	code      lipgloss.Style
	link      lipgloss.Style
}

var (
	colText   = lipgloss.Color("#E6E6E6")
	colSubtle = lipgloss.Color("#A0A0A8")
	colDim    = lipgloss.Color("#5C5C66")
	colTrack  = lipgloss.Color("#34343C")
	colError  = lipgloss.Color("#FF5F6D")
	colDark   = lipgloss.Color("#101014")
	colSelBg  = lipgloss.Color("#24242C")
)

func newTheme(accentHex string) theme {
	accent := lipgloss.Color(accentHex)
	return theme{
		accent: accent,

		app: lipgloss.NewStyle().Padding(1, 2),
		card: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(accent).
			Padding(1, 3),
		pill: lipgloss.NewStyle().
			Background(accent).
			Foreground(colDark).
			Bold(true).
			Padding(0, 1),
		title:     lipgloss.NewStyle().Foreground(accent).Bold(true),
		artist:    lipgloss.NewStyle().Foreground(colText).Bold(true),
		album:     lipgloss.NewStyle().Foreground(colSubtle).Italic(true),
		dim:       lipgloss.NewStyle().Foreground(colDim),
		text:      lipgloss.NewStyle().Foreground(colText),
		accentTxt: lipgloss.NewStyle().Foreground(accent),
		errorTxt:  lipgloss.NewStyle().Foreground(colError),
		key:       lipgloss.NewStyle().Foreground(accent).Bold(true),
		code: lipgloss.NewStyle().
			Foreground(colDark).
			Background(accent).
			Bold(true).
			Padding(0, 2),
		link: lipgloss.NewStyle().Foreground(accent).Underline(true),
	}
}
