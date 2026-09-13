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
	files, err := Download(context.Background(), "yt-dlp", "https://youtu.be/x", music)
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
