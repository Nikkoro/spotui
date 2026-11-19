# spotui

A good-looking terminal Spotify client written in Go. The terminal itself is a
Spotify Connect device (powered by an embedded
[go-librespot](https://github.com/devgianlu/go-librespot)), so audio plays
right here and the device shows up in the Connect list on your phone.

Runs on Linux, WSL and Windows. Requires Spotify Premium.

## Status

Stage 1 (current):

- [x] Embedded Spotify Connect device, playback through ALSA / PulseAudio (WSLg) / WASAPI
- [x] Login by approving a code at spotify.com/pair (no browser needed on the machine)
- [x] Now-playing view: progress, volume, shuffle, repeat
- [x] Pixel-art album covers with adjustable resolution and colour depth
- [x] UI accent colour taken from the current cover
- [x] Play any Spotify link or URI

Stage 2:

- [x] Library: playlists (with folders), Liked Songs, saved albums, followed artists
- [x] Playlist / album / artist pages, play from any track
- [x] Search (tracks, albums, artists, playlists) through the Web API
- [x] Queue view, add to queue, like/unlike the current track
- [x] Fuzzy filter on every list

Stage 3:

- [x] Real-time spectrum visualizer from the decoded audio itself (FFT, log bands,
      auto gain, peak caps), tinted with the cover colour; `v` toggles it

Stage 4:

- [x] Live synced lyrics from [LRCLIB](https://lrclib.net) on their own page (`5`),
      current line on Now Playing, sync offset with `<` `>`
- [x] Polite LRCLIB client: identifying User-Agent, one request at a time,
      debounced lookups, Retry-After with jitter, on-disk cache incl. misses

Stage 5:

- [x] MPRIS on Linux: media keys, `playerctl`, desktop media widgets
- [x] Global media keys on Windows (play/pause, next, previous, stop)
- [x] Track-change notifications with the cover: freedesktop on Linux,
      Windows toasts on Windows and from WSL (via `powershell.exe`)

Stage 6:

- [x] Devices overlay (`d`): see every Connect device, move playback
      anywhere, including pulling it to this terminal
- [x] Help overlay (`?`)
- [x] Themes: spotify, catppuccin, gruvbox, nord, rose-pine, tokyo-night
- [x] Mirror visualizer style (`V`)
- [x] Mouse: wheel scrolls lists and lyrics, click the page tabs

Stage 7:

- [x] Spotify DJ: `D` (or "◉ DJ" at the top of Library) starts it, with the
      DJ talking between tracks (go-librespot plays its narration)
- [x] Ask DJ: while it plays, `D` opens a panel to type a request or pick one
      of Spotify's suggested prompts; "Pick something different" starts a new set

Polish 1:

- [x] Covers cached on disk (instant on replays, work offline), next track's
      cover prefetched; library cached per account and shown at once while
      it refreshes
- [x] Remembers page, library tab, cover look, visualizer and search
      history (`↑` `↓` in the search box) between runs
- [x] Add to playlist (`P`), go to album / artist of the playing track
      (`A` / `I`), sleep timer (`z`)
- [x] Rebind any key in `[keys]`; seek and volume steps configurable
- [x] Player restarts on its own after a lost session, backing off 5 s → 1 min

Command palette:

- [x] `ctrl+k` (or `:`) runs any action by name, jumps to any playlist,
      album or artist in your library, switches themes live, or searches
      Spotify for whatever you typed

Mouse & pages:

- [x] Click a row to select it, double-click to open or play; click the
      progress bar to seek; the wheel is the volume knob on Now Playing
- [x] Playlist, album and artist pages show their cover next to the title

Small things:

- [x] Hearts on liked tracks in playlist / album pages; `f` likes the
      selected track in a list (the playing one elsewhere)
- [x] `spotui logout`, `spotui paths`, `spotui clear-cache`; the previous
      run's log is kept as `spotui.log.1`

- [x] Karaoke-style fill of the current lyric line
- [x] Crossfade and autoplay settings
- [x] `[colors]` overrides single colours on top of any theme
- [x] Library tabs for Recently played (resumes in the playlist or album it
      came from) and your Top tracks / Top artists of the last weeks

## Install

Grab `spotui-linux-amd64.tar.gz` (Linux/WSL) or `spotui-windows-amd64.zip`
(Windows) from the latest [release](../../releases). Every push to `main` also
leaves binaries as [build](../../actions/workflows/build.yml) artifacts.

Releases are made by pushing a `v*` tag.

### Linux / WSL runtime libraries

```sh
# Debian/Ubuntu
sudo apt install libasound2 libvorbis0a libogg0 libflac12 libmpg123-0
```

On WSL, audio goes through WSLg's PulseAudio server (Windows 11).

The Windows build is a single static `.exe`.

## First run

1. Start `spotui`.
2. Open the shown link (press `o`) and approve the device. Credentials are
   stored, so this happens once.
3. Pick the device in Spotify Connect on any client, or press `u` and paste a
   Spotify link.

## Keys

| Key | Action |
| --- | --- |
| `1` `2` `3` `4` `5` | Now Playing / Library / Search / Queue / Lyrics |
| `↑` `↓` (`k` `j`), `g` `G`, `ctrl+u` `ctrl+d` | move in lists |
| `enter` | open playlist/album/artist, play track |
| `esc` / `backspace` | back, clear filter |
| `tab` / `shift+tab` | switch section (Library, Search results) |
| `/` | filter the list (on Search: edit the query) |
| `a` | add selected track to the queue |
| `R` | reload the current list |
| `space` | play / pause |
| `n` / `p` | next / previous |
| `←` `→` (`h` `l`) | seek ±5 s |
| `+` / `-` | volume |
| `s` | shuffle |
| `r` | repeat: off → all → track |
| `f` | like / unlike the current track |
| `v` | visualizer on / off |
| `u` | play a Spotify link or URI |
| `[` / `]` | cover pixel resolution |
| `{` / `}` | cover colour depth |
| `<` `>` / `0` | lyrics sync offset ±250 ms / reset (Lyrics page) |
| `D` | DJ: start it; while it plays, ask it for something |
| `d` | devices: move playback |
| `ctrl+k` / `:` | command palette |
| `?` | help |
| `V` | visualizer style (bars / mirror) |
| `P` | add selected / playing track to a playlist |
| `A` / `I` | go to album / artist of the playing track |
| `z` | sleep timer: 15, 30, 45, 60, 90 min, end of track, off |
| `q` | quit |

## Configuration

`config.toml` lives in your config folder (`~/.config/spotui` on Linux,
`%APPDATA%\spotui` on Windows) and is created on first run.

```toml
device_name = "spotui (my-pc)"
client_id = ""            # your Spotify app's client id (for search)
audio_backend = ""        # alsa | pulseaudio | wasapi | pipe; empty = auto
audio_device = "default"
bitrate = 320             # 96 | 160 | 320
initial_volume = 70
normalisation = true
crossfade_ms = 0          # blend tracks, e.g. 6000
autoplay = true           # keep similar music going after a playlist ends
audio_cache = true
audio_cache_size_mb = 1024
notifications = true      # desktop notification on track change
media_controls = true     # MPRIS (Linux) / global media keys (Windows)
seek_step_seconds = 5
volume_step = 5
theme = "spotify"         # spotify | catppuccin | gruvbox | nord | rose-pine | tokyo-night

[colors]                  # optional per-colour overrides on top of the theme
# accent = "#ff79c6"      # also: text, subtle, dim, track, selection, background_text

[keys]                    # extra bindings: action = "key[, key…]"
# next = "ctrl+n"
# play_pause = "k"
# actions: play_pause next previous seek_forward seek_backward volume_up
# volume_down shuffle repeat like add_to_queue add_to_playlist go_to_album
# go_to_artist devices dj sleep_timer visualizer visualizer_style play_link
# filter help quit now_playing library search queue lyrics reload

[cover]
enabled = true
pixel_size = 32           # 8–64; lower = chunkier pixels
levels = 8                # colour levels per channel; 0 = full colour
dynamic_accent = true     # tint the UI with the cover's colour
cache_size_mb = 200       # on-disk cover cache

[visualizer]
enabled = true
delay_ms = 120            # raise if bars run ahead of what you hear
bar_width = 2
style = "bars"            # bars | mirror

[lyrics]
enabled = true
now_playing_line = true   # show the current line on Now Playing
```

### Your own Spotify app (recommended)

Library, playlists and playback go through the Connect protocol and need no
app. Search uses the Web API: without `client_id` it borrows the session
token, which is shared by every librespot client and often rate limited.

1. Create an app at <https://developer.spotify.com/dashboard> (or reuse the
   one you use with spotify_player).
2. Add `http://127.0.0.1:8989/login` as a redirect URI.
3. Put its client id in `client_id`. No secret is needed (PKCE).

The first search opens the browser to approve the app; the token is kept in
`webapi_token.json`.

Logs are written to `spotui.log` in the same folder (`--debug` for more).

## Development

Render sample screens without a Spotify connection:

```sh
PREVIEW_DIR=/tmp/frames go test -tags preview -run Preview ./internal/ui
python3 scripts/ans2png.py /tmp/frames/1-now-playing.ans /tmp/np.png 120 36
```

## Vendored go-librespot

`third_party/go-librespot` is go-librespot with a tiny patch that exposes the
PCM stream for the visualizer; see `third_party/PATCHES.md`.

## Building

Needs Go 1.25+ and cgo with `libogg`, `libvorbis`, `flac`, `mpg123` (and
`alsa` on Linux):

```sh
sudo apt install libasound2-dev libvorbis-dev libogg-dev libflac-dev libmpg123-dev
go build ./cmd/spotui
```

Windows binaries are cross-compiled in CI with static vcpkg builds of the
decoders; see `.github/workflows/build.yml`.

## License

GPL-3.0, as required by go-librespot.
