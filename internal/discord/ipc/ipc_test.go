//go:build !windows

package ipc

import (
	"encoding/binary"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeDiscord serves one handshake + one SET_ACTIVITY like a real client:
// framed JSON in, framed JSON out.
func fakeDiscord(t *testing.T, l net.Listener, gotHandshake, gotActivity chan json.RawMessage) {
	t.Helper()
	defer l.Close()
	conn, err := l.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	for _, ch := range []chan json.RawMessage{gotHandshake, gotActivity} {
		var hdr [8]byte
		if _, err := readFull(conn, hdr[:]); err != nil {
			t.Errorf("read header: %v", err)
			return
		}
		n := int(binary.LittleEndian.Uint32(hdr[4:8]))
		data := make([]byte, n)
		if _, err := readFull(conn, data); err != nil {
			t.Errorf("read payload: %v", err)
			return
		}
		op := int32(binary.LittleEndian.Uint32(hdr[0:4]))
		reply := `{"cmd":"DISPATCH","evt":"READY","data":{},"nonce":null}`
		if op == opFrame {
			reply = `{"cmd":"SET_ACTIVITY","evt":null,"data":{"state":"ok"},"nonce":"1"}`
		}
		var out [8]byte
		binary.LittleEndian.PutUint32(out[0:4], uint32(opPong))
		binary.LittleEndian.PutUint32(out[4:8], uint32(len(reply)))
		conn.Write(out[:])
		conn.Write([]byte(reply))
		ch <- data
	}
}

func readFull(conn net.Conn, b []byte) (int, error) {
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	total := 0
	for total < len(b) {
		n, err := conn.Read(b[total:])
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func TestDialAndSetActivity(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "discord-ipc-0")
	os.Remove(sock)
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	hs, act := make(chan json.RawMessage, 1), make(chan json.RawMessage, 1)
	go fakeDiscord(t, l, hs, act)

	cli, err := Dial(sock, "12345", 2*time.Second)
	if err != nil {
		t.Fatalf("dial+handshake: %v", err)
	}
	defer cli.Close()

	var h struct {
		V        int    `json:"v"`
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal(<-hs, &h); err != nil || h.ClientID != "12345" || h.V != 1 {
		t.Fatalf("bad handshake: %s", string(<-hs))
	}

	if err := cli.SetActivity("SET_ACTIVITY", map[string]any{
		"pid":      4242,
		"activity": map[string]any{"details": "Using GoMusic", "state": "Listening to X"},
	}); err != nil {
		t.Fatalf("set activity: %v", err)
	}
	var frame struct {
		Cmd  string `json:"cmd"`
		Args struct {
			Pid      int `json:"pid"`
			Activity struct {
				Details string `json:"details"`
				State   string `json:"state"`
			} `json:"activity"`
		} `json:"args"`
	}
	body := <-act
	if err := json.Unmarshal(body, &frame); err != nil {
		t.Fatalf("bad frame: %v: %s", err, string(body))
	}
	if frame.Cmd != "SET_ACTIVITY" || frame.Args.Pid != 4242 ||
		frame.Args.Activity.Details != "Using GoMusic" || frame.Args.Activity.State != "Listening to X" {
		t.Fatalf("bad activity frame: %s", string(body))
	}
}

func TestDialMissingSocket(t *testing.T) {
	if _, err := Dial(filepath.Join(t.TempDir(), "nope"), "1", time.Second); err == nil {
		t.Fatal("expected error for missing socket")
	}
}

func TestCandidatesCoverStandardDirs(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/4242")
	var foundXDG, foundTmp bool
	for _, c := range Candidates() {
		switch {
		case strings.HasPrefix(c, "/run/user/4242/discord-ipc-"):
			foundXDG = true
		case strings.HasPrefix(c, "/tmp/discord-ipc-"):
			foundTmp = true
		}
	}
	if !foundXDG || !foundTmp {
		t.Fatal("standard dirs missing from candidates")
	}
	// discord-ipc-0..9 range present per dir.
	n := 0
	for _, c := range Candidates() {
		if strings.HasPrefix(c, "/tmp/discord-ipc-") {
			n++
		}
	}
	if n != 10 {
		t.Fatalf("want 10 tmp candidates, got %d", n)
	}
}

func TestFlatpakRunDirs(t *testing.T) {
	base := t.TempDir()
	app := filepath.Join(base, ".flatpak", "com.discordapp.Discord", "xdg-run")
	if err := os.MkdirAll(app, 0o755); err != nil {
		t.Fatal(err)
	}
	dirs := flatpakRunDirs(base)
	if len(dirs) != 1 || dirs[0] != app {
		t.Fatalf("flatpak dirs = %v", dirs)
	}
}
