package ui

import "strings"

// Easter eggs: a track can bring its own shader scene, or its own colour.
// While one plays (with ui.easter_eggs on), the Windows Terminal shader
// shows it whatever effect is set, and the status line greets it once.
// They are meant to be found: keep what they are out of commit messages,
// PRs and release notes.
type shaderEggSpec struct {
	title  string // a part of the track's name, lower case
	style  string // a style in shaderStyles
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
// shaderStyles and its greeting, or -1.
func (m Model) shaderEgg() (int, string) {
	if e := m.activeEgg(); e != nil {
		for i, s := range shaderStyles {
			if s == e.style {
				return i, e.flash
			}
		}
	}
	return -1, ""
}
