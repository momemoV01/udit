package watch

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// Watcher wires a filesystem watcher around a root directory. It walks the
// tree once at Start, subscribes fsnotify to every directory, handles
// CREATE-dir events by adding the new dir to the watch set, and emits
// Event values through its Events channel.
//
// It does NOT debounce — that's Debouncer's job. It DOES filter via the
// injected Ignorer so the debouncer never sees Library/ churn.
//
// Single goroutine owns the fsnotify channel; callers read via Events().
// Stop() is idempotent; closing the channel signals the watcher to exit.
type Watcher struct {
	root    string
	watcher *fsnotify.Watcher
	ignore  *Ignorer

	events chan Event
	errs   chan error

	mu   sync.Mutex
	done bool
}

// NewWatcher constructs a watcher rooted at `root` (must be absolute,
// existing directory). `ignore` filters events at fsnotify-event time
// (AND at add-dir time so ignored dirs aren't subscribed in the first
// place — saves fsnotify buffer pressure). `bufferSize` tunes the internal
// kernel-to-userspace buffer on Windows; ignored on Linux/macOS.
func NewWatcher(root string, ignore *Ignorer, bufferSize int) (*Watcher, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve root: %w", err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return nil, fmt.Errorf("stat root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("root is not a directory: %s", absRoot)
	}

	// fsnotify 1.9+ exposes NewBufferedWatcher(uint) which sets the
	// Windows ReadDirectoryChangesW buffer to bufferSize kilobytes (the
	// arg is a slot count). For Unity projects we want a generous buffer
	// (~512 slots, equivalent to bumping from 64KB default to ~512KB).
	slots := uint(bufferSize / 1024)
	if slots == 0 {
		slots = 512
	}
	fw, err := fsnotify.NewBufferedWatcher(slots)
	if err != nil {
		return nil, fmt.Errorf("create fsnotify watcher: %w", err)
	}

	return &Watcher{
		root:    absRoot,
		watcher: fw,
		ignore:  ignore,
		events:  make(chan Event, 256),
		errs:    make(chan error, 16),
	}, nil
}

// Events returns the channel callers read debounced-or-raw events from.
// Closed when Stop completes.
func (w *Watcher) Events() <-chan Event { return w.events }

// Errors exposes fsnotify errors (buffer overflow, add failures). Callers
// should consume non-fatally; most errors log-and-continue.
func (w *Watcher) Errors() <-chan error { return w.errs }

// Start walks the tree once, subscribing every directory (except ignored
// ones), then runs the event loop in the caller's goroutine. Blocks until
// Stop is called or the fsnotify watcher errors fatally. Callers typically
// invoke Start in a dedicated goroutine.
func (w *Watcher) Start() error {
	if err := w.walkAndAdd(w.root); err != nil {
		// Non-fatal: some dirs may be unreadable (Unity's PackageCache can
		// have permission quirks). Log, continue.
		w.sendErr(fmt.Errorf("initial walk: %w", err))
	}

	for {
		select {
		case ev, ok := <-w.watcher.Events:
			if !ok {
				w.closeChannels()
				return nil
			}
			w.handleFSEvent(ev)
		case err, ok := <-w.watcher.Errors:
			if !ok {
				w.closeChannels()
				return nil
			}
			w.sendErr(err)
		}
	}
}

// Stop signals the watcher to exit. Idempotent; safe to call from multiple
// goroutines. After Stop returns the Events channel is closed.
func (w *Watcher) Stop() error {
	w.mu.Lock()
	already := w.done
	w.done = true
	w.mu.Unlock()
	if already {
		return nil
	}
	return w.watcher.Close()
}

func (w *Watcher) closeChannels() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.done {
		w.done = true
	}
	// Close events so downstream consumers (debouncer feeder) know we're done.
	// Errors channel left open — we don't currently use close-as-signal for
	// it; Stop handles the exit.
	close(w.events)
}

// walkAndAdd recursively Adds each directory under root to the fsnotify
// watcher, skipping dirs that the Ignorer matches. Best-effort: per-dir
// errors (permission denied on a subdir) are logged and the walk continues.
func (w *Watcher) walkAndAdd(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Log & continue — one bad subdir shouldn't kill the walk.
			w.sendErr(fmt.Errorf("walk %s: %w", path, err))
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		// Compute project-relative form for ignore matching.
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil
		}
		if w.ignore != nil && w.ignore.Match(rel) {
			return filepath.SkipDir
		}
		if err := w.watcher.Add(path); err != nil {
			w.sendErr(fmt.Errorf("add watch on %s: %w", path, err))
			// don't abort the whole walk on one failed Add
		}
		return nil
	})
}

// handleFSEvent translates an fsnotify.Event into our Event type, applies
// ignore filtering, and forwards to the Events channel. Also extends the
// watch set when a new directory is created under the root.
func (w *Watcher) handleFSEvent(ev fsnotify.Event) {
	abs := filepath.ToSlash(ev.Name)
	rel, err := filepath.Rel(w.root, ev.Name)
	if err == nil {
		rel = filepath.ToSlash(rel)
	} else {
		rel = abs
	}

	// Ignore filtering first — most Library/* events should never escape.
	if w.ignore != nil && w.ignore.Match(rel) {
		return
	}

	kind := mapOp(ev.Op)

	// If this is a CREATE-dir, walk it and subscribe its children too —
	// otherwise `mkdir -p Assets/NewFolder/Scripts` silently misses events
	// inside NewFolder because fsnotify doesn't recurse.
	if ev.Op.Has(fsnotify.Create) {
		if info, statErr := os.Stat(ev.Name); statErr == nil && info.IsDir() {
			if addErr := w.walkAndAdd(ev.Name); addErr != nil {
				w.sendErr(fmt.Errorf("walk new dir %s: %w", ev.Name, addErr))
			}
		}
	}

	// If the path is a directory, we don't want to emit an Event for it —
	// hooks match on files. (CREATE-dir's children will emit their own
	// events shortly; the parent Add covers them.)
	if isDir(ev.Name) {
		return
	}

	select {
	case w.events <- Event{Path: abs, Kind: kind}:
	default:
		// Unbounded event drops are bad — log and drop the oldest to keep
		// liveness. 256-slot channel; full means we're behind. Simpler
		// alternative: block here; we prefer loss-log over pipe stall.
		w.sendErr(fmt.Errorf("event channel full; dropped %s (%s)", abs, kind))
	}
}

// sendErr writes to the errors channel non-blockingly. Full channel ⇒ silent
// drop (callers receive a best-effort diagnostic stream).
func (w *Watcher) sendErr(err error) {
	if err == nil {
		return
	}
	select {
	case w.errs <- err:
	default:
	}
}

// mapOp turns fsnotify's bitmask into our single-string event kind.
// Priority when multiple bits are set: remove > create > write > rename.
func mapOp(op fsnotify.Op) string {
	switch {
	case op.Has(fsnotify.Remove):
		return "remove"
	case op.Has(fsnotify.Create):
		return "create"
	case op.Has(fsnotify.Write):
		return "write"
	case op.Has(fsnotify.Rename):
		return "rename"
	}
	return "write"
}

// isDir reports whether path is a directory. Tolerates missing paths
// (remove events on since-deleted entries) — returns false.
func isDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// DefaultFileExists is a convenience os.Stat wrapper that callers pass to
// NewDebouncer so the sibling-exists check hits real disk.
func DefaultFileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil && !strings.HasSuffix(path, "/")
}
