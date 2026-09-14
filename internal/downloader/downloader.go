// Package downloader fetches tracks via yt-dlp straight into the music
// folder. Playlists/albums download whole; single videos as one track.
package downloader

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"sync"
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

// Progress is a mutex-guarded snapshot of a running download. The
// downloader goroutine feeds it via observe while the TUI polls
// Snapshot on its tick loop — no channels, no tea.Program needed.
type Progress struct {
	mu      sync.Mutex
	Total   int     // playlist size (1 for a single video)
	Done    int     // fully finished items
	Current string  // title being fetched/converted right now
	Percent float64 // 0-100 within the current item
}

// NewProgress seeds a snapshot (Check's count, so the bar has a total
// before yt-dlp prints its first line).
func NewProgress(total int) *Progress {
	if total < 1 {
		total = 1
	}
	return &Progress{Total: total}
}

// Snapshot returns a consistent copy for rendering.
func (p *Progress) Snapshot() (total, done int, cur string, pct float64) {
	if p == nil {
		return 1, 0, "", 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.Total, p.Done, p.Current, p.Percent
}

// Overall is the whole-job fraction 0..1 (finished items + the
// in-flight one), for a single bar across a playlist.
func (p *Progress) Overall() float64 {
	total, done, _, pct := p.Snapshot()
	if total <= 1 {
		return clamp01(pct / 100)
	}
	return clamp01((float64(done) + clamp01(pct/100)) / float64(total))
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// itemPrefix is our --print marker, emitted before each item downloads:
// @@ITEM@@:<playlist_index>:<playlist_count>:<title> (index/count are
// NA for single videos).
const itemPrefix = "@@ITEM@@:"

// observe folds one yt-dlp output line (stdout or stderr) into the
// snapshot. Unknown lines are ignored so format drift never breaks
// the download itself.
func (p *Progress) observe(line string) {
	if p == nil {
		return
	}
	t := strings.TrimSpace(line)
	if rest, ok := strings.CutPrefix(t, itemPrefix); ok {
		parts := strings.SplitN(rest, ":", 3)
		if len(parts) != 3 {
			return
		}
		n, nErr := strconv.Atoi(parts[0])
		m, mErr := strconv.Atoi(parts[1])
		p.mu.Lock()
		if mErr == nil && m > 0 {
			p.Total = m
			if nErr == nil && n > 0 {
				p.Done = min(n-1, m)
			}
		}
		p.Current = parts[2]
		p.Percent = 0
		p.mu.Unlock()
		return
	}
	if !strings.HasPrefix(t, "[") {
		return
	}
	stage := t
	body := t
	if i := strings.Index(t, "]"); i >= 0 {
		stage = t[:i+1]
		body = strings.TrimSpace(t[i+1:])
	}
	switch stage {
	case "[download]":
		// Playlist position fallback (in case --print before_dl
		// is ever dropped): "Downloading item 3 of 12".
		var n, m int
		if _, err := fmt.Sscanf(body, "Downloading item %d of %d", &n, &m); err == nil && m > 0 {
			p.mu.Lock()
			p.Total = m
			if n > 0 {
				p.Done = min(n-1, m)
			}
			p.Percent = 0
			p.mu.Unlock()
			return
		}
		if strings.Contains(body, "has already been downloaded") {
			p.mu.Lock()
			p.Percent = 100
			p.mu.Unlock()
			return
		}
		if body == "Destination:" || strings.HasPrefix(body, "Destination:") {
			return // fetch starting; progress lines follow
		}
		// " 45.2% of ~4.0MiB at ..." — number right before %.
		if i := strings.Index(body, "%"); i >= 0 {
			j := i - 1
			for j >= 0 && (body[j] == '.' || (body[j] >= '0' && body[j] <= '9')) {
				j--
			}
			if v, err := strconv.ParseFloat(body[j+1:i], 64); err == nil {
				p.mu.Lock()
				p.Percent = clamp01(v / 100) * 100
				p.mu.Unlock()
			}
			return
		}
	case "[ExtractAudio]", "[Merger]", "[Metadata]", "[EmbedThumbnail]":
		// Post-process stages: the audio is fetched, yt-dlp is
		// converting/tagging — read as done for this item.
		p.mu.Lock()
		p.Percent = 100
		p.mu.Unlock()
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func scanLines(r io.Reader, fn func(string)) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		fn(sc.Text())
	}
}

// Download fetches best audio as mp3 into dir and returns final file paths.
// --add-metadata writes title/artist tags (uploader as artist fallback)
// and --embed-thumbnail stores the cover art, so library, player card
// and Discord presence pick them up with no extra work.
func Download(ctx context.Context, ytBin, link, dir string) ([]string, error) {
	return DownloadWithProgress(ctx, ytBin, link, dir, nil)
}

// DownloadWithProgress is Download plus a live snapshot: every yt-dlp
// output line is folded into prog (may be nil) while the transfer runs.
//
// Playlists/albums download whole (--yes-playlist makes that explicit
// even for watch-URLs carrying a list param); the -o template carries
// the video id so two playlist entries with the same title can never
// overwrite each other.
func DownloadWithProgress(ctx context.Context, ytBin, link, dir string, prog *Progress) ([]string, error) {
	cmd := exec.CommandContext(ctx, ytBin,
		"-x", "--audio-format", "mp3", "--audio-quality", "0",
		"--add-metadata", "--embed-thumbnail",
		"--no-warnings", "--newline", "--progress", "--yes-playlist",
		"--print", "after_move:filepath",
		"--print", "before_dl:"+itemPrefix+"%(playlist_index)s:%(playlist_count)s:%(title)s",
		"-o", dir+"/%(title)s [%(id)s].%(ext)s", link)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("download failed: %s", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("download failed: %s", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("download failed: %s", err)
	}
	var mu sync.Mutex
	var files []string
	var errBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		scanLines(stdout, func(ln string) {
			prog.observe(ln)
			if s := strings.TrimSpace(ln); s != "" && !strings.HasPrefix(s, "[") && !strings.HasPrefix(s, itemPrefix) {
				mu.Lock()
				files = append(files, s)
				mu.Unlock()
			}
		})
	}()
	go func() {
		defer wg.Done()
		scanLines(stderr, func(ln string) {
			prog.observe(ln)
			errBuf.WriteString(ln + "\n")
		})
	}()
	wg.Wait()
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("download failed: %s", tailLine(errBuf.String(), err))
	}
	mu.Lock()
	defer mu.Unlock()
	if len(files) == 0 {
		return nil, fmt.Errorf("download finished but no files appeared")
	}
	if prog != nil {
		// Markers ride on stdout, percentages on stderr: their
		// relative order is not guaranteed, so a "skipped" line
		// may land before its item marker. On success the file
		// list is ground truth — settle the snapshot on it.
		prog.mu.Lock()
		if prog.Total < len(files) {
			prog.Total = len(files)
		}
		prog.Done = min(len(files), prog.Total)
		prog.Percent = 100
		prog.mu.Unlock()
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
