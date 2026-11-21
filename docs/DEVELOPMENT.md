# Developing spotui

## Building

Needs Go 1.25+ and cgo with `libogg`, `libvorbis`, `flac`, `mpg123` (and
`alsa` on Linux):

```sh
sudo apt install build-essential pkg-config libasound2-dev libvorbis-dev libogg-dev libflac-dev libmpg123-dev
go build ./cmd/spotui
```

A local build links the system's FLAC and mpg123. The release binaries for
Linux and Windows carry static vcpkg builds of the decoders instead, so the
Linux one needs only ALSA; see `.github/workflows/build.yml`. Every push to
`main` leaves binaries as build artifacts.

## Tests and previews

```sh
go test -race ./...
```

Render sample screens without a Spotify connection:

```sh
PREVIEW_DIR=/tmp/frames go test -tags preview -run Preview ./internal/ui
python3 scripts/ans2png.py /tmp/frames/1-now-playing.ans /tmp/np.png 120 36
```

## The Windows Terminal shader

`shaders/spotui.hlsl` is compiled by Windows Terminal, not by the build, so
a mistake only shows as WT's error dialog. While working on it, point a WT
profile straight at the repo's file (from WSL,
`\\wsl.localhost\<distro>\...\shaders\spotui.hlsl`, with `"padding": "0"`)
and open a new tab after each edit to reload it. spotui sends the shader the
music through a row of data cells (layout at the top of both
`internal/ui/wtshader.go` and the `.hlsl`); a test checks that the two agree
on the signature, the cell and band counts and the style numbers.

## Tuning the effects to the music

Tune against real tracks, from numbers rather than by eye:

- `spotui --debug` logs a "shader values" line every second (with the
  playback position: energy, bass, the tempo and how sure of it the tracker
  is, the three MilkDrop-style bands) and a "shader frames" line every 5 s
  on how evenly frames go out; `grep` them in `spotui.log`.
- `wt_shader_style = "debug"` draws bars timed by the shader and by spotui
  side by side, with the spectrum and the beat values under them.
- `SPOTUI_TEMPO_DUMP=track.csv spotui` records a track's levels (log
  energies only, no audio), and
  `SPOTUI_TEMPO_RECORDING=track.csv SPOTUI_TEMPO_EXPECT=148 go test -run OnRecording -v ./internal/viz`
  replays them through the tempo tracker as often as needed.

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
- Spotify lyrics (asked first): answers cached on disk, misses for a day;
  a 429 stops asking until its `Retry-After`; a network error falls
  through to LRCLIB without being remembered.
- LRCLIB: one request at a time, debounced lookups, `Retry-After` with
  jitter, misses cached on disk.
- GitHub releases: one conditional request (ETag) per `spotui update`, at
  most one a day from the UI, rate-limit resets remembered.

The history of what was built is in [FEATURES.md](FEATURES.md).
