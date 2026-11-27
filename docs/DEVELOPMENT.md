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

## Remote control and `spotui now`

`internal/remote` holds the three pieces; `cmd/spotui/remote.go` wires
them up. The socket (`spotui.sock` next to `config.toml`) takes one line
of JSON per connection (`{"cmd":"next","args":[]}`) and answers with one
(`{"ok":true}`); the handler passes it to the interface as a
`ui.RemoteMsg`, so commands act like keys (flashes, the heart) and the
answer waits for the player's result. The state file (`now.json` in the
cache folder) is built by `Model.remoteState` after every `Update`; the
publisher drops unchanged states, so it only writes on events. The
position is stored with its timestamp and readers move it on, which keeps
the file from changing every frame.

Try it against a running spotui:

```sh
go run ./cmd/spotui next
go run ./cmd/spotui now --json
watch -n1 go run ./cmd/spotui now --format "'{icon} {title} {bar} {pos}'"
```

## Playlist edits

Creating, renaming, deleting and moving go through the embedded player to
Spotify's playlist service (`third_party/go-librespot`, patch 10 in
`PATCHES.md`), not the Web API. The UI changes the list at once and sends
the change in the background; a failure reads the playlist again. To try
them, make a throwaway playlist with `N` and work on that: `x`/`U`, `J`/`K`,
`e`, `E`, and `X` at the end. `spotui --debug` logs each request's error.

## The Windows Terminal shader

`shaders/spotui.hlsl` is compiled by Windows Terminal, not by the build, so
a mistake only shows as WT's error dialog. While working on it, point a WT
profile straight at the repo's file (from WSL,
`\\wsl.localhost\<distro>\...\shaders\spotui.hlsl`, with `"padding": "0"`)
and open a new tab after each edit to reload it. WT compiles it when a tab
opens, so a heavy shader shows as a slow first start: keep `effect()`
called once and outside any loop, use no screen derivatives, and prefer
`[loop]` to `[unroll]` (see CLAUDE.md). `tools/shadertime` compiles it
with Windows' own `d3dcompiler_47.dll`, as WT does, and says whether it
compiles, how long it took and what the compiler warns about; `-each`
adds what each effect costs. About 1 s is where it should stay. On
Windows, `go run ./tools/shadertime`; from WSL:

```sh
GOOS=windows go build -o /tmp/shadertime.exe ./tools/shadertime
/tmp/shadertime.exe -each shaders/spotui.hlsl
```

(run it without a pipe: piped through WSL, a long run's output can get
lost). spotui sends the shader the
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
- chains (style 5) is the one heavy effect: each pixel marches only
  against the chains its ray passes near (`nearAxis`, a cylinder around
  each chain's axis), from where it comes within reach. The knobs are at
  the top of `chainAt` (life 20…40 s, depth 2.5…10, size, tilt, slide
  speed), the studio lights in `chainShade`, the dimming behind the lyrics
  at the top of `chains`, and slots and steps per `wt_shader_quality`
  (5/24, 7/32, 9/48: chains, steps per chain). Check a change on the
  slowest machine with `wt_shader_quality = "low"` and the "shader frames"
  log line.
- rain (style 6) is built from cells, each holding at most one light or
  drop that stays inside it, so no pixel looks at its neighbours: lights
  in `rainBokeh` (three layers, how many lit, sizes), drops in `rainDrops`
  (cell size, how fast a column slides, the stop-and-go), condensation in
  `rainDew`. Keep a drop's movement inside its cell, or it is cut off at
  the cell's edge.
- Easter eggs live in `internal/ui/eastereggs.go`: a part of a track's
  name, the style it shows, optionally a colour of its own (and `mono`,
  the whole effect in that colour: 128 added to the style in cell 4), and
  a greeting. A colour costs nothing to compile; a new scene does, so
  check it with `tools/shadertime`. Their styles are hidden from
  Settings and auto (`shaderHidden`); to see one without its track, set
  `wt_shader_style` to its style name in config.toml. Keep what they are
  out of commit messages, PRs and release notes: they are meant to be
  found.
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

## Packages

After each release the `packages` job of `.github/workflows/release.yml`
fills the templates in `packaging/` with the version and the hashes from
`SHA256SUMS` (`packaging/render.sh <version> SHA256SUMS <out>`) and
publishes them:

- **Scoop**: `bucket/spotui.json` in this repo (`scoop bucket add spotui
  https://github.com/Nikkoro/spotui`), committed to main by the job. Its
  `checkver` reads the newest release, pre-releases included.
- **Homebrew** (Linux only until there's a macOS build): the formula goes
  to `Nikkoro/homebrew-spotui` (`Formula/spotui.rb`) when the secret
  `HOMEBREW_TAP_TOKEN` is set: a fine-grained token with Contents
  read/write on that repo. Users: `brew install Nikkoro/spotui/spotui`.
- **AUR** (`spotui-bin`): pushed when the secret `AUR_SSH_KEY` holds the
  private half of an SSH key added to an AUR account that owns the package.
  The first upload is by hand: `git clone
  ssh://aur@aur.archlinux.org/spotui-bin.git`, render the files, add
  `PKGBUILD` and `.SRCINFO`, push.
- **winget**: not automated yet; the first submission goes through
  `wingetcreate new` with the release's zip (a portable `spotui.exe`
  inside).

Without a secret its step says "skipped" and the release still succeeds.
`spotui update` notices a package-managed install (`update.Packaged`, by
the binary's path) and points to the package manager instead of
replacing the binary.

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
