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
