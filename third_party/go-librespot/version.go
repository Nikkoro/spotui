package go_librespot

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
)

const SpotifyVersionCode = 127700358

var version string

// Extract and return the commit hash stored in the binary, if available.
func commitHash() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				return setting.Value
			}
		}
	}
	return ""
}

func VersionNumberString() string {
	if len(version) > 0 {
		return strings.TrimPrefix(version, "v")
	} else if commit := commitHash(); len(commit) >= 8 {
		return commit[:8]
	} else {
		return "dev"
	}
}

func SpotifyLikeClientVersion() string {
	if len(version) > 0 {
		if commit := commitHash(); len(commit) >= 8 {
			return fmt.Sprintf("%s.g%s", version, commit[:8])
		} else {
			return version
		}
	}

	// spotui patch: without a release version, introduce ourselves as the
	// desktop client librespot does. Spotify gates features such as DJ on
	// the client version, and answers "0.0.0" with 404 for them.
	return SpotifySemanticVersion
}

// SpotifySemanticVersion is a desktop client version Spotify serves every
// feature to (the one librespot uses).
const SpotifySemanticVersion = "1.2.52.442"

// SpotifyUserAgent is how Spotify's desktop client names itself to spclient.
func SpotifyUserAgent() string {
	platform := "Linux"
	switch runtime.GOOS {
	case "windows":
		platform = "Win32"
	case "darwin":
		platform = "OSX"
	}
	return fmt.Sprintf("Spotify/%d %s/0 (%s)", SpotifyVersionCode, platform, VersionString())
}

func VersionString() string {
	return fmt.Sprintf("go-librespot %s", VersionNumberString())
}

func SystemInfoString() string {
	return fmt.Sprintf("%s; Go %s (%s %s)", VersionString(), runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

func UserAgent() string {
	return fmt.Sprintf("go-librespot/%s Go/%s", VersionNumberString(), runtime.Version())
}
