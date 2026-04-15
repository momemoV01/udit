package watch

import (
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Debouncer collapses bursts of file-system events into single logical
// change notifications per file. Events for the same path arriving within
// the debounce window reset the timer — callback fires once, `delta` after
// the last event in the burst.
//
// Also performs Unity .meta sibling collapse: a `.meta` event is silently
// dropped when its sibling non-.meta file has a pending timer. An orphan
// .meta (no sibling on disk AND no pending sibling timer) is forwarded as
// the sibling path with event=remove — Unity writes orphan metas when it
// processes a deletion, so this signals the deletion of the real asset.
//
// Concurrency: Enqueue() can be called from any goroutine (the fsnotify
// consumer). Callback fires on the timer goroutine; it must not block for
// long or subsequent callbacks will queue behind.
type Debouncer struct {
	mu         sync.Mutex
	clock      Clock
	delta      time.Duration
	pending    map[string]*pendingEntry
	callback   func(Event)
	fileExists func(string) bool // injectable for tests; defaults to os.Stat
}

type pendingEntry struct {
	timer Timer
	event Event // latest event for the path; fed to the callback on fire
}

// NewDebouncer constructs a debouncer. `delta` is the quiet period; events
// for the same path arriving inside it reset the timer. `callback` is
// invoked once per file after debounce settles.
//
// fileExists is optional — nil ⇒ use filepath-based stat via os.Stat.
// Tests inject a map-backed fake to avoid touching real disk.
func NewDebouncer(clock Clock, delta time.Duration, callback func(Event), fileExists func(string) bool) *Debouncer {
	return &Debouncer{
		clock:      clock,
		delta:      delta,
		pending:    map[string]*pendingEntry{},
		callback:   callback,
		fileExists: fileExists,
	}
}

// Enqueue receives a raw event from the watcher and either schedules /
// resets a debounce timer, or collapses it against an existing entry (for
// .meta siblings).
//
// Contract: ev.Path is absolute forward-slash; ev.Kind is create/write/
// remove/rename.
func (d *Debouncer) Enqueue(ev Event) {
	ev.Path = filepath.ToSlash(ev.Path)

	// .meta collapse. If sibling is pending, drop the meta event entirely;
	// sibling's timer already covers both changes Unity will make.
	if strings.HasSuffix(ev.Path, ".meta") {
		sibling := strings.TrimSuffix(ev.Path, ".meta")
		d.mu.Lock()
		if _, ok := d.pending[sibling]; ok {
			d.mu.Unlock()
			return // sibling's debounce covers us
		}
		d.mu.Unlock()
		// No sibling pending. Check disk — if sibling still exists, this
		// is just a metadata change Unity made for reasons unrelated to
		// the asset (e.g. labels). Forward as a write on the sibling so
		// hooks can react to the asset, not the meta.
		if d.sibExists(sibling) {
			ev = Event{Path: sibling, Kind: "write"}
		} else {
			// Orphan meta — Unity typically writes these right after an
			// asset is deleted, OR rewrites them before the actual delete
			// hits disk. Forward as a remove signal on the sibling.
			ev = Event{Path: sibling, Kind: "remove"}
		}
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if entry, ok := d.pending[ev.Path]; ok {
		entry.event = mergeEvent(entry.event, ev)
		entry.timer.Reset(d.delta)
		return
	}

	path := ev.Path
	// Capture path by value for the closure.
	d.pending[path] = &pendingEntry{
		event: ev,
		timer: d.clock.AfterFunc(d.delta, func() { d.fire(path) }),
	}
}

// fire is the timer callback. Removes the pending entry and invokes the
// user callback with the final collapsed event.
func (d *Debouncer) fire(path string) {
	d.mu.Lock()
	entry, ok := d.pending[path]
	delete(d.pending, path)
	d.mu.Unlock()
	if !ok || d.callback == nil {
		return
	}
	d.callback(entry.event)
}

// Flush synchronously drains pending entries, firing each callback
// immediately in path-sorted order. Called on graceful shutdown so no
// in-flight debounces are lost.
func (d *Debouncer) Flush() {
	d.mu.Lock()
	entries := make([]Event, 0, len(d.pending))
	for p, e := range d.pending {
		e.timer.Stop()
		entries = append(entries, e.event)
		delete(d.pending, p)
	}
	d.mu.Unlock()

	// Stable-ish order for tests: sort by path.
	sortEvents(entries)
	for _, e := range entries {
		if d.callback != nil {
			d.callback(e)
		}
	}
}

// Pending returns the count of not-yet-fired debounced events. Used in
// tests and for user-facing diagnostics.
func (d *Debouncer) Pending() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.pending)
}

// sibExists is the indirection for stat; tests override via the constructor.
func (d *Debouncer) sibExists(path string) bool {
	if d.fileExists != nil {
		return d.fileExists(path)
	}
	// Default: cheap stat. Not using os.Stat to keep this file focused.
	// The watcher.go that wires the real thing installs a closure using
	// os.Stat. When nil (e.g. tests that haven't set one), be conservative
	// and report "exists" so we don't spuriously fire remove signals.
	return true
}

// mergeEvent combines two events on the same path. Create wins over write;
// remove wins over create (a create+quick-remove still surfaces as remove).
// Rename is treated the same as write for this merge — watch cares about
// the file's current identity, not its history.
func mergeEvent(prev, next Event) Event {
	prio := map[string]int{"remove": 4, "create": 3, "write": 2, "rename": 1}
	if prio[next.Kind] >= prio[prev.Kind] {
		return Event{Path: next.Path, Kind: next.Kind}
	}
	return prev
}

// sortEvents orders events by path ascending; deterministic for tests.
func sortEvents(ev []Event) {
	// insertion sort: caller sets usually tiny (<100).
	for i := 1; i < len(ev); i++ {
		for j := i; j > 0 && ev[j].Path < ev[j-1].Path; j-- {
			ev[j], ev[j-1] = ev[j-1], ev[j]
		}
	}
}
