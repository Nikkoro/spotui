// Package platform holds the small OS-specific helpers.
package platform

import (
	"os/exec"
	"runtime"

	"github.com/Nikkoro/spotui/internal/config"
)

// OpenURL opens url in the user's browser. Inside WSL the Windows browser is
// used, since a Linux one usually is not installed.
func OpenURL(url string) error {
	var cmd *exec.Cmd
	switch {
	case runtime.GOOS == "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case runtime.GOOS == "darwin":
		cmd = exec.Command("open", url)
	case config.IsWSL():
		if path, err := exec.LookPath("wslview"); err == nil {
			cmd = exec.Command(path, url)
		} else {
			// rundll32 avoids cmd.exe mangling '&' in query strings.
			cmd = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", url)
		}
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
