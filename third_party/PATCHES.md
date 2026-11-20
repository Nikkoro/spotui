# Vendored dependencies

## go-librespot

`go-librespot/` is [devgianlu/go-librespot](https://github.com/devgianlu/go-librespot)
at **v0.10.7** (GPL-3.0), wired in through a `replace` directive in `go.mod`.
Tests, CI files and the `cmd/daemon` binary were left out.

Local changes:

1. `output/output.go`: `ReaderHook` lets the embedding app wrap the PCM
   reader handed to every audio backend. spotui taps it for the real-time
   visualizer. Worth proposing upstream; until then keep this patch minimal
   so updating is a copy plus re-applying these few lines.
2. `mpris/names.go` (new), `server.go`, `interfaces.go`: `BusName` and `Identity`
   variables instead of the hard-coded `go-librespot`, so spotui shows up
   as itself in playerctl / desktop media widgets and does not clash with a
   running go-librespot daemon.
3. `spclient/dj.go`, `daemon/dj.go` (new) and two lines in
   `daemon/player.go`: "Ask DJ" (`ResolveInteractiveContext`) and prompt
   suggestions (`GetSuggestedPrompts`) on Spotify's DJ interaction service,
   exposed as the `dj_ask` / `dj_suggestions` API requests. The protobuf
   messages are encoded by hand with protowire. Modelled on the open
   librespot DJ work (librespot-org/librespot#1797, as used by cloofy).
   DJ narration itself (intro / jump / outro lines between tracks) is
   upstream go-librespot and needs no patch.
4. DJ's answer to a request (`daemon/dj.go`, a few lines in `player.go`,
   `controls.go` and `narration.go`): the `play` command DJ sends back
   (`interactivity_response`) is held until the requested segment
   (`idj_request_id`) is in the list or arrives in an `update_context`;
   playback then jumps to it and the `narration.welcome.*` line is spoken
   before the track's own intro.
5. Starting DJ (`spclient/dj_context.go` (new), a few lines in
   `spclient.go`): DJ's context resolves to an empty shell whose
   `lexicon_context_url` names the session that holds its tracks. The
   session is resolved instead (skipping the playlist service, which answers
   DJ with 404), and its next page is taken as a fresh set, as Spotify's
   clients and librespot#1797 do. Only `hm://lexicon-session-provider/` urls
   are followed.
7. PulseAudio buffer (`output/driver-pulseaudio.go`): the stream asks for
   a latency (`PULSE_LATENCY_MSEC` if set, as libpulse reads it, else
   200 ms). Without one the server chose its default, seconds long under
   WSLg, so audio was read in huge chunks long before it was heard, which
   made the visualizer lag and stutter.
6. Client identity (`version.go`, one line in `spclient/spclient.go`):
   without a release version stamped in, go-librespot asked for its client
   token as version `0.0.0` and sent spclient requests with Go's default
   User-Agent. Spotify gates features on the client version (DJ's context
   answered 404), so it now presents the desktop version librespot uses and a
   `Spotify/<code> <platform>/0` User-Agent.

The patches have small tests (`spclient/dj_test.go`, `spclient/dj_context_test.go`,
`daemon/dj_spotui_test.go`). Our CI does not run this module's tests; to run
them together with upstream's, copy upstream's `*_test.go` files for the same
tag next to them and run `go test -tags test_unit ./daemon/ ./spclient/`.

To update: copy the new release over this directory (minus `.github`, `cmd`,
`*_test.go`, Docker and vcpkg files), re-apply the patch above, run
`go mod tidy` and update the version here.

## jfreymuth/pulse

`third_party/pulse` is [jfreymuth/pulse](https://github.com/jfreymuth/pulse)
v0.1.3 (MIT), go-librespot's PulseAudio client, with one fix (marked
`spotui patch`):

- `PlaybackStream.Start` waited without limit for the server's `Started`.
  After `Stop` and a flush, which go-librespot does on every skip, WSLg's
  PulseAudio never sends it: the new track played while the player loop hung,
  so the track was never reported (the UI kept the old one) and every later
  seek, volume change or status request timed out. Start now waits at most
  500 ms, the `started` channel is buffered, and the client's dispatcher never
  blocks sending on it.

To update: copy the new release over the directory (minus `.github` and
`demo`) and re-apply the fix.

