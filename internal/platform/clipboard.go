package platform

import (
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/Nikkoro/spotui/internal/config"
)

// ErrNoClipboard means no clipboard tool was found; CopyOSC52 is the way left.
var ErrNoClipboard = errors.New("no clipboard tool found")

// Copy puts text on the system clipboard with the platform's tool: clip on
// Windows and in WSL, wl-copy, xclip or xsel elsewhere.
func Copy(text string) error {
	var cands [][]string
	switch {
	case runtime.GOOS == "windows":
		cands = [][]string{{"clip"}}
	case runtime.GOOS == "darwin":
		cands = [][]string{{"pbcopy"}}
	case config.IsWSL():
		cands = [][]string{{"clip.exe"}}
	default:
		cands = [][]string{{"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}}
	}
	for _, c := range cands {
		path, err := exec.LookPath(c[0])
		if err != nil {
			continue
		}
		cmd := exec.Command(path, c[1:]...)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}
	return ErrNoClipboard
}

// CopyOSC52 asks the terminal itself to set the clipboard (OSC 52), which
// works over SSH and in most modern terminals.
func CopyOSC52(text string) error {
	_, err := os.Stdout.WriteString("\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text)) + "\a")
	return err
}
