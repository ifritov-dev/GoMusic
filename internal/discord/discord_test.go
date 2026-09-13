package discord

import (
	"encoding/binary"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// No Discord client runs in CI: Login fails and every call must degrade
// to a silent no-op instead of blocking or panicking.
func TestDisabledIsNoop(t *testing.T) {
	m := New()
	m.Configure(Settings{})
	m.Update(TrackInfo{Active: true, Title: "T", Artist: "A", Playing: true})
	m.Shutdown()
}

func TestEnabledWithoutClientKeepsRetrying(t *testing.T) {
	m := New()
	m.Configure(Settings{Enabled: true, ClientID: "000000000000000000"})
	m.Update(TrackInfo{Active: true, Title: "T", Length: time.Minute, Playing: true})
	m.Update(TrackInfo{Active: true, Title: "T", Playing: false})
	m.Update(TrackInfo{})
	m.Configure(Settings{Enabled: false})
	m.Shutdown()
}

func TestBuildActivity(t *testing.T) {
	start := time.Now()
	a := buildActivity(Settings{ShowTime: true, ShowButton: true},
		TrackInfo{Active: true, Title: "Song", Artist: "Art", Length: 200 * time.Second, Position: 5 * time.Second, Playing: true})
	if a.Type != 2 {
		t.Fatalf("type = %d, want Listening(2)", a.Type)
	}
	if a.Details != "Song" {
		t.Fatalf("details = %q, want title only", a.Details)
	}
	if a.State != "Art" {
		t.Fatalf("state = %q, want artist only", a.State)
	}
	if len(a.Buttons) != 1 || a.Buttons[0].Label != "GitHub" {
		t.Fatalf("button wrong: %+v", a.Buttons)
	}
	if a.Name != "GoMusic" {
		t.Fatalf("name = %q", a.Name)
	}
	if a.Assets == nil || a.Assets.LargeImage != LogoAsset {
		t.Fatalf("logo asset wrong: %+v", a.Assets)
	}
	if a.Timestamps == nil || a.Timestamps.End == nil {
		t.Fatal("expected start+end timestamps while playing")
	}
	// Millis: end ≈ now + length − position.
	wantEnd := uint64(start.Add(195*time.Second).UnixNano() / 1e6)
	if got := *a.Timestamps.End; got < wantEnd-5000 || got > wantEnd+5000 {
		t.Fatalf("end timestamp off: got %d want ~%d", got, wantEnd)
	}
	if len(a.Buttons) != 1 || a.Buttons[0].Url != GitHubURL {
		t.Fatalf("buttons wrong: %+v", a.Buttons)
	}

	p := buildActivity(Settings{}, TrackInfo{Active: true, Title: "Song", Artist: "A", Playing: false})
	if p.Details != "Paused · Song" || p.State != "A" || p.Timestamps != nil {
		t.Fatalf("paused wrong: %+v", p)
	}
	u := buildActivity(Settings{}, TrackInfo{Active: true, Title: "Song", Playing: true})
	if u.State != "Unknown Artist" {
		t.Fatalf("artist fallback wrong: %+v", u)
	}
	b := buildActivity(Settings{}, TrackInfo{})
	if b.Details != "Browsing library" {
		t.Fatalf("idle wrong: %+v", b)
	}
	if got := buildActivity(Settings{}, TrackInfo{Active: true, Title: strings.Repeat("x", 200), Playing: true}); len([]rune(got.Details)) > 128 {
		t.Fatalf("details not truncated: %d runes", len([]rune(got.Details)))
	}
}

// gatedServer is a fake Discord: the first connection gets a slow
// handshake (1s), later ones are instant. Received activity states go
// to states.
type gatedServer struct {
	t        *testing.T
	l        net.Listener
	accepted chan struct{}
	states   chan string
}

func startGatedServer(t *testing.T, sock string) *gatedServer {
	t.Helper()
	os.Remove(sock)
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	g := &gatedServer{t: t, l: l, accepted: make(chan struct{}, 16), states: make(chan string, 64)}
	go g.serve()
	return g
}

func (g *gatedServer) serve() {
	n := 0
	for {
		conn, err := g.l.Accept()
		if err != nil {
			return
		}
		n++
		g.accepted <- struct{}{}
		go g.session(conn, n == 1)
	}
}

func (g *gatedServer) session(conn net.Conn, slow bool) {
	defer conn.Close()
	readFrame := func() (int32, json.RawMessage) {
		var hdr [8]byte
		conn.SetDeadline(time.Now().Add(10 * time.Second))
		if _, err := readFull(conn, hdr[:]); err != nil {
			return -1, nil
		}
		n := int(binary.LittleEndian.Uint32(hdr[4:8]))
		data := make([]byte, n)
		if _, err := readFull(conn, data); err != nil {
			return -1, nil
		}
		return int32(binary.LittleEndian.Uint32(hdr[0:4])), data
	}
	reply := func() {
		raw := []byte(`{"cmd":"DISPATCH","evt":"READY","data":{},"nonce":null}`)
		var out [8]byte
		binary.LittleEndian.PutUint32(out[0:4], 1)
		binary.LittleEndian.PutUint32(out[4:8], uint32(len(raw)))
		conn.SetDeadline(time.Now().Add(10 * time.Second))
		conn.Write(out[:])
		conn.Write(raw)
	}
	if slow {
		// Hold the first handshake: any push in flight here is stale
		// by the time it proceeds and must drop itself.
		time.Sleep(time.Second)
	}
	for {
		op, data := readFrame()
		if op < 0 {
			return
		}
		if op == 0 {
			reply()
			continue
		}
		var f struct {
			Args struct {
				Activity struct {
					Details string `json:"details"`
				} `json:"activity"`
			} `json:"args"`
		}
		if err := json.Unmarshal(data, &f); err == nil {
			g.states <- f.Args.Activity.Details
		}
		reply()
	}
}

func readFull(conn net.Conn, b []byte) (int, error) {
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

func active(title string) TrackInfo {
	return TrackInfo{Active: true, Title: title, Length: time.Minute, Playing: true}
}

// A push stuck in a slow handshake while newer snapshots arrive must
// never hit the wire afterwards (no stale presence flashes).
func TestStalePushDropped(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	g := startGatedServer(t, filepath.Join(dir, "discord-ipc-0"))
	defer g.l.Close()

	m := New()
	m.Configure(Settings{Enabled: true, ClientID: "1"})
	<-g.accepted          // first push in slow handshake now
	m.Update(active("A")) // superseded almost immediately
	time.Sleep(200 * time.Millisecond)
	m.Update(active("B")) // latest; must be the last word
	time.Sleep(2500 * time.Millisecond)

	var got []string
	for {
		select {
		case st := <-g.states:
			got = append(got, st)
		default:
			goto done
		}
	}
done:
	if len(got) == 0 {
		t.Fatal("no activity received")
	}
	if got[len(got)-1] != "B" {
		t.Fatalf("last state = %q, want B; all: %v", got[len(got)-1], got)
	}
	for i := 1; i < len(got); i++ {
		if got[i] != "B" && got[i-1] == "B" {
			t.Fatalf("stale state after B: %v", got)
		}
	}
	m.Shutdown()
}

func TestConcurrentUse(t *testing.T) {
	m := New()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			m.Configure(Settings{Enabled: i%2 == 0, ClientID: "x"})
			m.Update(TrackInfo{Active: true, Title: "T", Playing: true})
		}(i)
	}
	wg.Wait()
	m.Shutdown()
}
