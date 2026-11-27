package update

import "testing"

func TestPackaged(t *testing.T) {
	for exe, want := range map[string]string{
		`C:\Users\niko\scoop\apps\spotui\0.3.1\spotui.exe`:                                          "Scoop",
		`C:\Users\niko\AppData\Local\Microsoft\WinGet\Packages\Nikkoro.spotui_Microsoft\spotui.exe`: "winget",
		"/home/linuxbrew/.linuxbrew/Cellar/spotui/0.3.1/bin/spotui":                                 "Homebrew",
		"/usr/bin/spotui":              "your system's package manager",
		"/home/niko/.local/bin/spotui": "",
		`C:\Users\niko\bin\spotui.exe`: "",
		"/usr/local/bin/spotui":        "",
	} {
		got, cmd, ok := Packaged(exe)
		if got != want || ok != (want != "") || (ok && cmd == "") {
			t.Errorf("Packaged(%q) = %q, %q, %v; want %q", exe, got, cmd, ok, want)
		}
	}
}
