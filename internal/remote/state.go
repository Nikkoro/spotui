// Package remote lets other programs see and steer a running spotui: a
// small state file that `spotui now` reads (status bars poll it every
// second, so it never touches Spotify), a local socket for commands such as
// `spotui next`, and a command run on every track change.
package remote

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"
)

// StateFileName is the state file's name in the cache directory.
const StateFileName = "now.json"

// heartbeat is how often the running spotui rewrites the state file while
// nothing changes, and staleAfter how old a file may get before readers
// take it for one left behind by a spotui that is gone.
const (
	heartbeat  = 10 * time.Second
	staleAfter = 35 * time.Second
)

// State is what the state file holds: the track and how it plays.
type State struct {
	Title      string   `json:"title"`
	Artists    []string `json:"artists"`
	Album      string   `json:"album"`
	URI        string   `json:"uri"`
	URL        string   `json:"url"`
	CoverURL   string   `json:"cover_url,omitempty"`
	DurationMs int64    `json:"duration_ms"`
	// PositionMs is the position at PositionAt; readers move it on while
	// playing, so the file need not change every second.
	PositionMs int64     `json:"position_ms"`
	PositionAt time.Time `json:"position_at"`
	// Status is "playing", "paused" or "stopped".
	Status  string `json:"status"`
	Liked   bool   `json:"liked"`
	Accent  string `json:"accent,omitempty"` // #rrggbb, the cover's colour
	Volume  int    `json:"volume"`           // percent
	Shuffle bool   `json:"shuffle"`
	Repeat  string `json:"repeat"` // "off", "context" or "track"
	Device  string `json:"device,omitempty"`

	// UpdatedAt is when the file was last written, at least every
	// heartbeat while spotui runs.
	UpdatedAt time.Time `json:"updated_at"`
}

// Position is where playback is at now.
func (s State) Position(now time.Time) int64 {
	pos := s.PositionMs
	if s.Status == "playing" && !s.PositionAt.IsZero() {
		pos += now.Sub(s.PositionAt).Milliseconds()
	}
	if s.DurationMs > 0 && pos > s.DurationMs {
		pos = s.DurationMs
	}
	return max(pos, 0)
}

// Stale reports whether the file was left behind by a spotui that no
// longer runs.
func (s State) Stale(now time.Time) bool {
	return now.Sub(s.UpdatedAt) > staleAfter
}

// ErrNoState means no running spotui wrote a state file.
var ErrNoState = errors.New("spotui is not running")

// ReadState returns the freshest of the state files at paths, or ErrNoState.
func ReadState(paths []string, now time.Time) (State, error) {
	var best State
	found := false
	for _, p := range paths {
		if p == "" {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var s State
		if json.Unmarshal(b, &s) != nil || s.Stale(now) {
			continue
		}
		if !found || s.UpdatedAt.After(best.UpdatedAt) {
			best, found = s, true
		}
	}
	if !found {
		return State{}, ErrNoState
	}
	return best, nil
}

// Publisher keeps the state files up to date. Publish is cheap enough to
// call on every frame: it writes only when something changed, from its
// own goroutine, so a slow disk (a Windows path from WSL) never holds up
// the interface.
type Publisher struct {
	paths []string
	logf  func(string, ...any)

	mu   sync.Mutex
	last *State
	next chan State
	stop chan struct{}
	done chan struct{}
}

// NewPublisher writes to every path (empty ones are skipped). logf takes
// write errors.
func NewPublisher(paths []string, logf func(string, ...any)) *Publisher {
	p := &Publisher{
		logf: logf,
		next: make(chan State, 1),
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	for _, path := range paths {
		if path != "" {
			p.paths = append(p.paths, path)
		}
	}
	go p.run()
	return p
}

// Publish hands over the current state; unchanged states are dropped.
func (p *Publisher) Publish(s State) {
	if p == nil {
		return
	}
	p.mu.Lock()
	if p.last != nil && reflect.DeepEqual(*p.last, s) {
		p.mu.Unlock()
		return
	}
	p.last = &s
	p.mu.Unlock()
	// Latest wins: replace a state the writer has not picked up yet.
	select {
	case <-p.next:
	default:
	}
	p.next <- s
}

// Close stops writing and removes the files, so `spotui now` goes quiet.
func (p *Publisher) Close() {
	if p == nil {
		return
	}
	close(p.stop)
	<-p.done
	for _, path := range p.paths {
		_ = os.Remove(path)
	}
}

func (p *Publisher) run() {
	defer close(p.done)
	t := time.NewTicker(heartbeat)
	defer t.Stop()
	var cur *State
	for {
		select {
		case <-p.stop:
			return
		case s := <-p.next:
			cur = &s
		case <-t.C:
			if cur == nil {
				continue
			}
		}
		cur.UpdatedAt = time.Now()
		p.write(*cur)
	}
}

func (p *Publisher) write(s State) {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	for _, path := range p.paths {
		if err := writeAtomic(path, b); err != nil && p.logf != nil {
			p.logf("remote: writing %s: %v", path, err)
		}
	}
}

// writeAtomic replaces path in one step, so a reader never sees half a file.
func writeAtomic(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// DefaultFormat is what `spotui now` prints without --format.
const DefaultFormat = "{icon} {artist} – {title}  {pos}/{dur}"

// Format fills the placeholders of f from s:
// {title} {artist} {artists} {album} {uri} {url} {pos} {dur} {remaining}
// {bar} {percent} {state} {icon} {liked} {volume} {accent}.
func Format(s State, f string, now time.Time) string {
	pos := s.Position(now)
	percent := 0
	if s.DurationMs > 0 {
		percent = int(pos * 100 / s.DurationMs)
	}
	artist := ""
	if len(s.Artists) > 0 {
		artist = s.Artists[0]
	}
	liked := ""
	if s.Liked {
		liked = "♥"
	}
	r := strings.NewReplacer(
		"{title}", s.Title,
		"{artist}", artist,
		"{artists}", strings.Join(s.Artists, ", "),
		"{album}", s.Album,
		"{uri}", s.URI,
		"{url}", s.URL,
		"{pos}", clock(pos),
		"{dur}", clock(s.DurationMs),
		"{remaining}", clock(max(s.DurationMs-pos, 0)),
		"{bar}", Bar(pos, s.DurationMs, 10),
		"{percent}", fmt.Sprint(percent),
		"{state}", s.Status,
		"{icon}", icon(s.Status),
		"{liked}", liked,
		"{volume}", fmt.Sprint(s.Volume),
		"{accent}", s.Accent,
	)
	return r.Replace(f)
}

// Waybar is the JSON waybar's custom modules read (return-type json).
func Waybar(s State, f string, now time.Time) ([]byte, error) {
	tooltip := s.Title
	if len(s.Artists) > 0 {
		tooltip += "\n" + strings.Join(s.Artists, ", ")
	}
	if s.Album != "" {
		tooltip += "\n" + s.Album
	}
	percent := 0
	if s.DurationMs > 0 {
		percent = int(s.Position(now) * 100 / s.DurationMs)
	}
	return json.Marshal(struct {
		Text       string `json:"text"`
		Tooltip    string `json:"tooltip"`
		Class      string `json:"class"`
		Alt        string `json:"alt"`
		Percentage int    `json:"percentage"`
	}{Format(s, f, now), tooltip, s.Status, s.Status, percent})
}

// Bar draws a progress bar width cells wide.
func Bar(pos, dur int64, width int) string {
	filled := 0
	if dur > 0 {
		filled = int(pos * int64(width) / dur)
	}
	filled = min(max(filled, 0), width)
	return strings.Repeat("━", filled) + strings.Repeat("─", width-filled)
}

func clock(ms int64) string {
	s := ms / 1000
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

func icon(status string) string {
	switch status {
	case "playing":
		return "▶"
	case "paused":
		return "⏸"
	}
	return "■"
}
