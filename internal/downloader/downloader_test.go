package downloader

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidURL(t *testing.T) {
	valid := []string{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://youtube.com/watch?v=dQw4w9WgXcQ&list=PL123",
		"https://music.youtube.com/watch?v=dQw4w9WgXcQ&list=OLAK123",
		"https://music.youtube.com/playlist?list=OLAK123",
		"https://m.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ",
		"  https://youtu.be/dQw4w9WgXcQ  ",
	}
	for _, u := range valid {
		if _, err := ValidURL(u); err != nil {
			t.Errorf("ValidURL(%q) = %v, want nil", u, err)
		}
	}
	invalid := []string{
		"",
		"   ",
		"not a link",
		"https://vimeo.com/123",
		"https://notyoutube.com/watch?v=1",
		"ftp://youtube.com/watch?v=1",
		"youtube.com/watch?v=1", // no scheme
	}
	for _, u := range invalid {
		if _, err := ValidURL(u); err == nil {
			t.Errorf("ValidURL(%q) = nil, want error", u)
		}
	}
}

// fakeYTDLP installs a stub yt-dlp on PATH: check phase (flat-playlist)
// prints two titles, download phase materializes two mp3s under the -o dir.
func fakeYTDLP(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
flat=0
out=""
prev=""
for a in "$@"; do
  if [ "$a" = "--flat-playlist" ]; then flat=1; fi
  if [ "$prev" = "-o" ]; then out="$a"; fi
  prev="$a"
done
if [ "$flat" = "1" ]; then
  printf 'Song One\nSong Two\n'
  exit 0
fi
# Lock the tagging recipe: downloads must request metadata + thumbnail.
case " $* " in
  *" --add-metadata "*"--embed-thumbnail "*) ;;
  *) echo "missing tagging flags" >&2; exit 1 ;;
esac
d=$(dirname "$out")
mkdir -p "$d"
touch "$d/Song One.mp3" "$d/Song Two.mp3"
printf '%s\n' "$d/Song One.mp3" "$d/Song Two.mp3"
`
	p := filepath.Join(dir, "yt-dlp")
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestCheck(t *testing.T) {
	fakeYTDLP(t)
	title, n, err := Check(context.Background(), "yt-dlp", "https://music.youtube.com/playlist?list=X")
	if err != nil {
		t.Fatal(err)
	}
	if title != "Song One" || n != 2 {
		t.Fatalf("Check = (%q, %d), want (Song One, 2)", title, n)
	}
}

func TestDownload(t *testing.T) {
	fakeYTDLP(t)
	music := t.TempDir()
	files, err := DownloadWithProgress(context.Background(), "yt-dlp", "https://youtu.be/x", music, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || !strings.HasSuffix(files[0], "Song One.mp3") {
		t.Fatalf("Download files = %v", files)
	}
	for _, f := range files {
		if _, err := os.Stat(f); err != nil {
			t.Fatalf("missing downloaded file %s: %v", f, err)
		}
	}
}

func TestBinsMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no binaries here
	if _, _, err := Bins(); err == nil || !strings.Contains(err.Error(), "yt-dlp") {
		t.Fatalf("Bins = %v, want yt-dlp error", err)
	}
}

func TestProgressObserve(t *testing.T) {
	p := NewProgress(0) // clamps to 1
	if total, _, _, _ := p.Snapshot(); total != 1 {
		t.Fatalf("NewProgress(0).Total = %d, want 1", total)
	}
	p.observe("@@ITEM@@:3:12:Some Song")
	if total, done, cur, pct := p.Snapshot(); total != 12 || done != 2 || cur != "Some Song" || pct != 0 {
		t.Fatalf("item marker = (%d,%d,%q,%v), want (12,2,Some Song,0)", total, done, cur, pct)
	}
	p.observe("[download]  45.2% of ~4.0MiB at 1.0MiB/s ETA 00:02")
	if _, _, _, pct := p.Snapshot(); pct != 45.2 {
		t.Fatalf("percent = %v, want 45.2", pct)
	}
	if ov := p.Overall(); ov < 0.204 || ov > 0.205 {
		t.Fatalf("overall = %v, want ~(2+0.452)/12", ov)
	}
	p.observe("[ExtractAudio] Destination: Song.mp3")
	if _, _, _, pct := p.Snapshot(); pct != 100 {
		t.Fatalf("post-process percent = %v, want 100", pct)
	}
	p.observe("[download] Downloading item 4 of 12")
	if _, done, _, pct := p.Snapshot(); done != 3 || pct != 0 {
		t.Fatalf("fallback marker = done %d pct %v, want done 3 pct 0", done, pct)
	}
	p.observe("[download] Song Two has already been downloaded and merged")
	if _, _, _, pct := p.Snapshot(); pct != 100 {
		t.Fatalf("skipped percent = %v, want 100", pct)
	}
	// Singles carry NA indexes: title still lands, total untouched,
	// and a new item always restarts its percent.
	p.observe("@@ITEM@@:NA:NA:Lone Track")
	if total, _, cur, pct := p.Snapshot(); total != 12 || cur != "Lone Track" || pct != 0 {
		t.Fatalf("single marker = (%d,%q,%v), want (12,Lone Track,0)", total, cur, pct)
	}
	// Garbage never corrupts the snapshot.
	p.observe("random noise [[[")
	if total, done, cur, pct := p.Snapshot(); total != 12 || done != 3 || cur != "Lone Track" || pct != 0 {
		t.Fatalf("after noise = (%d,%d,%q,%v), want (12,3,Lone Track,0)", total, done, cur, pct)
	}
}

// fakeProgressYTDLP stubs yt-dlp with playlist markers on stdout,
// percentages on stderr, then two materialized mp3s.
func fakeProgressYTDLP(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
out=""
prev=""
for a in "$@"; do
  if [ "$prev" = "-o" ]; then out="$a"; fi
  prev="$a"
done
printf '@@ITEM@@:1:2:Song One\n'
echo '[download]  50.0% of ~3MiB' >&2
echo '[download] 100% of ~3MiB' >&2
printf '@@ITEM@@:2:2:Song Two\n'
echo '[download] Song Two has already been downloaded' >&2
d=$(dirname "$out")
mkdir -p "$d"
touch "$d/Song One.mp3" "$d/Song Two.mp3"
printf '%s\n' "$d/Song One.mp3" "$d/Song Two.mp3"
`
	p := filepath.Join(dir, "yt-dlp")
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestDownloadWithProgress(t *testing.T) {
	fakeProgressYTDLP(t)
	music := t.TempDir()
	prog := NewProgress(2)
	files, err := DownloadWithProgress(context.Background(), "yt-dlp", "https://music.youtube.com/playlist?list=X", music, prog)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("files = %v, want 2", files)
	}
	total, done, cur, pct := prog.Snapshot()
	if total != 2 || done != 2 || cur != "Song Two" || pct != 100 {
		t.Fatalf("snapshot = (%d,%d,%q,%v), want (2,2,Song Two,100)", total, done, cur, pct)
	}
}
