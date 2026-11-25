# Feature log

What has been built, stage by stage. For using spotui, see the
[README](../README.md).

Stage 1:

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
- [x] Oscilloscope visualizer style drawn with braille dots (`V` → wave)
- [x] Now Playing layouts (`L`): split (cover and info beside synced
      lyrics), panels (plus up next, device and DJ), compact (for small or
      zoomed-in terminals, the visualizer shrinks but stays) and the classic
      card; `auto` picks by the window's size
- [x] Library tabs for Recently played (resumes in the playlist or album it
      came from) and your Top tracks / Top artists of the last weeks

Real cover images:

- [x] Sixel (Windows Terminal 1.22+, foot, WezTerm, mlterm, Konsole, …) and
      the kitty graphics protocol (kitty, Ghostty, WezTerm), detected at
      start; pixel blocks everywhere else
- [x] Pixelated on purpose if you like: `[` `]` pick 8…128 pixels per side
      or full detail (default 64, crisp square pixels), `{` `}` colours
- [x] `c` switches between the real image and pixel blocks
- [x] The image is sent once per track: everything that moves sits below
      it, and it is only redrawn when something actually covered it

Later:

- [x] Now Playing sections: `tab` moves to the lyrics and to Up next
- [x] Self-update: `spotui update`, with a pre-release channel
- [x] Settings page (`,`) that writes `config.toml` line by line
- [x] Lyrics from Spotify first, LRCLIB behind it; click a lyric line to
      jump there, click Up next to play; the visualizer's delay matches
      the audio output by itself
- [x] Playlist and album pages show their cover as a real image
- [x] Shimmering progress bar, an animated start-up screen, a heart that
      sparkles when you like a track, `y` copies the track's link
- [x] Linux binaries carry their own FLAC and mpg123, so only ALSA is needed

Lyrics that move to the music:

- [x] Beat effects on the line being sung: subtle (it flashes), party
      (sparkles, a shake on drops), nightcore (a wave runs through it
      letter by letter on each beat, two from the middle on a drop, the
      letters spread a little on the hardest beats, colours flow in tempo)
- [x] Each lyric effect has its own switch (flash, flowing colours, letter
      wave, letter spread, sparkles, shake on drops); the presets stay as
      ready-made sets, and changing a switch moves to custom
- [x] They follow the track's tempo: a tempo tracker (onset envelope,
      autocorrelation, a beat grid locked to the kicks) keeps the beat
      under a steady bass line that a plain bass threshold misses
- [x] The audio tap's playhead moves on smoothly between the output's
      reads (under WSLg they come 70–100 ms at a time), so the visualizer
      and the effects no longer step

Shader background in Windows Terminal:

- [x] The effect blends into the soft edges of letters by their coverage
      (a premultiplied "over"), so bright effects leave no dark rim around
      the text

- [x] A pixel shader (`shaders/spotui.hlsl`) draws behind the lyrics in
      the cover's colours; spotui sends it the music through a row of
      coloured cells in the corner, which it reads and hides
- [x] Effects, switched in Settings with no reload: plasma, tunnel (a
      flight that keeps the tempo, a ring arriving on every beat), warp
      (stars streaking past, hyperspace on drops), aurora (curtains of
      light, a glow sweeping along them each beat), synthwave (a striped sun
      over a grid flying at you in tempo, mountains made of the spectrum),
      chains (raymarched chrome chains in 3D at different depths, links
      sliding along them to the music, a light running over the metal on
      each kick, chains drifting in and out so the picture never repeats),
      rain (a calm one: drops sliding down fogged glass in front of soft
      city lights, sharp through the drops, lights flaring on the kick)
- [x] Easter eggs (`ui.easter_eggs`): a track can bring its own shader
      scene and a word in the status line (which ones is left to find)
- [x] Auto fades between effects as a dissolve, so each effect is compiled
      once: a tab with the shader opens in about a second
- [x] `wt_shader_quality` (low, medium, high): how many chains and how far
      their rays march, so chains runs on a laptop without a GPU card
- [x] auto: moves between them like MilkDrop presets in Winamp, at a drop,
      a louder part, a new track or every 32 bars, fading from one to the
      next; calm parts lean to the aurora and the rain, loud and fast ones
      to the tunnel, the warp, the synthwave and the chains
- [x] Shader motion, MilkDrop-style: bass, mid and treble each against
      their own average; the view zooms with the bass, turns faster with
      the treble and ripples with the vocals
- [x] On the lyrics page and panel whether or not there are lyrics
      (instrumentals too)
- [x] `spotui shader` writes the shader from the binary and prints the
      line for the Windows Terminal profile; spotui keeps it up to date
- [x] For tuning: a `debug` effect, per-second values in the debug log,
      and recordings of a track's levels to replay through the tempo
      tracker
