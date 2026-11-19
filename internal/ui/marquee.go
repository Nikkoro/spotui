package ui

import (
	"time"

	"github.com/charmbracelet/x/ansi"
)

// marquee scrolls text that does not fit in w columns: it rests at the
// start, slides left one column per step, rests at the end and starts over.
// Plain text only (style it afterwards).
func marquee(text string, w int, now time.Time) string {
	if w <= 0 || ansi.StringWidth(text) <= w {
		return text
	}
	runes := []rune(text)
	overflow := len(runes) - w
	const step = 250 * time.Millisecond
	const rest = 8 // steps paused at each end
	cycle := overflow + 2*rest
	pos := int(now.UnixMilli()/step.Milliseconds()) % cycle
	switch {
	case pos < rest:
		pos = 0
	case pos < rest+overflow:
		pos -= rest
	default:
		pos = overflow
	}
	// Fit by display width, which matters for wide characters.
	return ansi.Truncate(string(runes[pos:]), w, "")
}
