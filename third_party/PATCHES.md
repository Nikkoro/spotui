# Vendored dependencies

## go-librespot

`go-librespot/` is [devgianlu/go-librespot](https://github.com/devgianlu/go-librespot)
at **v0.10.7** (GPL-3.0), wired in through a `replace` directive in `go.mod`.
Tests, CI files and the `cmd/daemon` binary were left out.

Local changes:

1. `output/output.go`: `ReaderHook` lets the embedding app wrap the PCM
   reader handed to every audio backend. spotui taps it for the real-time
   visualizer. Worth proposing upstream; until then keep this patch minimal
   so updating is a copy plus re-applying these few lines.
2. `mpris/names.go` (new), `server.go`, `interfaces.go`: `BusName` and `Identity`
   variables instead of the hard-coded `go-librespot`, so spotui shows up
   as itself in playerctl / desktop media widgets and does not clash with a
   running go-librespot daemon.
3. `spclient/dj.go`, `daemon/dj.go` (new) and two lines in
   `daemon/player.go`: "Ask DJ" (`ResolveInteractiveContext`) and prompt
   suggestions (`GetSuggestedPrompts`) on Spotify's DJ interaction service,
   exposed as the `dj_ask` / `dj_suggestions` API requests. The protobuf
   messages are encoded by hand with protowire. Modelled on the open
   librespot DJ work (librespot-org/librespot#1797, as used by cloofy).
   DJ narration itself (intro / jump / outro lines between tracks) is
   upstream go-librespot and needs no patch.
4. DJ's answer to a request (`daemon/dj.go`, a few lines in `player.go`,
   `controls.go` and `narration.go`): the `play` command DJ sends back
   (`interactivity_response`) is held until the requested segment
   (`idj_request_id`) is in the list or arrives in an `update_context`;
   playback then jumps to it and the `narration.welcome.*` line is spoken
   before the track's own intro.
5. Starting DJ (`spclient/dj_context.go` (new), a few lines in
   `spclient.go`): DJ's context resolves to an empty shell whose
   `lexicon_context_url` names the session that holds its tracks. The
   session is resolved instead (skipping the playlist service, which answers
   DJ with 404), and its next page is taken as a fresh set, as Spotify's
   clients and librespot#1797 do. Only `hm://lexicon-session-provider/` urls
   are followed.
10. Playlist management (`spclient/playlist_edit_spotui.go`,
   `daemon/playlists_spotui.go` (new), one case in `player.go`): the
   `playlist_create`, `playlist_edit`, `playlist_delete`, `playlist_move`
   and `playlist_insert` API requests, on the same playlist service as
   upstream's add and remove. Create posts a `ListUpdateRequest` to
   `/playlist/v2/playlist` (answered with `CreateListReply`; the service
   ignores the request's attributes, so the name and description follow
   as an `UPDATE_LIST_ATTRIBUTES` change) and adds the new URI to the
   rootlist; edit is an `UPDATE_LIST_ATTRIBUTES` change;
   delete removes the entry from the rootlist (unfollow, which is how the
   apps delete one's own); move is a `MOV` change, whose `to_index` counts
   before the move (as the Web API's `insert_before`); insert is an `ADD`
   at an index (undo of a removal). Each reads the revision first and
   retries once on 409, like upstream's append.
   An empty context (a new playlist) used to fail resolving with "has no
   tracks", which left its listing never ready and re-resolved on every
   poll: `spclient.ErrContextEmpty` now marks it (`context_resolver.go`)
   and the listing caches it as complete and empty
   (`daemon/track_meta_cache.go`).
11. Podcasts (`spclient/resumption_spotui.go`, `daemon/podcasts_spotui.go`
   (new), one case and two calls in `player.go`): `episode_states` asks
   the resumption service for many episodes at once (a CEL filter
   `cs.uri in [...]`, as upstream's single lookup does with `==`) and
   reads each one's latest resume point (position, finished);
   `episode_played` writes `marked_as_finished` / `unmarked_as_finished`.
   `set_liked` / `get_liked` send episodes to the `listenlater` collection
   set (Your Episodes) instead of Liked Songs. Your Episodes'
   context (`spotify:user:<name>:collection:your-episodes`) does not
   resolve, so its listing is read from that set, newest first
   (`daemon/track_meta_cache.go`), and is dropped when `f` changes it.
   Upstream saves an episode's place on pause and on the next track only,
   so quitting mid-episode lost it: the player loop now saves it every
   30 s while an episode plays and once more as it stops (`player.go`).
   A listing entry Spotify answers without metadata (any status but 429
   and 5xx, an empty answer, or left out) is remembered for 30 min and reported with
   `unavailable: true`, counted as cached, so clients stop waiting for it
   (`track_meta_cache.go`, one field in `api_gen.go`).
13. Queue editing (`daemon/queue_spotui.go` (new), one case in
   `player.go`): a `queue_edit` API request takes an entry out of the
   user's queue (by uid, or uri), moves it, puts a track in at an index
   (0: it plays next) or clears the queue. It rebuilds the queue and hands
   it to the track list's `SetQueue`, the path a `set_queue` from the
   Spotify app takes, so Connect shows the change on every device. The
   context's own next tracks are not the queue's and are left alone.
12. Radio (`daemon/radio_spotui.go`, `spclient/radio_spotui.go` (new), one
   case in `player.go`): a `radio` API request finds the radio of a track,
   artist, album or playlist. First the radio playlist the Spotify apps
   play, from `/inspiredby-mix/v2/seed_to_playlist/<uri>` (as librespot's
   `get_radio_for_track`); without one, its own
   `spotify:station:<type>:<id>` when context resolve gives it tracks,
   otherwise the station `/context-resolve/v1/autoplay`
   makes of the seed and a few of its tracks (what autoplay uses at the
   end of a playlist; read from the seed when the client sends none),
   and last the station of one of those songs (some albums have neither).
   A seed with no radio is logged. The client plays the station as a context, so it
   goes on without end. spotify_player asks `hm://autoplay-enabled` and
   `radio-apollo` for a fixed list of tracks instead. A play request's
   `first_uri` puts a song at the front of the context when it is not in
   it (`daemon/api_gen.go`, a few lines in `player.go`), so a song's radio
   starts with the song, as in the Spotify apps.
9. Lyrics from Spotify (`spclient/lyrics.go`, `daemon/lyrics_spotui.go`
   (new), one case in `player.go`): a `lyrics` API request fetches
   `/color-lyrics/v2/track/<id>`, as librespot's `Lyrics::get` (used by
   spotify_player and cloofy), off the player loop. 404 means none; 429 is
   reported with its Retry-After and not retried.
8. Release dates (`daemon/api_server.go`): `releaseDate` returned the
   protobuf's text form (`year:2011 month:10 day:18`) instead of
   `2011-10-18`, so "year" showed after every album name.
7. PulseAudio buffer (`output/driver-pulseaudio.go`): the stream asks for
   a latency (`PULSE_LATENCY_MSEC` if set, as libpulse reads it, else
   200 ms). Without one the server chose its default, seconds long under
   WSLg, so audio was read in huge chunks long before it was heard, which
   made the visualizer lag and stutter.
6. Client identity (`version.go`, one line in `spclient/spclient.go`):
   without a release version stamped in, go-librespot asked for its client
   token as version `0.0.0` and sent spclient requests with Go's default
   User-Agent. Spotify gates features on the client version (DJ's context
   answered 404), so it now presents the desktop version librespot uses and a
   `Spotify/<code> <platform>/0` User-Agent.

The patches have small tests (`spclient/dj_test.go`, `spclient/dj_context_test.go`,
`daemon/dj_spotui_test.go`). Our CI does not run this module's tests; to run
them together with upstream's, copy upstream's `*_test.go` files for the same
tag next to them and run `go test -tags test_unit ./daemon/ ./spclient/`.

To update: copy the new release over this directory (minus `.github`, `cmd`,
`*_test.go`, Docker and vcpkg files), re-apply the patch above, run
`go mod tidy` and update the version here.

## jfreymuth/pulse

`third_party/pulse` is [jfreymuth/pulse](https://github.com/jfreymuth/pulse)
v0.1.3 (MIT), go-librespot's PulseAudio client, with one fix (marked
`spotui patch`):

- `PlaybackStream.Start` waited without limit for the server's `Started`.
  After `Stop` and a flush, which go-librespot does on every skip, WSLg's
  PulseAudio never sends it: the new track played while the player loop hung,
  so the track was never reported (the UI kept the old one) and every later
  seek, volume change or status request timed out. Start now waits at most
  500 ms, the `started` channel is buffered, and the client's dispatcher never
  blocks sending on it.

To update: copy the new release over the directory (minus `.github` and
`demo`) and re-apply the fix.

