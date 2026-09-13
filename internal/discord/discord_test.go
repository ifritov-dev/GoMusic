package discord

import (
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
	a := buildActivity(Settings{Details: "Using GoMusic", ShowTime: true, ShowButton: true},
		TrackInfo{Active: true, Title: "Song", Artist: "Art", Length: 200 * time.Second, Position: 5 * time.Second, Playing: true})
	if a.State != "Listening to Song — Art" {
		t.Fatalf("state = %q", a.State)
	}
	if a.Details != "Using GoMusic" || a.Assets.LargeImage != LogoAsset || a.Assets.SmallImage != LogoAsset {
		t.Fatalf("assets/details wrong: %+v", a)
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

	p := buildActivity(Settings{}, TrackInfo{Active: true, Title: "Song", Playing: false})
	if p.State != "Paused · Song" || p.Timestamps != nil {
		t.Fatalf("paused wrong: %+v", p)
	}
	b := buildActivity(Settings{}, TrackInfo{})
	if b.State != "Browsing library" {
		t.Fatalf("idle wrong: %+v", b)
	}
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
