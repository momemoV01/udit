package watch

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Executor is the side-effectful boundary the runner relies on. The real
// implementation (in cmd/watch.go) forks exec.Command("udit", argv...). The
// test implementation records calls for assertion.
type Executor interface {
	// Run executes argv with the supplied additional env entries merged
	// on top of the parent env. Returns the child's exit code (0 on
	// success, non-zero on any failure the child reported). err is set
	// for *infrastructure* failures (couldn't spawn, lost stdio) — not
	// for child exit != 0; those come back as (code, nil).
	Run(ctx context.Context, argv []string, env []string) (exitCode int, err error)
}

// Logger is the minimal logging hook the runner calls at notable state
// transitions (dispatch start, exit, circuit-breaker trip, drop-on-ignore).
// Kept tiny — cmd/watch.go provides the concrete formatting.
type Logger func(format string, args ...interface{})

// Runner owns the per-hook concurrency policy — one hookRunner per Hook,
// each with its own mutex and worker goroutine. The runner is safe for
// concurrent Dispatch calls from the event producer.
type Runner struct {
	cfg         WatchCfg
	executor    Executor
	clock       Clock
	logger      Logger
	semCap      chan struct{} // global max_parallel semaphore
	projectRoot string

	mu      sync.Mutex
	hooks   map[string]*hookRunner
	closing bool
	wg      sync.WaitGroup
}

// NewRunner constructs a Runner from a validated WatchCfg. Call Defaults()
// on the cfg first if it came straight from YAML so zero fields are filled.
// `projectRoot` is used for $RELFILE expansion; empty falls back to abs paths.
func NewRunner(cfg WatchCfg, projectRoot string, executor Executor, clock Clock, logger Logger) *Runner {
	if executor == nil {
		panic("watch.NewRunner: executor is required")
	}
	if clock == nil {
		clock = RealClock{}
	}
	if logger == nil {
		logger = func(string, ...interface{}) {}
	}
	cap := cfg.MaxParallel
	if cap <= 0 {
		cap = 4
	}
	sem := make(chan struct{}, cap)

	return &Runner{
		cfg:         cfg,
		executor:    executor,
		clock:       clock,
		logger:      logger,
		semCap:      sem,
		projectRoot: projectRoot,
		hooks:       map[string]*hookRunner{},
	}
}

// Dispatch enqueues one event for hook h. If h has no active runner yet,
// one is created and its worker goroutine spawned. Otherwise the event is
// added to the pending set (deduped by path); the worker picks it up when
// the current run exits.
//
// Non-blocking — the caller (event producer) never waits on a hook.
func (r *Runner) Dispatch(h *Hook, ev Event) {
	if h == nil {
		return
	}
	r.mu.Lock()
	if r.closing {
		r.mu.Unlock()
		return
	}
	hr, ok := r.hooks[h.Name]
	if !ok {
		hr = &hookRunner{
			runner:  r,
			hook:    *h,
			pending: map[string]Event{},
			breaker: newBreaker(r.clock, 10, 10*time.Second),
		}
		// Apply hook-level override if present.
		hr.onBusy = h.OnBusy
		if hr.onBusy == "" {
			hr.onBusy = r.cfg.OnBusy
		}
		if hr.onBusy == "" {
			hr.onBusy = "queue"
		}
		r.hooks[h.Name] = hr
	}
	r.mu.Unlock()

	hr.enqueue(ev)
}

// Shutdown waits for in-flight hook runs to finish. Safe to call from a
// different goroutine than Dispatch. After Shutdown returns, new Dispatch
// calls are silently dropped.
func (r *Runner) Shutdown(ctx context.Context) {
	r.mu.Lock()
	r.closing = true
	r.mu.Unlock()

	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		// Caller gave up waiting. We return immediately; running goroutines
		// will exit on their own. Caller logs the timeout.
	}
}

// hookRunner serializes execution for one Hook. Single worker goroutine
// drains the pending set; Dispatch is non-blocking.
type hookRunner struct {
	runner  *Runner
	hook    Hook
	onBusy  string
	breaker *circuitBreaker

	mu      sync.Mutex
	pending map[string]Event // key=path
	running bool
}

func (hr *hookRunner) enqueue(ev Event) {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	if hr.breaker.tripped() {
		return
	}

	if hr.running && hr.onBusy == "ignore" {
		hr.runner.logger("[watch:%s] ignored (busy): %s", hr.hook.Name, ev.Path)
		return
	}

	// Dedupe by path: last event for the same path wins.
	hr.pending[ev.Path] = ev

	if !hr.running {
		hr.running = true
		hr.runner.wg.Add(1)
		go hr.runLoop()
	}
}

// runLoop is the worker body. Drains pending in a loop — each iteration
// pulls the current pending set, clears it, and dispatches. Exits when
// pending is empty after a completed run.
func (hr *hookRunner) runLoop() {
	defer hr.runner.wg.Done()
	for {
		hr.mu.Lock()
		if len(hr.pending) == 0 {
			hr.running = false
			hr.mu.Unlock()
			return
		}
		// Steal pending.
		events := make([]Event, 0, len(hr.pending))
		for _, e := range hr.pending {
			events = append(events, e)
		}
		hr.pending = map[string]Event{}
		hr.mu.Unlock()

		// Sort for determinism.
		sortEvents(events)

		// Circuit-breaker bookkeeping: count *each* batch as one fire —
		// the intent is to detect self-trigger loops, not path storms.
		if hr.breaker.recordFire() {
			hr.runner.logger("[watch:%s] DISABLED — %d fires in %s; likely self-trigger loop, check ignore patterns",
				hr.hook.Name, hr.breaker.threshold, hr.breaker.window)
			// Clear pending; new enqueues short-circuit on breaker.tripped().
			hr.mu.Lock()
			hr.pending = map[string]Event{}
			hr.running = false
			hr.mu.Unlock()
			return
		}

		hr.dispatchBatch(events)
	}
}

// dispatchBatch expands and executes the hook for one collapsed set of
// events. Each Command (per-file or single) goes through the global
// max_parallel semaphore before reaching the executor.
func (hr *hookRunner) dispatchBatch(events []Event) {
	batch := Batch{
		HookName:    hr.hook.Name,
		Files:       events,
		ProjectRoot: hr.runner.projectRoot,
	}
	spec, err := Expand(hr.hook.Run, batch)
	if err != nil {
		hr.runner.logger("[watch:%s] expand error: %v", hr.hook.Name, err)
		return
	}

	for _, cmd := range spec.Commands {
		// Acquire a semaphore slot. Blocks when max_parallel is saturated.
		hr.runner.semCap <- struct{}{}
		started := hr.runner.clock.Now()
		hr.runner.logger("[watch:%s] %s %s → %s", hr.hook.Name,
			dominantEvent(events), firstPath(events), joinArgs(cmd.Argv))

		ctx := context.Background() // MVP: no per-hook timeout
		code, err := hr.runner.executor.Run(ctx, cmd.Argv, cmd.Env)

		<-hr.runner.semCap
		elapsed := hr.runner.clock.Now().Sub(started)
		if err != nil {
			hr.runner.logger("[watch:%s] run error after %s: %v", hr.hook.Name, elapsed, err)
			continue
		}
		if code != 0 {
			hr.runner.logger("[watch:%s] exit=%d (%s) — non-zero, continuing", hr.hook.Name, code, elapsed)
			continue
		}
		hr.runner.logger("[watch:%s] exit=0 (%s)", hr.hook.Name, elapsed)
	}
}

// firstPath returns the lexicographically-first path in the batch (or
// empty). Used for human-readable log prefix.
func firstPath(events []Event) string {
	if len(events) == 0 {
		return ""
	}
	return events[0].Path
}

// joinArgs is a lightweight fmt helper for logging argv.
func joinArgs(argv []string) string {
	s := ""
	for i, a := range argv {
		if i > 0 {
			s += " "
		}
		s += a
	}
	return s
}

// circuitBreaker protects against infinite self-trigger loops. If the hook
// fires more than `threshold` times inside a sliding `window`, the breaker
// trips and the hook is disabled until the process restarts.
type circuitBreaker struct {
	mu        sync.Mutex
	clock     Clock
	fires     []time.Time
	threshold int
	window    time.Duration
	tripFlag  bool
}

func newBreaker(clock Clock, threshold int, window time.Duration) *circuitBreaker {
	return &circuitBreaker{clock: clock, threshold: threshold, window: window}
}

// recordFire appends a fire timestamp, prunes stale ones, and returns true
// when the threshold is exceeded (meaning the breaker just tripped).
func (b *circuitBreaker) recordFire() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.tripFlag {
		return false // already tripped — don't re-log
	}
	now := b.clock.Now()
	cutoff := now.Add(-b.window)
	// Drop stale entries.
	keep := b.fires[:0]
	for _, t := range b.fires {
		if !t.Before(cutoff) {
			keep = append(keep, t)
		}
	}
	b.fires = append(keep, now)
	if len(b.fires) >= b.threshold {
		b.tripFlag = true
		return true
	}
	return false
}

func (b *circuitBreaker) tripped() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.tripFlag
}

// Reset is currently unused — breaker only resets on process restart per
// design D4b. Exposed for potential future "watch reload" command.
func (b *circuitBreaker) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.fires = nil
	b.tripFlag = false
}

// WrapExecError wraps an executor error with hook context for logging.
// Used by the CLI when surfacing exec.Command failures. Kept in this file
// to avoid a circular dep from cmd/watch.go.
func WrapExecError(hookName string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("hook %s: %w", hookName, err)
}
