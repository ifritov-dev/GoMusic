![GoMusic logo](assets/GoMusic-bg.png)

# ♪ GoMusic

A comfortable, beautiful, lightweight terminal music player in Go,
built on the [Charm](https://charm.sh) ecosystem (bubbletea, bubbles,
lipgloss). See [RESEARCH.md](RESEARCH.md) for backend decisions.

- **Controls:** play/pause/stop, next/previous, ±5 s seek, shuffle,
  repeat (off/all/one), volume + mute, type-to-filter library, `?` full help
- **Layout:** library on the left, cover art + song card on the right,
  full-width control bar at the bottom (progress + time, state,
  volume, shuffle/repeat, queue position)
- **Now playing card:** embedded cover art (truecolor half-blocks,
  decoded async so the UI never stalls), song title, artist — album
  and a format badge (`MP3 · 44.1 kHz · Stereo`); `▶` marker on the
  playing track in the list
- **Adaptive:** everything scales with the window — fullscreen gets a
  big cover, wide columns and a full-width progress bar; small
  terminals shrink the same arrangement (narrow stacks card under
  the list, tiny hides art and collapses list chrome). The render
  always fits exactly (verified 200×50, 120×32, 90×24, 70×22, 48×14)
- **Settings:** pick the music folder inside the app (`S`)
- **Formats:** mp3, flac, wav, ogg/oga — decoded natively, streamed
  (constant memory, no ffmpeg)
- **Quality:** 48 kHz stereo float32 output, one high-quality resample
  only when the source differs, logarithmic volume (100 % = unity)
- **Platforms:** Linux, Windows, macOS — needs only a C compiler
  (gcc/clang/MinGW) for the bundled miniaudio backend

## Build & run

```sh
go build -o gomusic .
./gomusic
```

On each OS just run the same `go build` natively (cgo requires a C
compiler: `gcc` on Linux, Xcode CLT on macOS, MinGW-w64 on Windows).
No system audio dev packages, no external binaries.

Config lives in the OS config dir (`~/.config/gomusic/config.json`

## Discord Rich Presence (optional)

`S` → Discord section: `d` enables/disables, `tab` edits the
Application ID, `t` toggles playback progress, `b` toggles the
GitHub button, `i` toggles the logo artwork (on by default, needs
the uploaded `logo` asset), and a status line shows whether
Discord is connected. GoMusic finds Discord itself — native,
Flatpak and Snap installs are auto-detected — including the
Flatpak sandbox runtime dir (`$XDG_RUNTIME_DIR/app/<id>/`, any UID,
`ipc-0..9`, plus a `/proc` fallback). No Discord settings needed:
Rich Presence bypasses game detection entirely (remove any manual
game entry for gomusic to avoid a duplicate "Playing" row).
Presence renders as Listening (type 2): main row is the song title,
second row the artist, plus live progress, our logo, and a GitHub
button to this repo (no hover texts: some clients render them as
duplicate rows). The logo asset must be uploaded under the
Application ID or the card degrades; it updates on track change, pause/resume
and seek, and clears on quit. Pushes are generation-gated (no stale
flashes) and Discord-absent rescans back off, so playback never
waits on Discord.

Setup (once): Discord requires every presence to belong to an
Application — create one at
https://discord.com/developers/applications → Rich Presence → Art
Assets → upload `assets/GoMusic-bg.png` as `logo`, then paste the
Application ID into Settings (a future update bakes the official ID
in as the default so this step disappears). Note: Discord only renders images
pre-uploaded under your Application ID, so local song covers can't be
used — the logo is shown and the song rides in the hover text
on Linux): music folder, volume, shuffle, repeat — all persisted.

## Keys

| Key | Action |
|---|---|
| `enter` | play selected |
| `space` | pause / resume |
| `x` | stop |
| `n` / `p` | next / previous (`p` restarts track if >3 s in) |
| `s` | shuffle on/off |
| `r` | repeat off → all → one |
| `+` / `-` (or `↑`/`↓`) | volume |
| `m` | mute |
| `←` / `→` (or `h`/`l`) | seek ∓5 s |
| `/` | filter library |
| `?` | full help on/off |
| `S` | settings (music folder) |
| `esc` | back |
| `q` | quit |

Settings: navigate with `↑/↓` or `j/k`, open with `enter`/`l`,
up with `h`/`backspace`, press `a` to use the current folder.

## Layout

```
main.go                  entry: config → player → bubbletea program
internal/discord/        optional Discord Rich Presence (rich-go IPC)
internal/audio/          miniaudio output backend (pulls beep streamers)
internal/player/         decode → resample → volume → transport controls
internal/library/        recursive scan + tag metadata + cover extraction
internal/config/         JSON config in os.UserConfigDir
internal/ui/             bubbletea model: list, progress, filepicker, help
internal/ui/art.go       cover renderer (stdlib-only half-block truecolor)
internal/ui/ui.go        responsive layout (wide/stacked/tiny) + views
```

Cover art notes: extracted from the playing file's embedded picture
(`dhowden/tag` `Picture()`), decoded with stdlib `image/jpeg`+`image/png`,
rendered to the exact art-box size in a background Cmd and cached.
Files without art get a dim `♪` placeholder. Truecolor half-blocks
(`▀` fg=top pixel, bg=bottom) keep it dependency-free.

## Tests

```sh
go test ./...            # unit + null-audio-backend streaming test
go test -race ./...      # race-checked (audio callback is concurrent)
```
