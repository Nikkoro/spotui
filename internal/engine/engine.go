// Package engine embeds go-librespot so the terminal itself becomes a Spotify
// Connect device, and exposes a small typed API for the UI.
package engine

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"time"

	librespot "github.com/devgianlu/go-librespot"
	"github.com/devgianlu/go-librespot/daemon"
	"github.com/devgianlu/go-librespot/mpris"

	"github.com/Nikkoro/spotui/internal/config"
)

// Event is anything the engine reports to the UI.
type Event interface{ isEvent() }

// AuthEvent reports the device authorization flow. Pending is nil once the
// flow has finished (or was never needed because credentials were stored).
type AuthEvent struct {
	Pending *daemon.ApiDeviceAuth
}

// ReadyEvent is sent once the session is logged in and the player answers.
type ReadyEvent struct {
	Status *daemon.ApiStatus
}

// PlayerEvent is a raw daemon event (playing, paused, metadata, ...).
type PlayerEvent struct {
	Type daemon.ApiEventType
	Data any
}

// FatalEvent reports that the daemon stopped.
type FatalEvent struct {
	Err error
}

func (AuthEvent) isEvent()   {}
func (ReadyEvent) isEvent()  {}
func (PlayerEvent) isEvent() {}
func (FatalEvent) isEvent()  {}

const requestTimeout = 10 * time.Second

// Engine owns the embedded Connect device.
type Engine struct {
	app    *daemon.App
	server *localServer
	log    librespot.Logger
}

// New builds the embedded daemon from the user configuration.
func New(cfg *config.Config, log librespot.Logger) (*Engine, error) {
	server := newLocalServer()

	dc := &daemon.Config{
		DeviceName:       cfg.DeviceName,
		DeviceType:       "computer",
		AudioBackend:     cfg.AudioBackend,
		AudioDevice:      cfg.AudioDevice,
		MixerControlName: "Master",

		Bitrate:               cfg.Bitrate,
		VolumeSteps:           100,
		InitialVolume:         cfg.InitialVolume,
		NormalisationDisabled: !cfg.Normalisation,
		CrossfadeDuration:     max(cfg.CrossfadeMs, 0),
		DisableAutoplay:       !cfg.Autoplay,
		SkipDebounce:          600 * time.Millisecond,

		// Discovery happens through Spotify's servers once logged in, which
		// also works behind WSL2's NAT where mDNS does not.
		ZeroconfEnabled: false,
		ZeroconfBackend: "builtin",

		ImageSize: "large",
	}
	dc.Cache.Enabled = cfg.AudioCache
	dc.Cache.Dir = filepath.Join(cfg.CacheDir, "audio")
	dc.Cache.SizeLimit = cfg.AudioCacheSizeMB * 1024 * 1024
	dc.Metadata.Enabled = true
	dc.Metadata.MaxTracks = 500
	dc.Credentials.Type = "device_auth"

	opts := &daemon.Options{
		Logger:     log,
		Config:     dc,
		StateStore: newFileStateStore(filepath.Join(cfg.ConfigDir, "state.json")),
		APIServer:  server,
	}
	// MPRIS gives media keys, playerctl and desktop media widgets on Linux.
	// Without a session bus (e.g. plain WSL) it waits quietly for one.
	if cfg.MediaControls && runtime.GOOS == "linux" {
		mpris.BusName, mpris.Identity = "spotui", "spotui"
		if mp, err := mpris.NewServer(log); err == nil {
			opts.MediaPlayer = mp
		}
	}

	app, err := daemon.New(opts)
	if err != nil {
		return nil, fmt.Errorf("creating player: %w", err)
	}

	return &Engine{app: app, server: server, log: log}, nil
}

// Events streams everything the UI needs to know about.
func (e *Engine) Events() <-chan Event {
	return e.server.events
}

// Run starts the daemon and blocks until ctx is cancelled or it fails.
func (e *Engine) Run(ctx context.Context) {
	go e.waitReady(ctx)

	err := e.app.Run(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		e.server.push(FatalEvent{Err: err})
	}
}

// Close releases the audio device and network connections.
func (e *Engine) Close() error {
	return e.app.Close()
}

// waitReady polls the player until it answers, which happens only after
// login, then announces the initial status.
func (e *Engine) waitReady(ctx context.Context) {
	for {
		reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		status, err := e.Status(reqCtx)
		cancel()
		if err == nil {
			e.server.push(ReadyEvent{Status: status})
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

func (e *Engine) request(ctx context.Context, typ daemon.ApiRequestType, data any) (any, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, requestTimeout)
		defer cancel()
	}

	req, wait := daemon.NewApiRequest(typ, data)
	select {
	case e.server.requests <- req:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return wait(ctx)
}

// Status returns the current player state.
func (e *Engine) Status(ctx context.Context) (*daemon.ApiStatus, error) {
	resp, err := e.request(ctx, daemon.ApiRequestTypeStatus, nil)
	if err != nil {
		return nil, err
	}
	switch s := resp.(type) {
	case *daemon.ApiStatus:
		return s, nil
	case daemon.ApiStatus:
		return &s, nil
	default:
		return nil, fmt.Errorf("unexpected status response %T", resp)
	}
}

// Token returns a Web API access token for the logged-in account.
func (e *Engine) Token(ctx context.Context) (string, error) {
	resp, err := e.request(ctx, daemon.ApiRequestTypeToken, nil)
	if err != nil {
		return "", err
	}
	switch t := resp.(type) {
	case *daemon.ApiToken:
		return t.Token, nil
	case daemon.ApiToken:
		return t.Token, nil
	default:
		return "", fmt.Errorf("unexpected token response %T", resp)
	}
}

// PlayPause toggles playback.
func (e *Engine) PlayPause(ctx context.Context) error {
	_, err := e.request(ctx, daemon.ApiRequestTypePlayPause, nil)
	return err
}

// Next skips to the next track.
func (e *Engine) Next(ctx context.Context) error {
	_, err := e.request(ctx, daemon.ApiRequestTypeNext, daemon.ApiNext{})
	return err
}

// Prev goes back, restarting the current track first if it played a while.
func (e *Engine) Prev(ctx context.Context) error {
	_, err := e.request(ctx, daemon.ApiRequestTypePrev, daemon.ApiPrev{})
	return err
}

// Seek moves the playback position, relative to the current one if relative.
func (e *Engine) Seek(ctx context.Context, positionMs int64, relative bool) error {
	_, err := e.request(ctx, daemon.ApiRequestTypeSeek, daemon.ApiSeek{Position: positionMs, Relative: relative})
	return err
}

// SetVolume sets the volume in steps (0-100), relative to the current one if relative.
func (e *Engine) SetVolume(ctx context.Context, volume int32, relative bool) error {
	_, err := e.request(ctx, daemon.ApiRequestTypeSetVolume, daemon.ApiSetVolume{Volume: volume, Relative: relative})
	return err
}

// SetShuffle turns context shuffling on or off.
func (e *Engine) SetShuffle(ctx context.Context, on bool) error {
	_, err := e.request(ctx, daemon.ApiRequestTypeSetShufflingContext, on)
	return err
}

// SetRepeat sets the repeat mode.
func (e *Engine) SetRepeat(ctx context.Context, context_, track bool) error {
	if _, err := e.request(ctx, daemon.ApiRequestTypeSetRepeatingContext, context_); err != nil {
		return err
	}
	_, err := e.request(ctx, daemon.ApiRequestTypeSetRepeatingTrack, track)
	return err
}

// Play starts playing a Spotify URI (track, album, playlist, artist, ...).
func (e *Engine) Play(ctx context.Context, uri string) error {
	_, err := e.request(ctx, daemon.ApiRequestTypePlay, daemon.ApiPlay{Uri: uri})
	return err
}

// Pause pauses playback.
func (e *Engine) Pause(ctx context.Context) error {
	_, err := e.request(ctx, daemon.ApiRequestTypePause, nil)
	return err
}

// Debugf writes a debug line to the session log.
func (e *Engine) Debugf(format string, args ...any) {
	if e != nil && e.log != nil {
		e.log.Debugf(format, args...)
	}
}
