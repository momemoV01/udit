package watch

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// recordingExecutor is a test Executor that records each Run call and can
// be programmed to block until released (for testing queuing / max_parallel).
type recordingExecutor struct {
	mu   sync.Mutex
	runs []recordedRun

	// block controls whether Run blocks until released. When non-nil,
	// every Run receives from the channel before returning.
	block chan struct{}
	// result is returned by every Run. Zero code + nil err = success.
	code int
	err  error

	inFlight     atomic.Int32
	peakInFlight atomic.Int32
}

type recordedRun struct {
	Argv []string
	Env  []string
}

func (r *recordingExecutor) Run(ctx context.Context, argv []string, env []string) (int, error) {
	cur := r.inFlight.Add(1)
	for {
		peak := r.peakInFlight.Load()
		if cur <= peak {
			break
		}
		if r.peakInFlight.CompareAndSwap(peak, cur) {
			break
		}
	}
	if r.block != nil {
		<-r.block
	}
	r.mu.Lock()
	r.runs = append(r.runs, recordedRun{Argv: append([]string(nil), argv...), Env: append([]string(nil), env...)})
	r.mu.Unlock()
	r.inFlight.Add(-1)
	return r.code, r.err
}

func (r *recordingExecutor) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.runs)
}

func (r *recordingExecutor) snapshot() []recordedRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]recordedRun, len(r.runs))
	copy(out, r.runs)
	return out
}

// waitUntil polls fn up to timeout, returning true when fn returns true.
// Used to synchronize async test expectations without sleeps.
func waitUntil(fn func() bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return fn()
}

func newTestLogger() (Logger, *[]string) {
	var mu sync.Mutex
	lines := []string{}
	logger := func(format string, args ...interface{}) {
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, format)
	}
	return logger, &lines
}

func TestRunner_Dispatch_SingleEvent(t *testing.T) {
	ex := &recordingExecutor{}
	cfg := (&WatchCfg{Hooks: []Hook{{Name: "h", Paths: []string{"*"}, Run: "refresh --compile"}}}).Defaults()
	r := NewRunner(cfg, "", ex, RealClock{}, func(string, ...interface{}) {})
	defer r.Shutdown(context.Background())

	r.Dispatch(&cfg.Hooks[0], Event{Path: "/x/A.cs", Kind: "write"})

	if !waitUntil(func() bool { return ex.count() >= 1 }, 2*time.Second) {
		t.Fatalf("executor not invoked; count=%d", ex.count())
	}
	runs := ex.snapshot()
	if runs[0].Argv[0] != "refresh" || runs[0].Argv[1] != "--compile" {
		t.Errorf("argv: %v", runs[0].Argv)
	}
}

func TestRunner_Queue_DedupesWhileRunning(t *testing.T) {
	// Buffered block channel so multiple releases can be queued without
	// receiver-side synchronization — lets the test pace without
	// deadlocking between runs.
	ex := &recordingExecutor{block: make(chan struct{}, 8)}
	cfg := (&WatchCfg{
		OnBusy: "queue",
		Hooks:  []Hook{{Name: "h", Paths: []string{"*"}, Run: "refresh --compile"}},
	}).Defaults()
	r := NewRunner(cfg, "", ex, RealClock{}, func(string, ...interface{}) {})
	defer r.Shutdown(context.Background())

	// First dispatch: worker spawns, Run() blocks on the channel.
	r.Dispatch(&cfg.Hooks[0], Event{Path: "/x/A.cs", Kind: "write"})
	if !waitUntil(func() bool { return ex.inFlight.Load() == 1 }, 2*time.Second) {
		t.Fatalf("first invocation not in flight")
	}
	// Pile on 5 same-path dispatches (dedupe) + 1 different-path.
	for i := 0; i < 5; i++ {
		r.Dispatch(&cfg.Hooks[0], Event{Path: "/x/A.cs", Kind: "write"})
	}
	r.Dispatch(&cfg.Hooks[0], Event{Path: "/x/B.cs", Kind: "write"})
	// Release both pending run slots ahead of time so the buffered channel
	// has tokens waiting for each Run invocation to pick up.
	ex.block <- struct{}{}
	ex.block <- struct{}{}
	// Wait for both runs to complete.
	if !waitUntil(func() bool { return ex.count() >= 2 }, 2*time.Second) {
		t.Fatalf("expected 2 runs, got %d", ex.count())
	}
	// Let the worker exit cleanly.
	time.Sleep(50 * time.Millisecond)
	if c := ex.count(); c != 2 {
		t.Errorf("expected 2 runs (initial + 1 merged), got %d", c)
	}
}

func TestRunner_Ignore_DropsEventsWhileRunning(t *testing.T) {
	ex := &recordingExecutor{block: make(chan struct{})}
	cfg := (&WatchCfg{
		OnBusy: "ignore",
		Hooks:  []Hook{{Name: "h", Paths: []string{"*"}, Run: "refresh --compile"}},
	}).Defaults()
	r := NewRunner(cfg, "", ex, RealClock{}, func(string, ...interface{}) {})
	defer r.Shutdown(context.Background())

	r.Dispatch(&cfg.Hooks[0], Event{Path: "/x/A.cs", Kind: "write"})
	if !waitUntil(func() bool { return ex.inFlight.Load() == 1 }, 2*time.Second) {
		t.Fatalf("first invocation not in flight")
	}
	// Fire more — all should be dropped.
	for i := 0; i < 10; i++ {
		r.Dispatch(&cfg.Hooks[0], Event{Path: "/x/dropped.cs", Kind: "write"})
	}
	ex.block <- struct{}{}
	time.Sleep(100 * time.Millisecond) // let worker exit
	if c := ex.count(); c != 1 {
		t.Errorf("ignore policy should drop events during run; got %d runs", c)
	}
}

func TestRunner_MaxParallel_LimitsConcurrency(t *testing.T) {
	ex := &recordingExecutor{block: make(chan struct{}, 10)}
	cfg := (&WatchCfg{
		MaxParallel: 2,
		Hooks: []Hook{
			{Name: "h1", Paths: []string{"*"}, Run: "cmd one"},
			{Name: "h2", Paths: []string{"*"}, Run: "cmd two"},
			{Name: "h3", Paths: []string{"*"}, Run: "cmd three"},
			{Name: "h4", Paths: []string{"*"}, Run: "cmd four"},
		},
	}).Defaults()
	r := NewRunner(cfg, "", ex, RealClock{}, func(string, ...interface{}) {})
	defer r.Shutdown(context.Background())

	// Dispatch to all four hooks simultaneously.
	for i := range cfg.Hooks {
		r.Dispatch(&cfg.Hooks[i], Event{Path: "/x/A.cs", Kind: "write"})
	}
	// Wait until semaphore fills to cap.
	if !waitUntil(func() bool { return ex.inFlight.Load() == 2 }, 2*time.Second) {
		t.Fatalf("in-flight didn't reach cap 2; got %d", ex.inFlight.Load())
	}
	// Verify cap held: give scheduling a moment but expect no breakthrough.
	time.Sleep(100 * time.Millisecond)
	if peak := ex.peakInFlight.Load(); peak > 2 {
		t.Errorf("peak in-flight %d exceeds cap 2", peak)
	}
	// Release all.
	for i := 0; i < 4; i++ {
		ex.block <- struct{}{}
	}
	if !waitUntil(func() bool { return ex.count() == 4 }, 2*time.Second) {
		t.Errorf("not all 4 runs completed; got %d", ex.count())
	}
}

func TestRunner_CircuitBreaker_DisablesAfterThreshold(t *testing.T) {
	ex := &recordingExecutor{}
	cfg := (&WatchCfg{
		Hooks: []Hook{{Name: "h", Paths: []string{"*"}, Run: "cmd"}},
	}).Defaults()
	logger, lines := newTestLogger()
	r := NewRunner(cfg, "", ex, RealClock{}, logger)
	defer r.Shutdown(context.Background())

	// 12 single-event dispatches. After 10 fires, breaker trips.
	for i := 0; i < 12; i++ {
		r.Dispatch(&cfg.Hooks[0], Event{Path: "/x/A.cs", Kind: "write"})
		// Let each fire settle.
		waitUntil(func() bool { return ex.count() >= i+1 || ex.count() >= 10 }, 500*time.Millisecond)
	}
	// Expect exactly 10 runs (breaker trips on the 10th fire).
	if c := ex.count(); c > 10 {
		t.Errorf("circuit breaker didn't trip; got %d runs (want ≤10)", c)
	}
	// Verify a DISABLED line was logged.
	found := false
	for _, l := range *lines {
		if contains(l, "DISABLED") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("DISABLED log not emitted; lines=%v", *lines)
	}
}

func TestRunner_PerFileExpansion(t *testing.T) {
	ex := &recordingExecutor{}
	cfg := (&WatchCfg{
		Hooks: []Hook{{Name: "reser", Paths: []string{"*"}, Run: "reserialize $RELFILE"}},
	}).Defaults()
	r := NewRunner(cfg, "/project", ex, RealClock{}, func(string, ...interface{}) {})
	defer r.Shutdown(context.Background())

	r.Dispatch(&cfg.Hooks[0], Event{Path: "/project/Assets/A.prefab", Kind: "write"})
	if !waitUntil(func() bool { return ex.count() >= 1 }, 2*time.Second) {
		t.Fatalf("not invoked")
	}
	runs := ex.snapshot()
	if runs[0].Argv[1] != "Assets/A.prefab" {
		t.Errorf("RELFILE: %v", runs[0].Argv)
	}
}

func TestRunner_HookLevelOnBusy_Override(t *testing.T) {
	ex := &recordingExecutor{block: make(chan struct{})}
	cfg := (&WatchCfg{
		OnBusy: "queue", // global default queue
		Hooks: []Hook{{
			Name:   "h",
			Paths:  []string{"*"},
			Run:    "cmd",
			OnBusy: "ignore", // hook override
		}},
	}).Defaults()
	r := NewRunner(cfg, "", ex, RealClock{}, func(string, ...interface{}) {})
	defer r.Shutdown(context.Background())

	r.Dispatch(&cfg.Hooks[0], Event{Path: "/x/A.cs", Kind: "write"})
	if !waitUntil(func() bool { return ex.inFlight.Load() == 1 }, 2*time.Second) {
		t.Fatalf("not in flight")
	}
	// Fire more — hook-level ignore should drop.
	for i := 0; i < 5; i++ {
		r.Dispatch(&cfg.Hooks[0], Event{Path: "/x/A.cs", Kind: "write"})
	}
	ex.block <- struct{}{}
	time.Sleep(100 * time.Millisecond)
	if c := ex.count(); c != 1 {
		t.Errorf("hook ignore override failed; runs=%d", c)
	}
}

func TestCircuitBreaker_SlidingWindow(t *testing.T) {
	clk := newFakeClock()
	b := newBreaker(clk, 3, time.Second)

	if b.recordFire() {
		t.Errorf("trip on 1st fire")
	}
	if b.recordFire() {
		t.Errorf("trip on 2nd fire")
	}
	// Advance past window — first two fires age out.
	clk.Advance(1100 * time.Millisecond)
	// Next fire should not trip (only 1 in-window).
	if b.recordFire() {
		t.Errorf("trip after aging out old fires")
	}
	// Two more within window trips.
	b.recordFire()
	if !b.recordFire() {
		t.Errorf("3rd in-window fire should trip breaker")
	}
}

// Once tripped, recordFire returns false (don't re-log) and tripped()
// stays true until Reset is called. Reset clears the history, letting
// the breaker be reused — relevant for a future `watch reload`.
func TestCircuitBreaker_Reset(t *testing.T) {
	clk := newFakeClock()
	b := newBreaker(clk, 2, time.Second)

	b.recordFire()
	if !b.recordFire() {
		t.Fatal("2nd fire should trip a threshold=2 breaker")
	}
	if !b.tripped() {
		t.Fatal("breaker should be tripped")
	}
	// Further fires while tripped must not re-trip (returns false).
	if b.recordFire() {
		t.Errorf("recordFire on tripped breaker should return false (already tripped)")
	}

	// Reset brings it back to virgin state.
	b.Reset()
	if b.tripped() {
		t.Error("tripped() still true after Reset")
	}
	// And the breaker is usable again.
	if b.recordFire() {
		t.Error("1st fire after Reset shouldn't trip")
	}
	if !b.recordFire() {
		t.Error("2nd fire after Reset should trip again")
	}
}

// WrapExecError is used by cmd/watch.go to stamp a hook name onto an
// executor error. nil in, nil out; anything else gets wrapped so the
// caller can still errors.Is / errors.Unwrap the original.
func TestWrapExecError(t *testing.T) {
	if err := WrapExecError("compile", nil); err != nil {
		t.Errorf("nil input should return nil, got %v", err)
	}

	orig := errors.New("exit status 1")
	wrapped := WrapExecError("compile", orig)
	if wrapped == nil {
		t.Fatal("non-nil input must produce non-nil error")
	}
	if !strings.Contains(wrapped.Error(), "hook compile:") {
		t.Errorf("message should name the hook; got %q", wrapped.Error())
	}
	if !errors.Is(wrapped, orig) {
		t.Errorf("wrapped error must unwrap to the original (errors.Is)")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && indexOf(s, substr) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
