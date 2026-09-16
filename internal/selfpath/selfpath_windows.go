//go:build windows

package selfpath

import (
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// installWindowsExe copies the running binary to
// %LOCALAPPDATA%\gomusic\gomusic.exe and appends that dir to the user
// PATH (HKCU\Environment), then broadcasts WM_SETTINGCHANGE so already
// running shells (cmd, PowerShell, conhost-era terminals that never
// re-read the environment) pick it up without a logoff.
func installWindowsExe(exe string) string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		if home, err := os.UserHomeDir(); err == nil {
			base = filepath.Join(home, "AppData", "Local")
		}
	}
	if base == "" {
		return ""
	}
	dir := filepath.Join(base, "gomusic")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	target := filepath.Join(dir, Name+".exe")
	if !sameFile(target, exe) {
		if err := copyFile(exe, target); err != nil {
			return ""
		}
	}
	if !dirInPath(os.Getenv("PATH"), dir) {
		if err := appendUserPath(dir); err != nil {
			return "gomusic installed to " + target + " — reopen the terminal; if `gomusic` is still unknown, add " + dir + " to PATH"
		}
		// Async on purpose: the broadcast waits on third-party windows
		// (SMTO_ABORTIFHUNG skips hung ones), startup never blocks on it.
		go broadcastEnvironmentChange()
	}
	return ""
}

// installUnixExe is unavailable on windows (unix build provides it).
func installUnixExe(string) string { return "" }

func appendUserPath(dir string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	cur, _, err := k.GetStringValue("Path")
	if err != nil && err != registry.ErrNotExist {
		return err
	}
	if dirInPath(cur, dir) {
		return nil
	}
	if cur == "" {
		return k.SetStringValue("Path", dir)
	}
	return k.SetStringValue("Path", cur+";"+dir)
}

// broadcastEnvironmentChange tells all top-level windows the user
// environment changed, so cmd / PowerShell / older terminals re-read
// PATH (including our dir) without a logoff or reboot.
func broadcastEnvironmentChange() {
	const (
		hwndBroadcast   = 0xFFFF
		wmSettingChange = 0x001A
		smtoAbortIfHung = 0x0002
	)
	user32 := windows.NewLazySystemDLL("user32.dll")
	sendMessageTimeout := user32.NewProc("SendMessageTimeoutW")
	param, _ := windows.UTF16PtrFromString("Environment")
	_, _, _ = sendMessageTimeout.Call(
		hwndBroadcast, wmSettingChange, 0,
		uintptr(unsafe.Pointer(param)),
		smtoAbortIfHung, 5000, 0,
	)
}
