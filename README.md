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

Next:

- [ ] Library, playlists, search, queue (Web API with your own client id)
- [ ] Real-time audio visualizer (FFT of the decoded audio)
- [ ] Live synced lyrics
- [ ] Media keys and notifications (MPRIS on Linux, SMTC on Windows)
- [ ] Themes, mouse support

## Install

Grab `spotui` (Linux/WSL) or `spotui.exe` (Windows) from the latest
[build](../../actions/workflows/build.yml) artifacts.

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
| `space` | play / pause |
| `n` / `p` | next / previous |
| `←` `→` (`h` `l`) | seek ±5 s |
| `+` / `-` | volume |
| `s` | shuffle |
| `r` | repeat: off → all → track |
| `u` or `/` | play a Spotify link or URI |
| `[` / `]` | cover pixel resolution |
| `{` / `}` | cover colour depth |
| `q` | quit |

## Configuration

`config.toml` lives in your config folder (`~/.config/spotui` on Linux,
`%APPDATA%\spotui` on Windows) and is created on first run.

```toml
device_name = "spotui (my-pc)"
audio_backend = ""        # alsa | pulseaudio | wasapi | pipe; empty = auto
audio_device = "default"
bitrate = 320             # 96 | 160 | 320
initial_volume = 70
normalisation = true
audio_cache = true
audio_cache_size_mb = 1024

[cover]
enabled = true
pixel_size = 32           # 8–64; lower = chunkier pixels
levels = 8                # colour levels per channel; 0 = full colour
dynamic_accent = true     # tint the UI with the cover's colour
```

Logs are written to `spotui.log` in the same folder (`--debug` for more).

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
