package termimg

import (
	"bytes"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/muesli/cancelreader"
)

// Protocol is how images reach the terminal.
type Protocol int

const (
	Blocks Protocol = iota // coloured half blocks, any terminal
	Sixel
	Kitty
)

func (p Protocol) String() string {
	switch p {
	case Sixel:
		return "sixel"
	case Kitty:
		return "kitty"
	}
	return "blocks"
}

// Caps is what the terminal can do with images.
type Caps struct {
	Protocol Protocol
	// Cell size in image pixels. Windows Terminal always reports 10×20,
	// the VT340 cell its Sixel support emulates.
	CellW, CellH int
}

var (
	kittyOK  = regexp.MustCompile(`\x1b_Gi=31;OK`)
	cellSize = regexp.MustCompile(`\x1b\[6;(\d+);(\d+)t`)
	da1      = regexp.MustCompile(`\x1b\[\?([0-9;]*)c`)
)

// Detect asks the terminal which image protocol it speaks and how big its
// cells are. mode "blocks", "sixel" or "kitty" forces a protocol; "auto"
// (or "") picks the best the terminal answers for. It must run before the
// interface takes over the terminal, and gives up quickly on terminals
// that do not answer.
func Detect(mode string) Caps {
	caps := Caps{Protocol: Blocks, CellW: 10, CellH: 20}
	if mode == "blocks" {
		return caps
	}
	// tmux keeps few Sixel images and drops kitty ones; stay with blocks
	// unless asked otherwise.
	if os.Getenv("TMUX") != "" && (mode == "" || mode == "auto") {
		return caps
	}
	if !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stdout.Fd()) {
		return caps
	}

	answer := query()
	gotKitty := kittyOK.Match(answer)
	gotSixel := false
	if m := da1.FindSubmatch(answer); m != nil {
		for _, p := range bytes.Split(m[1], []byte(";")) {
			if string(p) == "4" {
				gotSixel = true
			}
		}
	}
	if m := cellSize.FindSubmatch(answer); m != nil {
		h, _ := strconv.Atoi(string(m[1]))
		w, _ := strconv.Atoi(string(m[2]))
		if w > 0 && h > 0 {
			caps.CellW, caps.CellH = w, h
		}
	}

	switch mode {
	case "sixel":
		caps.Protocol = Sixel
	case "kitty":
		caps.Protocol = Kitty
	default:
		switch {
		case gotKitty:
			caps.Protocol = Kitty
		case gotSixel:
			caps.Protocol = Sixel
		}
	}
	return caps
}

// query sends the kitty graphics probe, the cell size request and DA1, and
// collects the answers until DA1's (which every terminal sends) arrives.
func query() []byte {
	state, err := term.MakeRaw(os.Stdin.Fd())
	if err != nil {
		return nil
	}
	defer func() { _ = term.Restore(os.Stdin.Fd(), state) }()

	r, err := cancelreader.NewReader(os.Stdin)
	if err != nil {
		return nil
	}
	defer r.Close()

	const probe = "\x1b_Gi=31,s=1,v=1,a=q,t=d,f=24;AAAA\x1b\\" + "\x1b[16t" + "\x1b[c"
	if _, err := os.Stdout.WriteString(probe); err != nil {
		return nil
	}

	got := make(chan []byte, 16)
	go func() {
		buf := make([]byte, 256)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				got <- append([]byte(nil), buf[:n]...)
			}
			if err != nil {
				close(got)
				return
			}
		}
	}()

	var answer []byte
	timeout := time.After(600 * time.Millisecond)
	for {
		select {
		case b, ok := <-got:
			if !ok {
				return answer
			}
			answer = append(answer, b...)
			if da1.Match(answer) {
				r.Cancel()
				return answer
			}
		case <-timeout:
			r.Cancel()
			return answer
		}
	}
}
