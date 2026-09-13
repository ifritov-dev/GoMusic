package library

import (
	"bytes"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dhowden/tag"
)

// Supported extensions (decoded natively, no external binaries).
var SupportedExts = map[string]bool{
	".mp3":  true,
	".flac": true,
	".wav":  true,
	".ogg":  true,
	".oga":  true,
}

// Track is one audio file with best-effort metadata.
type Track struct {
	Path   string
	Title  string
	Artist string
	Album  string
}

func Supported(path string) bool {
	return SupportedExts[strings.ToLower(filepath.Ext(path))]
}

// Scan walks dir recursively and returns sorted tracks.
// Tag reading failures fall back to the file name — a scan never fails
// hard because of one bad file.
func Scan(dir string) ([]Track, error) {
	var out []Track
	if dir == "" {
		return out, nil
	}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !Supported(path) {
			return nil
		}
		out = append(out, readTrack(path))
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, err
}

func readTrack(path string) Track {
	t := Track{Path: path, Title: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))}
	f, err := os.Open(path)
	if err != nil {
		return t
	}
	defer f.Close()
	m, err := tag.ReadFrom(f)
	if err != nil || m == nil {
		return t
	}
	if v := strings.TrimSpace(m.Title()); v != "" {
		t.Title = v
	}
	if v := strings.TrimSpace(m.Artist()); v != "" {
		t.Artist = v
	}
	if v := strings.TrimSpace(m.Album()); v != "" {
		t.Album = v
	}
	return t
}

// Cover returns the embedded cover art of path, or an error when the
// file has none / can't be decoded. Only the first picture is used.
func Cover(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	m, err := tag.ReadFrom(f)
	if err != nil || m == nil {
		return nil, err
	}
	pic := m.Picture()
	if pic == nil || len(pic.Data) == 0 {
		return nil, errNoCover
	}
	img, _, err := image.Decode(bytes.NewReader(pic.Data))
	if err != nil {
		return nil, err
	}
	return img, nil
}

var errNoCover = errNoCoverType{}

type errNoCoverType struct{}

func (errNoCoverType) Error() string { return "no cover art" }
