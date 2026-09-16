//go:build !windows

package selfpath

import (
	"os"
	"path/filepath"
)

// installWindowsExe is unavailable on unix (the windows build provides it).
func installWindowsExe(string) string { return "" }

// installUnix links the running binary as ~/.local/bin/gomusic.
// A symlink (not a copy) means it never goes stale.
func installUnixExe(exe string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	dir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	target := filepath.Join(dir, Name)
	if !sameFile(target, exe) {
		_ = os.Remove(target) // ours from a previous run, or a stale copy
		if err := os.Symlink(exe, target); err != nil {
			if err := copyFile(exe, target); err != nil {
				return ""
			}
		}
	}
	if !dirInPath(os.Getenv("PATH"), dir) {
		return "gomusic installed to " + target + ` — add to PATH: export PATH="$HOME/.local/bin:$PATH"`
	}
	return ""
}
