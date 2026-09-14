package ui

import (
	"testing"

	"github.com/ifritov-dev/GoMusic/internal/config"
)

// Shuffle must play every track exactly once per cycle: a repeat is
// only allowed after the whole library has played.
func TestShuffleCycleHasNoRepeats(t *testing.T) {
	m := testModel(99, 31)
	m.cfg.Shuffle = true
	m.cfg.Repeat = config.RepeatOff
	m.resetShuffle(m.cur) // cur=2 first

	seen := map[int]bool{m.cur: true}
	for i := 1; i < len(m.tracks); i++ {
		j := m.nextIndex(1)
		if j < 0 {
			t.Fatalf("cycle ended early at step %d", i)
		}
		if seen[j] {
			t.Fatalf("repeat within shuffle cycle: track %d at step %d", j, i)
		}
		seen[j] = true
		m.syncShuffleOnPlay(j)
		m.cur = j
	}
	// Repeat OFF: the queue stops at the end of the cycle.
	if j := m.nextIndex(1); j != -1 {
		t.Fatalf("repeat off must stop at cycle end, got %d", j)
	}
}

func TestShuffleRepeatAllStartsNewCycle(t *testing.T) {
	m := testModel(99, 31)
	m.cfg.Shuffle = true
	m.cfg.Repeat = config.RepeatAll
	m.resetShuffle(m.cur)

	// Exhaust the first cycle.
	for i := 1; i < len(m.tracks); i++ {
		j := m.nextIndex(1)
		if j < 0 {
			t.Fatalf("first cycle ended early at step %d", i)
		}
		m.syncShuffleOnPlay(j)
		m.cur = j
	}
	last := m.cur
	j := m.nextIndex(1)
	if j < 0 {
		t.Fatal("repeat all must start a new shuffle cycle")
	}
	if j == last {
		t.Fatal("new cycle must not start with the just-played track")
	}
	// The second cycle is also repeat-free.
	m.syncShuffleOnPlay(j)
	m.cur = j
	seen := map[int]bool{j: true}
	for i := 1; i < len(m.tracks); i++ {
		jj := m.nextIndex(1)
		if jj < 0 {
			t.Fatalf("second cycle ended early at step %d", i)
		}
		if seen[jj] {
			t.Fatalf("repeat within second shuffle cycle: %d", jj)
		}
		seen[jj] = true
		m.syncShuffleOnPlay(jj)
		m.cur = jj
	}
}

func TestShufflePrevWalksHistory(t *testing.T) {
	m := testModel(99, 31) // cur=2
	m.cfg.Shuffle = true
	m.cfg.Repeat = config.RepeatOff
	m.resetShuffle(m.cur)

	a := m.nextIndex(1)
	m.syncShuffleOnPlay(a)
	m.cur = a
	if back := m.nextIndex(-1); back != 2 {
		t.Fatalf("prev must return to the previous shuffled track, got %d want 2", back)
	}
}
