package ui

import "strings"

// Easter eggs: a track can bring its own shader scene, or its own colour.
// While one plays (with ui.easter_eggs on), the Windows Terminal shader
// shows it whatever effect is set, and the status line greets it once.
// They are meant to be found: keep what they are out of commit messages,
// PRs and release notes.
type shaderEggSpec struct {
	title  string // a part of the track's name, lower case
	style  string // a style in shaderStyles, or "" to keep the one showing
	accent string // "#rrggbb" to use instead of the cover's colour, or ""
	mono   bool   // draw the whole effect in that one colour
	flash  string // said once when the track starts
}

var shaderEggs = []shaderEggSpec{
	// Videoclub's song, the sound of the Napoleon "there's nothing we can
	// do" memes: he stands on a cliff over the sea at dusk.
	{title: "amour plastique", style: "napoleon", flash: "There's nothing we can do…"},
	// Prince: the rain, all of it purple.
	{title: "purple rain", style: "rain", accent: "#a040ff", mono: true, flash: "It's raining purple."},
	// Bowie: an astronaut drifting high above the Earth.
	{title: "space oddity", style: "space", flash: "Signal lost…"},
	// Eiffel 65: whatever is showing, all of it blue.
	{title: "da ba dee", accent: "#2a5cff", mono: true, flash: "Everything is blue."},
	// AC/DC: the synthwave road, all of it red.
	{title: "highway to hell", style: "synthwave", accent: "#ff2a1a", mono: true, flash: "Next exit: hell."},
	// The Weeknd: the synthwave sun in neon red, his eighties.
	{title: "blinding lights", style: "synthwave", accent: "#ff1744", mono: true, flash: "Sunglasses on."},
	// Young Multi: the chains, in the cover's colours.
	{title: "za każdym ra2em", style: "chains", flash: "Chains on."},
	// Billie Eilish: rain in ocean blue.
	{title: "ocean eyes", style: "rain", accent: "#1e90ff", mono: true, flash: "Deep blue."},
}

// activeEgg is the easter egg for the track playing, or nil.
func (m Model) activeEgg() *shaderEggSpec {
	if m.cfg == nil || !m.cfg.UI.EasterEggs || m.track == nil {
		return nil
	}
	name := strings.ToLower(m.track.Name)
	for i := range shaderEggs {
		if strings.Contains(name, shaderEggs[i].title) {
			return &shaderEggs[i]
		}
	}
	return nil
}

// shaderEgg is the easter egg for the track playing: its style index in
// shaderStyles (-1 when it keeps the one showing, or there is none) and
// its greeting ("" when there is none).
func (m Model) shaderEgg() (int, string) {
	e := m.activeEgg()
	if e == nil {
		return -1, ""
	}
	for i, s := range shaderStyles {
		if s == e.style {
			return i, e.flash
		}
	}
	return -1, e.flash
}
