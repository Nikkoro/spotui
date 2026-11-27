# spotui: notes for Claude Code

A Spotify client for the terminal in Go (Bubble Tea, Lip Gloss) that is
itself a Spotify Connect device through an embedded, patched go-librespot.

## Build and run

```sh
sudo apt install build-essential pkg-config libasound2-dev libflac-dev libmpg123-dev libogg-dev libvorbis-dev
go run ./cmd/spotui            # or: go build -o ~/.local/bin/spotui ./cmd/spotui
go run ./cmd/spotui --debug    # logs to spotui.log next to config.toml (spotui paths)
go test -race ./...
gofmt -l . && go vet ./...
```

Local builds link the system FLAC and mpg123; release builds carry them
statically (vcpkg in CI). Go 1.25+.

Render screens without Spotify (sample track, invented lyrics):

```sh
PREVIEW_DIR=/tmp/frames go test -tags preview -run Preview ./internal/ui
```

## Where things are

- `cmd/spotui`: flags, subcommands (`update`, `logout`, `paths`,
  `clear-cache`, `shader`, `now` and the remote commands), session loop
  that restarts the player.
- `internal/engine`: wraps go-librespot's daemon in-process; every player
  call is a request on a channel.
- `internal/ui`: the interface. `model.go` (state, keys, events), `view.go`,
  `view_pages.go`, `nowplaying.go` (layouts split/panels/compact/classic),
  `lyrics.go`, `fx.go` (beat effects, each its own switch in `fxSet`:
  flash, colors, wave, spread, sparkles, shake; presets
  subtle/party/nightcore are ready-made sets, `custom` reads
  `[lyrics.fx]`), `beats.go`
  (the beats every effect follows: tempo grid, plain detector before it),
  `fun.go`
  (progress bar styles, splash, heart, copy link), `settings.go` (the `,`
  page; each setting writes its own line of config.toml), `images.go`
  (Sixel/kitty covers), `palette.go` (ctrl+k), `podcasts.go` (Podcasts tab,
  a show's page with episode progress, episode notes on the lyrics page,
  `m` mark played), `queue.go` (x/U/J/K/X on
  the queue page, Q play next; the player edits its own queue),
  `radio.go` (S: a seed's
  radio playlist or station, cached an hour, played as a context), `playlists.go` (create,
  rename, delete playlists, remove and move tracks with undo; the footer
  question `ask` / `confirm` lives there too).
- `shaders/spotui.hlsl` with `internal/ui/wtshader.go`: the Windows
  Terminal pixel shader behind the lyrics, embedded by `shaders/` for
  `spotui shader`. spotui paints sixteen data cells at the start of the
  top row (layout at the top of both files: beat, bass, accent, style,
  travel, 12 spectrum bands, tempo phase, snare, energy, MilkDrop bands,
  camera, auto's fade, `wt_shader_quality`; 128 added to the style draws
  it in one colour, for easter eggs); the shader finds
  and hides them. They are sent 60 times a second; the analysis behind
  them runs at 30 (the beat detector is tuned for it). One file holds every
  effect, picked by the style cell, so switching needs no reload. The
  shader keeps no state between frames: anything that moves at a speed
  the music sets is integrated in Go (`shaderFx.step`) and sent as a
  position. Values reach WT a little unevenly, which shows on slow steady
  motion, so the steady flow is timed by the shader's `Time` and the
  music drives brightness and fast beat moves. The plasma reads its own
  cell 10, the jumpy values it was first tuned with: the user preferred
  how that followed the music, so keep it. New effects follow the same
  rule: their core reacts at once to those jumpy values (ck, cb, cl), and
  tempo, the first of four beats, snares and energy are extras on top;
  effects built only on the smooth, "smarter" values felt bland. Slow
  effects (aurora) are the exception for shape: there the jumpy values
  only set brightness and the shape follows smooth ones (bass, level,
  bands), or it looks nervous. aurora and rain are the calm ones.
  Easter eggs' scenes (`eastereggs.go`: a track's name picks one whatever
  the effect) are hidden from Settings and auto; never describe them in
  commits, PRs or release notes. WT
  compiles the shader when a tab opens, and the compiler pastes every
  function in where it is called: keep `effect()` called once (auto's
  fade is a per-pixel dissolve for that) and never inside a loop (5x
  slower to compile), take no screen derivatives (fwidth, ddx; work them
  out, as synthwave does) so effects stay real branches, and prefer
  `[loop]` to `[unroll]` in helpers called from many places (`fbm`).
  Check with `tools/shadertime` (Windows' compiler, as WT uses; about 1 s
  is the budget, `-each` shows each effect's share).
  chains is the heavy one (raymarched 3D): a ray marches only near the
  chains it passes (`nearAxis`), and
  `wt_shader_quality` (cell 14 B) sets how many chains and steps; keep it
  running on an iGPU. Give
  every effect a soft ceiling on brightness (warp: fx / (1 + 1.5 fx)). The `debug` style (config only)
  and the "shader frames" debug log line show where judder comes from.
  WT hands the shader a premultiplied texture where the
  background is transparent black, so the effect is added with alpha
  untouched; text is laid over it by its coverage (`1 - alpha`), which
  any faster cut-off turns into a dark rim on bright effects. Signature, cell and band counts and style numbers must match
  in both files (a test checks). A new effect: a function in the HLSL, a
  branch in `main`, a name in `shaderStyles`, the style list in the HLSL
  header.
- `internal/viz`: audio tap, FFT analyzer, renderers, `beat.go` (beat and
  drop detection: a bass threshold, which misses kicks under a steady bass
  line), `tempo.go` (tempo tracking: onset envelope, autocorrelation, a
  beat grid locked to it, snares from the highs; the shader uses it).
  Tune against a real track: `spotui --debug`, then
  `grep "shader values" spotui.log` (one line a second with the position).
  To tune the tracker offline, record a track's frames with
  `SPOTUI_TEMPO_DUMP=track.csv spotui` (log energies only, no audio) and
  replay them: `SPOTUI_TEMPO_RECORDING=track.csv SPOTUI_TEMPO_EXPECT=148
  go test -run OnRecording -v ./internal/viz`.
  The audio tap must hand out a playhead that moves on smoothly: under
  WSLg the output reads 70-100 ms at a time, and a playhead that jumped by
  reads put every kick on a read's edge (see `playheadLocked`).
- `tools/shadertime`: times the WT shader's compile with Windows'
  `d3dcompiler_47.dll` (Windows only; build it with GOOS=windows from WSL).
- `internal/remote`: control from outside. The control socket
  (`spotui next`, `play`, `volume`, ...; commands reach the UI as
  `ui.RemoteMsg`, handled in `internal/ui/remote.go`), the state file
  `spotui now` reads (written by a `Publisher` after each `Update`, only
  when the state changed, plus a heartbeat; a file older than 35 s counts
  as stale) and the `[hooks]` runner. `cmd/spotui/remote.go` holds the
  subcommands.
- `internal/termimg`: Sixel/kitty encoding and the writer that places
  images; image cells are skipped with cursor moves so text beside them
  never erases them.
- `internal/lyrics`: Spotify first, LRCLIB behind it, disk cache. The
  Spotify side is wired in `internal/ui/lyrics_source.go`, with a probe
  (`SPOTUI_LYRICS_*`, see docs/DEVELOPMENT.md) for the shape of
  `color-lyrics` responses, translations included.
- `internal/webapi`: Spotify Web API with PKCE, cache and 429 back-off;
  `podcasts.go` reads saved shows, a show's episodes and an episode.
- `internal/update`: GitHub releases, self-update.
- `internal/config`: config.toml, defaults, line-preserving edits.
- `third_party/go-librespot`, `third_party/pulse`: vendored with patches,
  each listed in `third_party/PATCHES.md`; add an entry for any new one.

## Rules

- Any call to an outside API caches its answers and honours rate limits
  (Retry-After, back-off) from the start. Never hammer Spotify, LRCLIB or
  GitHub.
- Code identifiers, keys and file names in English.
- New behaviour gets a setting where it makes sense: a field in
  `internal/config`, a default in `Default()`, an entry in `settings.go`,
  a line in the README's config block.
- Tests and previews use invented lyrics only, never real song text.
- Every change that adds or alters a feature brings the docs along in the
  same PR: `README.md` (use, settings, the config block, troubleshooting),
  `docs/FEATURES.md` (what was built), `docs/DEVELOPMENT.md` (how to build,
  test, tune, release), `third_party/PATCHES.md` (any vendored change) and
  this file (where things are). Check all five before opening a PR.
- Keep `gofmt`, `go vet` and `go test -race ./...` clean.

## Tuning the beat effects

`internal/viz/beat.go`: `beatRatio` (how far above the recent bass energy a
beat must be), `beatMinGapMs`, `dropRatio`. `internal/ui/fx.go`: decay
rates in `stepFx` (`pulse`, `zoom`), wave speed and brightness per beat
in `stepFx` (`hop`, `hopTail`, glow in `hopAt`), the zoom threshold in
`nightcoreLine`, sparkle count and life (`sparkLife`, `sparkWidth`), shake
offsets. The current line is drawn by `viewSyncedLyrics` in `lyrics.go`.
