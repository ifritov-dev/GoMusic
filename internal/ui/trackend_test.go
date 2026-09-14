package ui

import (
	"testing"

	"github.com/ifritov-dev/GoMusic/internal/config"
)

// A manual switch (n/p/enter) closes the old Done channel, waking its
// waiter. That stale trackEndedMsg must be ignored — otherwise every
// manual switch cascades into another auto-advance and the queue
// visibly flies through the track list.
func TestStaleTrackEndedIgnored(t *testing.T) {
	m := testModel(99, 31) // cur=2, playing, 8 tracks
	m.cfg.Repeat = config.RepeatAll
	m.playGen = 5
	if cmd := m.onTrackEnded(4); cmd != nil {
		t.Fatal("stale trackEnded must return nil cmd")
	}
	if m.cur != 2 || !m.playing {
		t.Fatalf("stale trackEnded must not touch playback: cur=%d playing=%v", m.cur, m.playing)
	}
}

// End of queue with repeat OFF stops exactly once: a duplicate
// (already consumed) waiter message must not do anything.
func TestTrackEndedAtQueueEndOnce(t *testing.T) {
	m := testModel(99, 31)
	m.cfg.Repeat = config.RepeatOff
	m.cur = len(m.tracks) - 1 // last track
	m.playing = true
	m.playGen = 1
	m.onTrackEnded(1)
	if m.cur != -1 || m.playing {
		t.Fatalf("queue end must stop: cur=%d playing=%v", m.cur, m.playing)
	}
	if m.playGen != 2 {
		t.Fatalf("queue end must consume the generation: playGen=%d", m.playGen)
	}
	if cmd := m.onTrackEnded(1); cmd != nil {
		t.Fatal("duplicate trackEnded after stop must return nil cmd")
	}
	if m.cur != -1 {
		t.Fatalf("duplicate trackEnded must not restart: cur=%d", m.cur)
	}
}
