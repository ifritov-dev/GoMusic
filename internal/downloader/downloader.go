// Package downloader fetches tracks via yt-dlp straight into the music
// folder. Playlists/albums download whole; single videos as one track.
package downloader

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
)

// allowedHosts are the URL hosts we accept (YouTube + YouTube Music).
var allowedHosts = map[string]bool{
	"youtube.com":              true,
	"www.youtube.com":          true,
	"music.youtube.com":        true,
	"m.youtube.com":            true,
	"youtu.be":                 true,
	"www.youtu.be":             true,
	"youtube-nocookie.com":     true,
	"www.youtube-nocookie.com": true,
}

// InstallHint points at yt-dlp releases when the binary is missing.
const InstallHint = "install yt-dlp: https://github.com/yt-dlp/yt-dlp#installation"

// ValidURL trims raw and returns it if it looks like a YouTube(/Music) link.
func ValidURL(raw string) (string, error) {
	u := strings.TrimSpace(raw)
	if u == "" {
		return "", fmt.Errorf("paste a link first")
	}
	parsed, err := url.Parse(u)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("not a link — paste an https://… URL")
	}
	if !allowedHosts[strings.ToLower(parsed.Hostname())] {
		return "", fmt.Errorf("not a YouTube link (youtube.com, music.youtube.com, youtu.be)")
	}
	return u, nil
}

// Bins locates yt-dlp and ffmpeg (mp3 extraction needs ffmpeg).
func Bins() (yt, ff string, err error) {
	if yt, err = exec.LookPath("yt-dlp"); err != nil {
		return "", "", fmt.Errorf("yt-dlp not found — " + InstallHint)
	}
	if ff, err = exec.LookPath("ffmpeg"); err != nil {
		return "", "", fmt.Errorf("ffmpeg not found — mp3 extraction needs it (e.g. sudo apt install ffmpeg)")
	}
	return yt, ff, nil
}

// Check verifies the link resolves and reports the first title plus the
// entry count (playlists download whole, so count > 1 is normal).
func Check(ctx context.Context, ytBin, link string) (title string, count int, err error) {
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, ytBin,
		"--skip-download", "--flat-playlist", "--no-warnings",
		"--print", "%(title)s", link)
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", 0, fmt.Errorf("link check failed: %s", tailLine(out.String(), err))
	}
	lines := nonEmptyLines(out.String())
	if len(lines) == 0 {
		return "", 0, fmt.Errorf("link check failed: no tracks found")
	}
	return lines[0], len(lines), nil
}

// Download fetches best audio as mp3 into dir and returns final file paths.
func Download(ctx context.Context, ytBin, link, dir string) ([]string, error) {
	var out, errOut bytes.Buffer
	cmd := exec.CommandContext(ctx, ytBin,
		"-x", "--audio-format", "mp3", "--audio-quality", "0",
		"--no-warnings", "--print", "after_move:filepath",
		"-o", dir+"/%(title)s.%(ext)s", link)
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("download failed: %s", tailLine(errOut.String(), err))
	}
	files := nonEmptyLines(out.String())
	if len(files) == 0 {
		return nil, fmt.Errorf("download finished but no files appeared")
	}
	return files, nil
}

func nonEmptyLines(s string) []string {
	var lines []string
	for _, ln := range strings.Split(s, "\n") {
		if ln = strings.TrimSpace(ln); ln != "" {
			lines = append(lines, ln)
		}
	}
	return lines
}

// tailLine prefers yt-dlp's own last error line, else the Go error.
func tailLine(s string, err error) string {
	if lines := nonEmptyLines(s); len(lines) > 0 {
		last := lines[len(lines)-1]
		if len(last) > 200 {
			last = last[:200]
		}
		return last
	}
	return err.Error()
}
