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
| `?` | help |
| `V` | visualizer style (bars / mirror) |
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
audio_cache = true
audio_cache_size_mb = 1024
notifications = true      # desktop notification on track change
media_controls = true     # MPRIS (Linux) / global media keys (Windows)
theme = "spotify"         # spotify | catppuccin | gruvbox | nord | rose-pine | tokyo-night

[cover]
enabled = true
pixel_size = 32           # 8–64; lower = chunkier pixels
levels = 8                # colour levels per channel; 0 = full colour
dynamic_accent = true     # tint the UI with the cover's colour

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
