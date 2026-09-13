package library

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestScanFindsSupportedOnly(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.mp3", "b.flac", "c.wav", "d.ogg", "e.txt", "f.jpg"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "g.mp3"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	tracks, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 5 {
		t.Fatalf("got %d tracks, want 5", len(tracks))
	}
	for _, tr := range tracks {
		if tr.Title == "" || tr.Path == "" {
			t.Fatal("track missing title/path")
		}
	}
	// Sorted by path.
	for i := 1; i < len(tracks); i++ {
		if tracks[i-1].Path > tracks[i].Path {
			t.Fatal("tracks not sorted")
		}
	}
}

func TestScanMissingDir(t *testing.T) {
	tracks, _ := Scan(filepath.Join(t.TempDir(), "nope"))
	if len(tracks) != 0 {
		t.Fatal("want empty")
	}
}

func TestCoverFromID3APIC(t *testing.T) {
	// Build a 16x16 PNG in memory.
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.Set(x, y, color.NRGBA{R: 200, G: 50, B: 50, A: 255})
		}
	}
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		t.Fatal(err)
	}

	// Minimal ID3v2.3 tag: TIT2 + APIC frame (no audio needed for tags).
	apic := new(bytes.Buffer)
	apic.WriteByte(0x00) // encoding latin1
	apic.WriteString("image/png\x00")
	apic.WriteByte(0x03) // front cover
	apic.WriteByte(0x00) // empty description
	apic.Write(pngBuf.Bytes())
	tit2 := new(bytes.Buffer)
	tit2.WriteByte(0x00)
	tit2.WriteString("Cover Test")

	tag := new(bytes.Buffer)
	writeFrame := func(id string, data []byte) {
		tag.WriteString(id)
		var sz [4]byte
		binary.BigEndian.PutUint32(sz[:], uint32(len(data)))
		tag.Write(sz[:])
		tag.Write([]byte{0x00, 0x00})
		tag.Write(data)
	}
	writeFrame("TIT2", tit2.Bytes())
	writeFrame("APIC", apic.Bytes())

	var out bytes.Buffer
	out.WriteString("ID3")
	out.Write([]byte{0x03, 0x00, 0x00})
	syncSize(tag.Len(), &out)
	out.Write(tag.Bytes())

	path := filepath.Join(t.TempDir(), "cover.mp3")
	if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Cover(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Bounds().Dx() != 16 || got.Bounds().Dy() != 16 {
		t.Fatalf("cover size = %v, want 16x16", got.Bounds())
	}

	if _, err := Cover(filepath.Join(t.TempDir(), "missing.mp3")); err == nil {
		t.Fatal("want error for missing file")
	}
}

func syncSize(n int, w *bytes.Buffer) {
	w.Write([]byte{
		byte(n>>21) & 0x7f,
		byte(n>>14) & 0x7f,
		byte(n>>7) & 0x7f,
		byte(n) & 0x7f,
	})
}
