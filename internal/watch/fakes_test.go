package watch

import (
	"sort"
	"sync"
	"time"
)

// fakeClock is a test-only Clock that doesn't actually sleep. Time
// advances only when Advance is called; AfterFunc callbacks fire when
// their deadline is reached by a call to Advance.
//
// Single-goroutine model: tests call Enqueue / Advance on the main test
// goroutine; callbacks run synchronously inside Advance. This avoids the
// classic flake of time-based tests where "sleep 300ms, then assert" races
// against scheduler jitter.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Unix(1_700_000_000, 0)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) AfterFunc(d time.Duration, f func()) Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{clock: c, due: c.now.Add(d), fn: f, live: true}
	c.timers = append(c.timers, t)
	return t
}

// Advance moves time forward by d, firing every live timer whose due time
// is at or before the new now. Callbacks run synchronously (in the
// test goroutine) so post-Advance assertions see all side effects.
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	due := c.now
	// Collect fireable timers in due-order; mark dead.
	var fire []*fakeTimer
	for _, t := range c.timers {
		if t.live && !t.due.After(due) {
			fire = append(fire, t)
			t.live = false
		}
	}
	c.mu.Unlock()

	sort.Slice(fire, func(i, j int) bool { return fire[i].due.Before(fire[j].due) })
	for _, t := range fire {
		t.fn()
	}
}

type fakeTimer struct {
	clock *fakeClock
	due   time.Time
	fn    func()
	live  bool
}

func (t *fakeTimer) Stop() bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	if !t.live {
		return false
	}
	t.live = false
	return true
}

func (t *fakeTimer) Reset(d time.Duration) bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	wasLive := t.live
	t.due = t.clock.now.Add(d)
	t.live = true
	return wasLive
}
