package ui

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Actions that can be rebound in config.toml under [keys], with their
// default key. A binding adds a key; the default keeps working.
var defaultKeys = map[string]string{
	"play_pause":       " ",
	"next":             "n",
	"previous":         "p",
	"seek_forward":     "right",
	"seek_backward":    "left",
	"volume_up":        "+",
	"volume_down":      "-",
	"shuffle":          "s",
	"repeat":           "r",
	"like":             "f",
	"add_to_queue":     "a",
	"add_to_playlist":  "P",
	"go_to_album":      "A",
	"go_to_artist":     "I",
	"devices":          "d",
	"dj":               "D",
	"sleep_timer":      "z",
	"visualizer":       "v",
	"cover_mode":       "c",
	"visualizer_style": "V",
	"layout":           "L",
	"settings":         ",",
	"play_link":        "u",
	"filter":           "/",
	"help":             "?",
	"quit":             "q",
	"now_playing":      "1",
	"library":          "2",
	"search":           "3",
	"queue":            "4",
	"lyrics":           "5",
	"reload":           "R",
}

// ActionNames lists the bindable actions, for docs and validation.
func ActionNames() []string {
	names := make([]string, 0, len(defaultKeys))
	for k := range defaultKeys {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// buildKeymap turns [keys] (action → key, or several keys separated by
// commas) into a lookup from the user's key to the default key.
func buildKeymap(bindings map[string]string) (remap map[string]string, unknown []string) {
	remap = map[string]string{}
	for action, keys := range bindings {
		def, ok := defaultKeys[action]
		if !ok {
			unknown = append(unknown, action)
			continue
		}
		for _, k := range strings.Split(keys, ",") {
			k = strings.TrimSpace(k)
			if k == "space" {
				k = " "
			}
			if k != "" && k != def {
				remap[k] = def
			}
		}
	}
	sort.Strings(unknown)
	return remap, unknown
}

var namedKeys = map[string]tea.KeyType{
	" ":         tea.KeySpace,
	"enter":     tea.KeyEnter,
	"esc":       tea.KeyEsc,
	"tab":       tea.KeyTab,
	"backspace": tea.KeyBackspace,
	"up":        tea.KeyUp,
	"down":      tea.KeyDown,
	"left":      tea.KeyLeft,
	"right":     tea.KeyRight,
	"home":      tea.KeyHome,
	"end":       tea.KeyEnd,
	"pgup":      tea.KeyPgUp,
	"pgdown":    tea.KeyPgDown,
}

// keyMsg builds the key event a default key string stands for.
func keyMsg(k string) tea.KeyMsg {
	if t, ok := namedKeys[k]; ok {
		return tea.KeyMsg{Type: t}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

func (m Model) remapKey(msg tea.KeyMsg) tea.KeyMsg {
	if def, ok := m.keymap[msg.String()]; ok {
		return keyMsg(def)
	}
	return msg
}

// CheckKeys validates [keys] for the startup log.
func CheckKeys(bindings map[string]string) (int, []string) {
	remap, unknown := buildKeymap(bindings)
	return len(remap), unknown
}
