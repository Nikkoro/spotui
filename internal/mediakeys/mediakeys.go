// Package mediakeys listens for the keyboard's media keys where the
// platform has no richer integration (Linux uses MPRIS instead).
package mediakeys

// Key is a media key.
type Key int

const (
	PlayPause Key = iota
	Next
	Prev
	Stop
)
