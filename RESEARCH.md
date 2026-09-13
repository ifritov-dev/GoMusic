# Research: Go TUI music player — audio backend & Charm stack

Date: 2026-09-13. Sources are primary: package docs/source of the
libraries actually vendored in `go.mod`/`go.sum`.

## Decision

- **Decode:** `github.com/gopxl/beep/v2` (`mp3`, `flac`, `vorbis`, `wav`
  subpackages) — pure-Go, streaming (`StreamSeekCloser`), no ffmpeg.
  - mp3 via `github.com/hajimehoshi/go-mp3`
    (`beep/v2@v2.1.1/mp3/decode.go`)
  - flac via `github.com/mewkiz/flac`
    (`beep/v2@v2.1.1/flac/decode.go`)
  - ogg-vorbis via `github.com/jfreymuth/oggvorbis`
    (`beep/v2@v2.1.1/vorbis/decode.go`)
- **Output:** `github.com/gen2brain/malgo@v0.11.26` (bundled miniaudio)
  through a ~100-line backend in `internal/audio` that mirrors the
  `beep/speaker` pull model (`Ctrl` → `effects.Volume` → device).
- **TUI:** `bubbletea@v1.3.10` + `bubbles@v1.0.0`
  (`list`, `progress`, `filepicker`, `help`, `key`) + `lipgloss@v1.1.0`.
- **Tags:** `github.com/dhowden/tag` (best-effort; filename fallback).

## Alternatives rejected

1. **beep `speaker` (oto v3)** — `oto/v3@v3.3.2/driver_unix.go` has
   `#cgo pkg-config: alsa` + `#include <alsa/asoundlib.h`: Linux builds
   need ALSA dev headers installed. Verified: build fails on a stock
   machine without them. miniaudio needs only a C compiler.
2. **ffmpeg/mpv subprocess** — best format coverage (aac/m4a/opus) but
   violates "lightweight, almost no resources" (extra processes,
   megabytes of RAM, external binary to ship).
3. **malgo/miniaudio direct decode** — same output as chosen; beep adds
   tested decoders + quality resampler for free.

## Quality (why "highest possible")

- Fixed **48 kHz / stereo / float32** device format — natively supported
  by WASAPI/CoreAudio/ALSA/Pulse, so the OS never re-converts.
- Sources already at 48 kHz pass through untouched (float conversion
  only). Others go through `beep.ResampleRatio(quality=4, …)` once.
- Volume is `effects.Volume{Base: 2}` with `Volume = log2(pct/100)`:
  perceptual (logarithmic) gain, 100 % = bit-transparent unity gain,
  mute via `Silent` (true zero, no quantization hiss).
- Streaming: one file handle + one decode window in RAM regardless of
  track length (constant memory, ~1–2 % CPU in practice).

## Resources

- Idle TUI redraws at 2 Hz (`tea.Tick(500ms)`); no background polling.
- Library scan is one recursive walk + tag read per file, done once per
  folder change in a bubbletea Cmd (UI never blocks).
- Binary ≈ tens of MB; RSS ≈ 15–30 MB (measured headless during tests).

## Cross-platform (linux / windows / macOS)

- miniaudio backends: ALSA/Pulse/JACK (linux), WASAPI/WinMM/DirectSound
  (windows), CoreAudio (macOS) — selected at runtime, compiled from the
  same bundled C. Only requirement per OS is a C compiler
  (gcc / clang / MinGW) because of cgo.
- No platform-specific Go code: config dir via `os.UserConfigDir`,
  paths via `filepath`. If the device can't open (headless/CI), the
  player degrades to silent mode and the TUI stays fully usable.

## Charm ecosystem mapping (all verified against module source)

| Need | Component |
|---|---|
| App loop | `bubbletea` Model/Update/View + `tea.Tick` |
| Track list + type-to-filter | `bubbles/list` (`FilterValue`, `NewDefaultDelegate`) |
| Progress bar | `bubbles/progress` `ViewAs(pct)` (static, no animation Cmd) |
| Folder picker | `bubbles/filepicker` with `DirAllowed=false, FileAllowed=false` so `enter` only navigates; `a` applies `CurrentDirectory` (avoids the `Open`/`Select`-both-on-`enter` conflict in `filepicker.go`) |
| Footer help | `bubbles/help` + `key.Binding` (`ShortHelp`/`FullHelp`) |
| Styling | `lipgloss` adaptive colors, rounded borders |

## Verification done here

- `go build`, `go vet`, `gofmt` clean.
- `go test -race ./...` — all pass: null-backend streaming, decoder /
  seek / gain, ID3-APIC cover extraction, cover-cell renderer shape,
  library scan, config roundtrip.
- Live tmux E2E on Linux with real tagged MP3s (ffmpeg + mutagen):
  library scan, play, auto-advance, pause, next, shuffle/repeat/mute
  badges, red→blue cover-art switch on track change (truecolor SGR
  verified), settings folder apply, config persistence, quit — at
  200×50, 120×32, 90×24, 70×22 and 48×14, each rendering exactly
  the window height.

## Discord Rich Presence (rich-go IPC)

- `internal/discord.Manager`: snapshot in, goroutine out — the TUI
  never blocks on Discord. `Configure` applies settings (disabling
  logs out = clears presence), `Update` publishes track/pause/seek
  snapshots, `Shutdown` runs on quit (process exit closes the socket
  anyway, which also clears presence).
- First used rich-go, then replaced it with `internal/discord/ipc`:
  rich-go hardcodes the socket (`discord-ipc-0` under UID-1000
  Flatpak/Snap paths), so Flatpak Discord on other UIDs never
  connects. Ours tries `ipc-0..9` across `$XDG_RUNTIME_DIR`, `/tmp`,
  `/run/user/<uid>`, dynamic-UID `.flatpak/*/xdg-run` + snap dirs,
  plus a `/proc` scan for sockets visible only inside a Discord
  process's namespace. Missing at first: Flatpak's host-visible
  sandbox dir `$XDG_RUNTIME_DIR/app/<appid>/` (the actual Flatpak
  Discord socket home) — added via `appSandboxDirs`, which accepts
  any `<appid>` dir containing a `discord-ipc-*` socket.
- `EffectiveClientID`: custom settings ID wins, else the built-in
  official ID, so presence is zero-config.
- Layout fix: activity `type: 2` (Discord shows Listening, not
  Playing); the song line moved to Details (first/prominent row),
  the settings text to State; both truncated to Discord's 128 chars.
  Buttons were always in the payload (verified on the wire) — a
  missing button means Discord never connected or the `logo` asset
  was never uploaded. Covers stay impossible: only pre-uploaded Art
  Assets render, so logo + song hover text is the ceiling.
- Perf: staleness gate moved before dialing (startup herd collapses
  to one scan), failed scans cool off 30s, working socket cached —
  steady-state background cost ≈ zero, playback path never blocks. rich-go also had unsynchronized global socket
  state (found via `-race`) — the new client serializes frames
  internally. Absent or restarted Discord = silent no-op, next
  Update redials; a ~30s tick re-pushes so Discord started after
  GoMusic is picked up automatically.
- Wire-verified with a fake Discord IPC server over a temp
  `XDG_RUNTIME_DIR`: handshake + browsing/play/pause payloads byte-
  exact (state, timestamps, buttons, logo assets).
- Pushes carry a generation counter; a push superseded before hitting
  the wire drops itself. Without this, racing startup pushes arrived
  out of order (a stale `Browsing library` flashed after the current
  track — caught live, then reproduced deterministically with a slow-
  handshake fake: `TestStalePushDropped` fails with the gate disabled,
  passes with it).
- Timestamps: start = now − position, end = start + length (playing
  only; paused drops them so Discord doesn't keep counting).
- Images must be Art Assets of the user's Application ID → `logo`
  for both, song in `LargeText`; setup documented in README.

## UI notes (v2: left/right/bottom layout + proportional scaling)

- Regions, top to bottom: header (1) → middle (library left, song
  card right, stacked when narrow) → full-width controls (progress +
  buttons, 2) → status (0/1) → help (1 or full).
- Art keeps a square aspect (`artW = 2*artH`) and scales with the
  window (e.g. 60×30 cells at 200×50); card is top-aligned with
  the library, song text centered under the art.
- Bottom progress bar is sized from the time string each frame, so
  it always ends exactly at the terminal edge.
- bubbletea names the space key `" "` (`KeySpace.String`, kept
  for backwards compatibility) — binding `"space"` silently never
  matches. Bind `" "` with help text `space`.
- `bubbles/list` View only spans its content, so the middle row
  pads it to the full column (`PlaceHorizontal`) — otherwise the
  song card would float mid-terminal instead of docking right.
- Same exact-fit rules as v1 (list chrome reservation, compact
  collapse below 8 rows, async art loads with stale-drop).

## UI notes (v1: adaptive layout + art)

- One `layout()` function owns all sizing; `Update` (resize) and
  `View` share it, so art reloads and list sizes always agree.
- `bubbles/list` `View()` never clips (`lipgloss.Height` pads only),
  so the layout reserves chrome (title/status/pagination) + one full
  item and collapses the status bar + pagination below 8 list rows.
- Art loads in a `tea.Cmd` (`loadArtCmd`) — decode + render off the
  main path; stale renders (track/box changed) are dropped by key.
