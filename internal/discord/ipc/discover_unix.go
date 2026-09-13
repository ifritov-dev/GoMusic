//go:build !windows

package ipc

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func dialAddr(addr string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout("unix", addr, timeout)
}

// socketCandidates lists IPC socket paths to try, in order: standard
// runtime dirs, then sandboxed installs (Flatpak/Snap expose the app
// sandbox runtime on the host), then sockets found by scanning for
// Discord processes (custom prefixes, other UIDs, distrobox, ...).
func Candidates() []string {
	var dirs []string
	seenDirs := map[string]bool{}
	addDir := func(d string) {
		if d != "" && !seenDirs[d] {
			seenDirs[d] = true
			dirs = append(dirs, d)
		}
	}
	addDir(os.Getenv("XDG_RUNTIME_DIR"))
	if runtime.GOOS == "darwin" {
		tmp := os.Getenv("TMPDIR")
		if tmp == "" {
			tmp = "/tmp"
		}
		addDir(tmp)
	}
	addDir("/tmp")
	addDir("/run/user/" + strconv.Itoa(os.Getuid()))
	// Sandboxed Discord (Flatpak/Snap) on the host, dynamic UID.
	base := "/run/user/" + strconv.Itoa(os.Getuid())
	for _, d := range flatpakRunDirs(base) {
		addDir(d)
	}
	// Flatpak exposes each sandbox runtime at <runtime>/app/<appid>/
	// (e.g. .../app/com.discordapp.Discord/discord-ipc-0).
	for _, d := range appSandboxDirs(os.Getenv("XDG_RUNTIME_DIR"), base) {
		addDir(d)
	}
	addDir(base + "/snap.discord")

	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	for _, d := range dirs {
		for i := 0; i < 10; i++ {
			add(filepath.Join(d, "discord-ipc-"+strconv.Itoa(i)))
		}
	}
	// Whatever the layout, a socket owned by a Discord process works.
	for _, p := range procSockets() {
		add(p)
	}
	return out
}

// flatpakRunDirs globs the sandbox runtime dirs Flatpak exposes on the
// host: /run/user/<uid>/.flatpak/<app>/xdg-run.
func flatpakRunDirs(base string) []string {
	matches, _ := filepath.Glob(filepath.Join(base, ".flatpak", "*", "xdg-run"))
	return matches
}

// appSandboxDirs returns <base>/app/<appid> dirs that actually contain a
// discord-ipc socket — any sandboxed app (Discord, Vesktop, ...) that
// exposes its runtime on the host.
func appSandboxDirs(bases ...string) []string {
	var out []string
	for _, base := range bases {
		if base == "" {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(base, "app"))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			d := filepath.Join(base, "app", e.Name())
			if matches, _ := filepath.Glob(filepath.Join(d, "discord-ipc-*")); len(matches) > 0 {
				out = append(out, d)
			}
		}
	}
	return out
}

// procSockets finds discord-ipc sockets through /proc/<pid>/root for
// Discord processes (Linux). Same-user processes are readable, so this
// reaches sockets in namespaces with no host-visible path.
func procSockets() []string {
	if runtime.GOOS != "linux" {
		return nil
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		comm, err := os.ReadFile(filepath.Join("/proc", e.Name(), "comm"))
		if err != nil {
			continue
		}
		if !strings.Contains(strings.ToLower(strings.TrimSpace(string(comm))), "discord") {
			continue
		}
		root := filepath.Join("/proc", e.Name(), "root")
		for _, base := range []string{"run", "tmp"} {
			for i := 0; i < 3; i++ {
				p := filepath.Join(root, base, "discord-ipc-"+strconv.Itoa(i))
				if st, err := os.Stat(p); err == nil && st.Mode()&os.ModeSocket != 0 {
					out = append(out, p)
				}
				// XDG_RUNTIME_DIR inside the namespace.
				pu := filepath.Join(root, "run", "user")
				users, _ := os.ReadDir(pu)
				for _, u := range users {
					q := filepath.Join(pu, u.Name(), "discord-ipc-"+strconv.Itoa(i))
					if st, err := os.Stat(q); err == nil && st.Mode()&os.ModeSocket != 0 {
						out = append(out, q)
					}
				}
			}
		}
	}
	return out
}
