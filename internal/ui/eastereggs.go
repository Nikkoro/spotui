package ui

import "strings"

// Easter eggs: a track can bring its own shader scene. While one plays
// (with ui.easter_eggs on), the Windows Terminal shader shows that scene
// whatever effect is set, and the status line greets it once.
var shaderEggs = []struct {
	title string // a part of the track's name, lower case
	style string // a style in shaderStyles
	flash string // said once when the track starts
}{
	// Videoclub's song, the sound of the Napoleon "there's nothing we can
	// do" memes: he stands on a cliff over the sea at dusk.
	{"amour plastique", "napoleon", "There's nothing we can do…"},
}

// shaderEgg is the easter egg for the track playing: its style index in
// shaderStyles and its greeting, or -1.
func (m Model) shaderEgg() (int, string) {
	if m.cfg == nil || !m.cfg.UI.EasterEggs || m.track == nil {
		return -1, ""
	}
	name := strings.ToLower(m.track.Name)
	for _, e := range shaderEggs {
		if strings.Contains(name, e.title) {
			for i, s := range shaderStyles {
				if s == e.style {
					return i, e.flash
				}
			}
		}
	}
	return -1, ""
}
