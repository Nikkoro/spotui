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

To update: copy the new release over this directory (minus `.github`, `cmd`,
`*_test.go`, Docker and vcpkg files), re-apply the patch above, run
`go mod tidy` and update the version here.
