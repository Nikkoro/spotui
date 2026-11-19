package mpris

// BusName and Identity name the MPRIS player (org.mpris.MediaPlayer2.<BusName>).
// Set them before NewServer. spotui patch, see third_party/PATCHES.md.
var (
	BusName  = "go-librespot"
	Identity = "go-librespot"
)
