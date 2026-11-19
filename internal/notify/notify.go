// Package notify shows desktop notifications for track changes.
package notify

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
)

// Notifier shows a notification, replacing the previous one where the
// platform allows.
type Notifier interface {
	Notify(ctx context.Context, title, body, iconPath string) error
}

// New picks the best notifier for this platform, or nil if there is none.
// isWSL selects Windows toasts through powershell.exe from inside WSL.
func New(isWSL bool) Notifier {
	switch {
	case runtime.GOOS == "windows":
		return &toastNotifier{powershell: "powershell.exe"}
	case isWSL:
		if _, err := exec.LookPath("powershell.exe"); err == nil {
			return &toastNotifier{powershell: "powershell.exe", wsl: true}
		}
		return nil
	case runtime.GOOS == "linux":
		return newDBusNotifier()
	}
	return nil
}

// toastNotifier shows Windows toast notifications via PowerShell, borrowing
// PowerShell's registered app id since a console program has none.
type toastNotifier struct {
	powershell string
	wsl        bool
}

const toastScript = `
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null
$xml = New-Object Windows.Data.Xml.Dom.XmlDocument
$xml.LoadXml(@'
<toast duration="short"><visual><binding template="ToastGeneric">%IMAGE%<text>%TITLE%</text><text>%BODY%</text></binding></visual><audio silent="true"/></toast>
'@)
$toast = New-Object Windows.UI.Notifications.ToastNotification $xml
$toast.Tag = "spotui"
$toast.Group = "spotui"
$app = '{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe'
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($app).Show($toast)
`

func (t *toastNotifier) Notify(ctx context.Context, title, body, iconPath string) error {
	image := ""
	if iconPath != "" {
		if t.wsl {
			if out, err := exec.CommandContext(ctx, "wslpath", "-w", iconPath).Output(); err == nil {
				iconPath = strings.TrimSpace(string(out))
			} else {
				iconPath = ""
			}
		}
		if iconPath != "" {
			image = `<image placement="appLogoOverride" src="` + xmlEscape(iconPath) + `"/>`
		}
	}
	script := strings.NewReplacer(
		"%IMAGE%", image,
		"%TITLE%", xmlEscape(title),
		"%BODY%", xmlEscape(body),
	).Replace(toastScript)

	cmd := exec.CommandContext(ctx, t.powershell, "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", "-")
	cmd.Stdin = strings.NewReader(script)
	hideWindow(cmd)
	return cmd.Run()
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;").Replace(s)
}
