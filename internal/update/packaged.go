package update

import (
	"strings"
)

// Packaged tells whether the binary at exe was installed by a package
// manager, which then owns updates: its name and the command that updates
// spotui. ok is false for a binary spotui may replace itself.
func Packaged(exe string) (manager, command string, ok bool) {
	p := strings.ToLower(strings.ReplaceAll(exe, `\`, "/"))
	switch {
	case strings.Contains(p, "/scoop/apps/"):
		return "Scoop", "scoop update spotui", true
	case strings.Contains(p, "/microsoft/winget/packages/"):
		return "winget", "winget upgrade spotui", true
	case strings.Contains(p, "/cellar/") || strings.Contains(p, "/linuxbrew/"):
		return "Homebrew", "brew upgrade spotui", true
	case p == "/usr/bin/spotui":
		return "your system's package manager", "yay -Syu spotui-bin (or your AUR helper)", true
	}
	return "", "", false
}
