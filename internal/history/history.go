// Package history keeps a local record of what was played, for the Stats
// tab. Nothing leaves the machine: one JSON line per play in a file per
// account, next to config.toml.
package history

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Play is one track or episode listened to.
type Play struct {
	At         time.Time `json:"at"` // when it started
	ListenedMs int64     `json:"ms"` // time it actually played, pauses left out
	URI        string    `json:"uri"`
	Title      string    `json:"title"`
	Artists    []string  `json:"artists,omitempty"`
	Album      string    `json:"album,omitempty"`
	DurationMs int64     `json:"dur,omitempty"`
}

// MinListen is how long something must play to count, as Spotify counts
// a stream; shorter tracks count once half of them played.
const MinListen = 30 * time.Second

// Path is the history file of an account in dir.
func Path(dir, username string) string {
	name := "history.jsonl"
	if username != "" {
		name = "history-" + safeName(username) + ".jsonl"
	}
	return filepath.Join(dir, name)
}

func safeName(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r < ' ' {
			return '_'
		}
		return r
	}, s)
}

// Now is what is playing, as the recorder sees it.
type Now struct {
	URI        string
	Title      string
	Artists    []string
	Album      string
	DurationMs int64
	Playing    bool
}

// Recorder turns the playing state, seen after every UI update, into
// plays. Safe for use from several goroutines.
type Recorder struct {
	dir string
	mu  sync.Mutex
	cur *current
	err func(error)
}

type current struct {
	play     Play
	account  string
	playing  bool
	lastSeen time.Time
}

// NewRecorder writes plays to dir; failures go to onErr (may be nil).
func NewRecorder(dir string, onErr func(error)) *Recorder {
	return &Recorder{dir: dir, err: onErr}
}

// Observe takes the state now. A new track ends the one before, which is
// written when it played long enough.
func (r *Recorder) Observe(n Now, account string, now time.Time) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	c := r.cur
	if c != nil && c.playing {
		c.play.ListenedMs += now.Sub(c.lastSeen).Milliseconds()
	}
	if c != nil && (c.play.URI != n.URI || c.account != account) {
		r.finishLocked(c)
		c = nil
		r.cur = nil
	}
	if n.URI == "" {
		return
	}
	if c == nil {
		c = &current{account: account, play: Play{At: now, URI: n.URI, Title: n.Title,
			Artists: n.Artists, Album: n.Album, DurationMs: n.DurationMs}}
		r.cur = c
	}
	c.playing, c.lastSeen = n.Playing, now
}

// Close writes what is playing now, if it played long enough.
func (r *Recorder) Close(now time.Time) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if c := r.cur; c != nil {
		if c.playing {
			c.play.ListenedMs += now.Sub(c.lastSeen).Milliseconds()
		}
		r.finishLocked(c)
		r.cur = nil
	}
}

// counts reports whether a play is long enough to keep.
func counts(p Play) bool {
	need := MinListen.Milliseconds()
	if p.DurationMs > 0 && p.DurationMs/2 < need {
		need = p.DurationMs / 2
	}
	return p.ListenedMs >= need && p.ListenedMs > 0
}

func (r *Recorder) finishLocked(c *current) {
	if !counts(c.play) {
		return
	}
	if err := appendPlay(Path(r.dir, c.account), c.play); err != nil && r.err != nil {
		r.err(err)
	}
}

func appendPlay(path string, p Play) error {
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return errors.Join(err, f.Close())
}

// Load reads an account's plays, oldest first. A missing file is no plays;
// a broken line (a crash mid-write) is skipped.
func Load(path string) ([]Play, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Play
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		var p Play
		if json.Unmarshal(sc.Bytes(), &p) == nil && p.URI != "" {
			out = append(out, p)
		}
	}
	return out, sc.Err()
}
