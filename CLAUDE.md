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
  `clear-cache`, `shader`), session loop that restarts the player.
- `internal/engine`: wraps go-librespot's daemon in-process; every player
  call is a request on a channel.
- `internal/ui`: the interface. `model.go` (state, keys, events), `view.go`,
  `view_pages.go`, `nowplaying.go` (layouts split/panels/compact/classic),
  `lyrics.go`, `fx.go` (beat effects: subtle/party/nightcore), `fun.go`
  (progress bar styles, splash, heart, copy link), `settings.go` (the `,`
  page; each setting writes its own line of config.toml), `images.go`
  (Sixel/kitty covers), `palette.go` (ctrl+k).
- `shaders/spotui.hlsl` with `internal/ui/wtshader.go`: the Windows
  Terminal pixel shader behind the lyrics, embedded by `shaders/` for
  `spotui shader`. spotui paints twelve data cells at the start of the
  top row (layout at the top of both files: beat, bass, accent, style,
  travel, 12 spectrum bands, tempo phase, snare, energy); the shader finds
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
  effects built only on the smooth, "smarter" values felt bland. Give
  every effect a soft ceiling on brightness (warp: fx / (1 + 1.5 fx)). The `debug` style (config only)
  and the "shader frames" debug log line show where judder comes from.
  WT hands the shader a premultiplied texture where the
  background is transparent black, so the effect is added with alpha
  untouched. Signature, cell and band counts and style numbers must match
  in both files (a test checks). A new effect: a function in the HLSL, a
  branch in `main`, a name in `shaderStyles`, the style list in the HLSL
  header.
- `internal/viz`: audio tap, FFT analyzer, renderers, `beat.go` (beat and
  drop detection).
- `internal/termimg`: Sixel/kitty encoding and the writer that places
  images; image cells are skipped with cursor moves so text beside them
  never erases them.
- `internal/lyrics`: Spotify first, LRCLIB behind it, disk cache.
- `internal/webapi`: Spotify Web API with PKCE, cache and 429 back-off.
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
- Keep `gofmt`, `go vet` and `go test -race ./...` clean.

## Tuning the beat effects

`internal/viz/beat.go`: `beatRatio` (how far above the recent bass energy a
beat must be), `beatMinGapMs`, `dropRatio`. `internal/ui/fx.go`: decay
rates in `stepFx` (`pulse`, `zoom`), wave speed and brightness per beat
in `stepFx` (`hop`, `hopTail`, glow in `hopAt`), the zoom threshold in
`nightcoreLine`, sparkle count and life (`sparkLife`, `sparkWidth`), shake
offsets. The current line is drawn by `viewSyncedLyrics` in `lyrics.go`.
