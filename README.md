# spotui

Spotify in your terminal, and good-looking while at it. spotui is itself a
Spotify Connect device: music plays right in the terminal, and it shows up
next to your speakers in the Spotify app on your phone, so you can control it
from either side.

Runs on Linux, WSL and Windows. Needs Spotify Premium.

- **Pixel-art covers**: real images in terminals that show them (Sixel,
  kitty), pixelated as much as you like, or pixel blocks everywhere else.
- **Synced lyrics** from Spotify (LRCLIB when Spotify has none) that fill in
  as they are sung and move to the beat, on their own page or beside the
  cover; click a line to jump there.
- **Audio visualizer** drawn from the music itself: bars, mirror or wave.
- **Spotify's DJ**, talking between tracks; ask it for something.
- **Your library**: playlists, Liked Songs, albums, artists, recently played,
  top tracks; search; queue; add to playlists.
- **Now Playing layouts** for wide screens and small or zoomed-in terminals.
- **Themes**, a command palette (`ctrl+k`), a settings page (`,`), mouse
  support, media keys, notifications, a sleep timer, self-update.

## Install

Download the latest [release](../../releases): `spotui-linux-amd64.tar.gz`
for Linux and WSL, `spotui-windows-amd64.zip` for Windows.

**Linux / WSL** (Ubuntu 24.04 or newer, Debian 13): unpack it somewhere on
your `PATH`. The audio decoders are built in; only ALSA comes from the
system:

```sh
VERSION=v0.1.6   # the newest one on the releases page
mkdir -p ~/.local/bin
curl -L https://github.com/Nikkoro/spotui/releases/download/$VERSION/spotui-linux-amd64.tar.gz | tar xz -C ~/.local/bin spotui
sudo apt install libasound2t64
```

After this first install, `spotui update` keeps it current.

On WSL, sound goes through WSLg (Windows 11), nothing else to set up.

**Windows**: unzip `spotui.exe` anywhere and run it in Windows Terminal.

**Updating**: `spotui update` installs the newest release over the one you
have. While releases are marked pre-release, use `spotui update --pre` or
set the release channel to pre-release in the settings (`,`). spotui also
checks once a day and shows a new version in the header.

## First run

1. Start `spotui`.
2. Press `o` to open the link it shows, and approve the device. This happens
   only once.
3. Pick the device in Spotify Connect on your phone or computer, or browse
   your library right in spotui (`2`).

### Your own Spotify app (recommended)

Playback and your library work out of the box. Search and the Recently
played and Top lists use Spotify's Web API, which without an app of your own
goes through a shared, often rate-limited access. To use your own:

1. Create an app at <https://developer.spotify.com/dashboard> (or reuse the
   one you made for spotify_player).
2. Add `http://127.0.0.1:8989/login` as a redirect URI.
3. Put its client id in Settings (`,` → Account), or as `client_id` in
   `config.toml`. No secret is needed.

The first search opens your browser once to approve it.

## Keys

`?` shows them all in the app, and `ctrl+k` finds any action by name.

| Key | Action |
| --- | --- |
| `1` … `5` | Now Playing · Library · Search · Queue · Lyrics |
| `space` | play / pause |
| `n` / `p` | next / previous |
| `←` `→` | seek |
| `+` / `-` | volume |
| `s` / `r` | shuffle / repeat (off → all → track) |
| `f` | like / unlike |
| `↑` `↓`, `enter` | move in lists, open or play |
| `/` | filter the list |
| `a` / `P` | add to queue / to a playlist |
| `A` / `I` | go to the album / artist of what is playing |
| `D` | DJ: start it; while it plays, ask it for something |
| `d` | devices: move playback anywhere |
| `u` | play a Spotify link |
| `L` | Now Playing layout: auto · split · panels · compact · classic |
| `tab` | on Now Playing: move to the lyrics or Up next |
| `<` `>` / `0` | lyrics sync earlier / later / reset |
| `v` / `V` | visualizer on / off, style |
| `c`, `[` `]`, `{` `}` | cover: image or blocks, pixels, colours |
| `z` | sleep timer |
| `y` | copy the link to the selected entry or the playing track |
| `,` | settings |
| `ctrl+k` | command palette |
| `q` | quit |

The mouse works too: click to select, double-click to play, click the
progress bar or a lyric line to seek, click an Up next track to play it,
scroll lists and lyrics.

## Settings

Press `,` for the settings page: playback, look, visualizer, lyrics, updates
and account. Changes apply at once where they can and are saved to
`config.toml` straight away; the few marked ↻ need the player restarted,
which `R` does from that page.

`config.toml` lives in `~/.config/spotui` (Linux) or `%APPDATA%\spotui`
(Windows), and `spotui paths` prints where. Everything can also be set
there by hand:

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
# filter help quit now_playing library search queue lyrics reload layout settings
# copy_link

[cover]
enabled = true
mode = "auto"             # auto | sixel | kitty | blocks
image_pixels = 64         # real images: pixels per side, 0 = full detail
size = 0                  # max cover width in columns, 0 = as large as fits
pixel_size = 32           # 8–64; lower = chunkier pixels
levels = 8                # colour levels per channel; 0 = full colour
dynamic_accent = true     # tint the UI with the cover's colour
cache_size_mb = 200       # on-disk cover cache

[visualizer]
enabled = true
delay_ms = -1             # -1: match the audio buffer; or ms, raise if bars run ahead
bar_width = 2
style = "bars"            # bars | mirror | wave

[lyrics]
enabled = true
now_playing_line = true   # show the current line on Now Playing (classic layout)
effects = "subtle"        # off | subtle (flash on the beat) | party (sparkles, shake on drops)
                          # | nightcore (party, plus letter waves and flowing colours)

[now_playing]
layout = "auto"           # auto | split | panels | compact | classic  (L switches)

[ui]
progress_style = "shimmer" # plain | gradient | shimmer
splash = true             # animated start-up screen
like_animation = true     # the heart sparkles when you like a track
copy_as = "url"           # what y copies: url (open.spotify.com) | uri

[update]
check = true              # look for a new release once a day, shown in the header
prerelease = false        # include releases marked pre-release on GitHub
```

## Troubleshooting

- **Logs**: `spotui.log` next to `config.toml`; the previous run is kept as
  `spotui.log.1`. Start with `spotui --debug` for more detail.
- **No sound or crackles on WSL**: restart WSL with `wsl --shutdown` in
  PowerShell. spotui uses a 200 ms audio buffer; set `PULSE_LATENCY_MSEC`
  to change it.
- **Visualizer ahead of the music**: raise its delay in Settings.
- **No real cover images**: they need Sixel (Windows Terminal 1.22+, foot,
  WezTerm, Konsole, …) or the kitty protocol (kitty, Ghostty, WezTerm).
- **Start over**: `spotui logout` forgets the login, `spotui clear-cache`
  drops cached covers, lyrics and audio.

## More

- [Feature log](docs/FEATURES.md)
- [Building and developing](docs/DEVELOPMENT.md)

## License

GPL-3.0, as required by go-librespot.
