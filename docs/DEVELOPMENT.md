# Developing spotui

## Building

Needs Go 1.25+ and cgo with `libogg`, `libvorbis`, `flac`, `mpg123` (and
`alsa` on Linux):

```sh
sudo apt install libasound2-dev libvorbis-dev libogg-dev libflac-dev libmpg123-dev
go build ./cmd/spotui
```

Windows binaries are cross-compiled in CI with static vcpkg builds of the
decoders; see `.github/workflows/build.yml`. Every push to `main` leaves
binaries as build artifacts.

## Tests and previews

```sh
go test -race ./...
```

Render sample screens without a Spotify connection:

```sh
PREVIEW_DIR=/tmp/frames go test -tags preview -run Preview ./internal/ui
python3 scripts/ans2png.py /tmp/frames/1-now-playing.ans /tmp/np.png 120 36
```

## Vendored dependencies

`third_party/go-librespot` (the embedded Spotify Connect player) and
`third_party/pulse` (its PulseAudio client) carry small patches: the PCM tap
for the visualizer, DJ requests, the client identity Spotify expects, a
PulseAudio start that cannot hang, and more. Each is described in
[`third_party/PATCHES.md`](../third_party/PATCHES.md).

## Releases

Publishing a release on GitHub (which creates its `v*` tag), or pushing a
`v*` tag, runs `.github/workflows/release.yml`: it builds the Linux and
Windows binaries, attaches them with a `SHA256SUMS` file, and `spotui update`
picks them up.

## Being polite to APIs

Every outside API spotui talks to is rate limited on its side, so each
client caches and backs off:

- Spotify Web API: answers cached (search 10 min, recently played 2 min,
  top lists 1 h), one global back-off on 429 honouring `Retry-After`.
- LRCLIB: one request at a time, debounced lookups, `Retry-After` with
  jitter, misses cached on disk.
- GitHub releases: one conditional request (ETag) per `spotui update`, at
  most one a day from the UI, rate-limit resets remembered.

The history of what was built is in [FEATURES.md](FEATURES.md).
