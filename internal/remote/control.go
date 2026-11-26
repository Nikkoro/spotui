package remote

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// SocketName is the control socket's name in the config directory. It is
// a Unix socket on every platform: Windows 10 1803 and later have them too.
const SocketName = "spotui.sock"

// maxSocketPath is the longest path a Unix socket takes (sun_path holds
// 108 bytes on Linux, 104 on macOS).
const maxSocketPath = 103

// Request is one command, sent as a line of JSON.
type Request struct {
	Cmd  string   `json:"cmd"`
	Args []string `json:"args,omitempty"`
}

// Response answers a Request.
type Response struct {
	OK    bool   `json:"ok"`
	Text  string `json:"text,omitempty"`
	Error string `json:"error,omitempty"`
}

// Handler carries out a command and says what to print.
type Handler func(ctx context.Context, req Request) (string, error)

// ErrRunning means another spotui already answers on the socket.
var ErrRunning = errors.New("another spotui is already running")

// ErrNotRunning means nothing answers on the socket.
var ErrNotRunning = errors.New("spotui is not running")

// handleTimeout caps how long one command may take.
const handleTimeout = 15 * time.Second

// Listen serves commands on the socket at path until stop is called.
func Listen(path string, h Handler, logf func(string, ...any)) (stop func(), err error) {
	if len(path) > maxSocketPath {
		return nil, fmt.Errorf("socket path %s is too long for a Unix socket", path)
	}
	// A socket left by a spotui that crashed answers nothing: take it over.
	// One that answers belongs to a spotui still running.
	if c, err := net.DialTimeout("unix", path, 500*time.Millisecond); err == nil {
		_ = c.Close()
		return nil, ErrRunning
	}
	_ = os.Remove(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(path, 0o600) // the folder is the user's alone already
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			c, err := ln.Accept()
			if err != nil {
				if !errors.Is(err, net.ErrClosed) && logf != nil {
					logf("remote: accept: %v", err)
				}
				return
			}
			go serve(c, h)
		}
	}()
	return func() {
		_ = ln.Close()
		<-done
		_ = os.Remove(path)
	}, nil
}

func serve(c net.Conn, h Handler) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(handleTimeout + 5*time.Second))
	line, err := bufio.NewReader(io.LimitReader(c, 64<<10)).ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return
	}
	var req Request
	resp := Response{}
	if err := json.Unmarshal(line, &req); err != nil {
		resp.Error = "bad request: " + err.Error()
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), handleTimeout)
		text, err := h(ctx, req)
		cancel()
		if err != nil {
			resp.Error = err.Error()
		} else {
			resp.OK, resp.Text = true, text
		}
	}
	b, _ := json.Marshal(resp)
	_, _ = c.Write(append(b, '\n'))
}

// Send hands req to the spotui listening at path and returns its answer.
func Send(ctx context.Context, path string, req Request) (string, error) {
	var d net.Dialer
	c, err := d.DialContext(ctx, "unix", path)
	if err != nil {
		return "", ErrNotRunning
	}
	defer c.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = c.SetDeadline(dl)
	}
	b, _ := json.Marshal(req)
	if _, err := c.Write(append(b, '\n')); err != nil {
		return "", err
	}
	line, err := bufio.NewReader(c).ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return "", fmt.Errorf("no answer from spotui: %w", err)
	}
	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return "", fmt.Errorf("bad answer from spotui: %w", err)
	}
	if !resp.OK {
		return "", errors.New(resp.Error)
	}
	return resp.Text, nil
}
