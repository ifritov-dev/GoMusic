package selfpath

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDirInPath(t *testing.T) {
	sep := string(os.PathListSeparator)
	env := strings.Join([]string{"/usr/bin", string(filepath.Separator) + "home" + string(filepath.Separator) + "u" + string(filepath.Separator) + ".local" + string(filepath.Separator) + "bin"}, sep)
	if !dirInPath(env, "/home/u/.local/bin/") {
		t.Fatal("trailing slash must still match")
	}
	if dirInPath(env, "/home/u/.local/bint") {
		t.Fatal("prefix must not match")
	}
	if dirInPath("", "/home/u/.local/bin") {
		t.Fatal("empty env must not match")
	}
	if runtime.GOOS == "windows" {
		if !dirInPath(`C:\A;D:\Tools`, `d:\tools`) {
			t.Fatal("windows match must be case-insensitive")
		}
	}
}

func TestEnsureSkipsTempBinary(t *testing.T) {
	// The test binary itself lives in the temp dir, and PATH below has
	// no gomusic: Ensure must return "" without installing anything.
	t.Setenv("PATH", t.TempDir())
	if !inTemp(os.Args[0]) {
		t.Skip("test binary is not in temp dir, nothing to assert")
	}
	if hint := Ensure(); hint != "" {
		t.Fatalf("temp binary must be skipped silently, got %q", hint)
	}
}

func TestCopyFileRoundtrip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.bin")
	if err := os.WriteFile(src, []byte("gomusic"), 0o755); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "sub", "b.bin")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	// copyFile does not create parents (callers MkdirAll first).
	dst = filepath.Join(dir, "b.bin")
	if err := copyFile(src, dst); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dst)
	if err != nil || string(data) != "gomusic" {
		t.Fatalf("roundtrip mismatch: %q %v", data, err)
	}
}
