package ui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// defaultAccent is the palette accent, used when no cover colour applies.
var defaultAccent = "#1DB954" // Spotify green

// theme holds every style derived from the current accent colour, so the
// whole UI can be re-tinted when the album cover changes.
type theme struct {
	accent    lipgloss.Color
	accentHex string

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

type palette struct {
	accent, text, subtle, dim, track, selBg, dark string
}

// Palettes selectable with `theme` in config.toml. The accent is replaced
// by the cover colour while dynamic_accent is on.
var palettes = map[string]palette{
	"spotify":     {"#1DB954", "#E6E6E6", "#A0A0A8", "#5C5C66", "#34343C", "#24242C", "#101014"},
	"catppuccin":  {"#CBA6F7", "#CDD6F4", "#A6ADC8", "#6C7086", "#313244", "#292C3C", "#1E1E2E"},
	"gruvbox":     {"#FABD2F", "#EBDBB2", "#A89984", "#665C54", "#3C3836", "#32302F", "#1D2021"},
	"nord":        {"#88C0D0", "#ECEFF4", "#D8DEE9", "#4C566A", "#3B4252", "#353C4A", "#2E3440"},
	"rose-pine":   {"#EBBCBA", "#E0DEF4", "#908CAA", "#6E6A86", "#26233A", "#21202E", "#191724"},
	"tokyo-night": {"#7AA2F7", "#C0CAF5", "#A9B1D6", "#565F89", "#292E42", "#24283B", "#1A1B26"},
}

var overrides map[string]string

// SetColorOverrides sets per-colour overrides from config.toml's [colors],
// applied on top of whichever palette is active. It returns the keys it
// did not understand.
func SetColorOverrides(c map[string]string) (unknown []string) {
	overrides = map[string]string{}
	for k, v := range c {
		k = strings.ToLower(strings.TrimSpace(k))
		switch k {
		case "accent", "text", "subtle", "dim", "track", "selection", "background_text":
		default:
			unknown = append(unknown, k)
			continue
		}
		if !hexColor.MatchString(v) {
			unknown = append(unknown, k+"="+v)
			continue
		}
		overrides[k] = v
	}
	applyOverrides()
	return unknown
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func applyOverrides() {
	set := func(key string, dst *lipgloss.Color) {
		if v, ok := overrides[key]; ok {
			*dst = lipgloss.Color(v)
		}
	}
	if v, ok := overrides["accent"]; ok {
		defaultAccent = v
	}
	set("text", &colText)
	set("subtle", &colSubtle)
	set("dim", &colDim)
	set("track", &colTrack)
	set("selection", &colSelBg)
	set("background_text", &colDark)
}

// ApplyPalette switches the base colours; unknown names keep the default.
// Call before the UI starts.
func ApplyPalette(name string) bool {
	p, ok := palettes[name]
	if !ok {
		return false
	}
	defaultAccent = p.accent
	colText = lipgloss.Color(p.text)
	colSubtle = lipgloss.Color(p.subtle)
	colDim = lipgloss.Color(p.dim)
	colTrack = lipgloss.Color(p.track)
	colSelBg = lipgloss.Color(p.selBg)
	colDark = lipgloss.Color(p.dark)
	applyOverrides()
	return true
}

func newTheme(accentHex string) theme {
	accent := lipgloss.Color(accentHex)
	return theme{
		accent:    accent,
		accentHex: accentHex,

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
