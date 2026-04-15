package watch

import "time"

// Clock abstracts the two time-related calls watch relies on so tests can
// advance time deterministically instead of sleeping. The real implementation
// is a thin wrapper around `time.Now` and `time.AfterFunc`; the fake (in
// clock_test.go helpers) exposes a manual `Advance` hook.
type Clock interface {
	// Now returns the current time.
	Now() time.Time

	// AfterFunc schedules f to run after d. The returned Timer allows
	// callers to Reset or Stop the pending call — matching time.Timer
	// semantics closely enough for per-file debouncing.
	AfterFunc(d time.Duration, f func()) Timer
}

// Timer is the minimal subset of *time.Timer the watcher uses. Methods match
// stdlib semantics so RealClock can return raw *time.Timer values.
type Timer interface {
	// Stop cancels a not-yet-fired callback. Returns true if the call was
	// stopped before firing, false if it already fired (or was already
	// stopped).
	Stop() bool

	// Reset reschedules a pending timer to fire after d instead of the
	// original duration. Matches time.Timer.Reset — only safe to call on
	// stopped or already-fired timers, per stdlib docs.
	Reset(d time.Duration) bool
}

// RealClock delegates to stdlib `time`. Use this in production.
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }

func (RealClock) AfterFunc(d time.Duration, f func()) Timer {
	return realTimer{time.AfterFunc(d, f)}
}

// realTimer adapts *time.Timer to our Timer interface so tests can swap in a
// fake without the caller knowing which side it got.
type realTimer struct{ t *time.Timer }

func (rt realTimer) Stop() bool                 { return rt.t.Stop() }
func (rt realTimer) Reset(d time.Duration) bool { return rt.t.Reset(d) }
