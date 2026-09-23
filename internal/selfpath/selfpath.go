// Package selfpath makes the gomusic command available from any
// terminal: on startup the app checks whether it is reachable as
// "gomusic" on PATH and, if not, installs itself once following each
// OS conventions (Linux/macOS: ~/.local/bin symlink, Windows:
// %LOCALAPPDATA%\gomusic + user PATH). It never fails startup.
package selfpath

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Name is the command made available on PATH.
const Name = "gomusic"

// Ensure makes sure the current binary is reachable as Name from any
// terminal. It returns "" when everything is fine (or was fixed
// silently) and a one-line manual hint otherwise. It never fails.
func Ensure() string {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		return ""
	}
	if _, err := exec.LookPath(Name); err == nil {
		return ""
	}
	// `go run` uses a throwaway binary in the temp dir: installing it
	// would leave a dead copy behind, so skip it silently.
	if inTemp(exe) {
		return ""
	}
	if runtime.GOOS == "windows" {
		return installWindowsExe(exe)
	}
	return installUnixExe(exe)
}

func inTemp(exe string) bool {
	tmp := os.TempDir()
	if tmp == "" {
		return false
	}
	rel, err := filepath.Rel(tmp, exe)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

// dirInPath reports whether dir is on the PATH-style list env.
func dirInPath(env, dir string) bool {
	dir = filepath.Clean(dir)
	for _, e := range strings.Split(env, string(os.PathListSeparator)) {
		if e == "" {
			continue
		}
		if runtime.GOOS == "windows" {
			if strings.EqualFold(filepath.Clean(e), dir) {
				return true
			}
			continue
		}
		if filepath.Clean(e) == dir {
			return true
		}
	}
	return false
}

func sameFile(a, b string) bool {
	ra, err := filepath.EvalSymlinks(a)
	if err != nil {
		return false
	}
	rb, err := filepath.EvalSymlinks(b)
	if err != nil {
		return false
	}
	if ra == rb {
		return true
	}
	sa, err := os.Stat(ra)
	if err != nil {
		return false
	}
	sb, err := os.Stat(rb)
	if err != nil {
		return false
	}
	return os.SameFile(sa, sb)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
