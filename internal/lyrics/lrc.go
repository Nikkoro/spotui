// Package lyrics fetches and times synced lyrics.
package lyrics

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Line is one timed lyric line.
type Line struct {
	At   time.Duration
	Text string
}

// Lyrics for one track. Synced is empty when only plain text is known.
type Lyrics struct {
	Synced       []Line
	Plain        []string
	Instrumental bool
	Source       string
}

// Empty reports whether there is nothing to show.
func (l *Lyrics) Empty() bool {
	return l == nil || (len(l.Synced) == 0 && len(l.Plain) == 0 && !l.Instrumental)
}

var (
	timeTag   = regexp.MustCompile(`\[(\d{1,3}):(\d{1,2})(?:[.:](\d{1,3}))?\]`)
	offsetTag = regexp.MustCompile(`(?i)^\[offset:\s*([+-]?\d+)\s*\]$`)
)

// ParseLRC parses LRC text. Lines may carry several timestamps
// ("[00:12.00][01:30.50]chorus"); metadata tags are skipped and [offset:]
// is honoured (positive means lyrics appear earlier).
func ParseLRC(text string) []Line {
	var out []Line
	var offset time.Duration

	for _, raw := range strings.Split(text, "\n") {
		raw = strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if raw == "" {
			continue
		}
		if m := offsetTag.FindStringSubmatch(raw); m != nil {
			if ms, err := strconv.Atoi(m[1]); err == nil {
				offset = time.Duration(ms) * time.Millisecond
			}
			continue
		}

		var stamps []time.Duration
		rest := raw
		for {
			loc := timeTag.FindStringSubmatchIndex(rest)
			if loc == nil || loc[0] != 0 {
				break
			}
			m := timeTag.FindStringSubmatch(rest)
			min_, _ := strconv.Atoi(m[1])
			sec, _ := strconv.Atoi(m[2])
			frac := 0
			if m[3] != "" {
				frac, _ = strconv.Atoi(m[3])
				switch len(m[3]) {
				case 1:
					frac *= 100
				case 2:
					frac *= 10
				}
			}
			stamps = append(stamps, time.Duration(min_)*time.Minute+
				time.Duration(sec)*time.Second+time.Duration(frac)*time.Millisecond)
			rest = rest[loc[1]:]
		}
		if len(stamps) == 0 {
			continue // metadata like [ar:...] or untimed text
		}
		text := strings.TrimSpace(rest)
		for _, at := range stamps {
			at -= offset
			if at < 0 {
				at = 0
			}
			out = append(out, Line{At: at, Text: text})
		}
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].At < out[j].At })
	return out
}

// Index returns the line playing at pos, or -1 before the first line.
func Index(lines []Line, pos time.Duration) int {
	i := sort.Search(len(lines), func(i int) bool { return lines[i].At > pos })
	return i - 1
}
