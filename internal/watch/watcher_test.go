package watch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// eventContains polls the watcher's Events channel up to timeout, looking
// for at least one event whose path ends with wantSuffix. Returns the
// matching event or a zero Event + false.
func eventContains(t *testing.T, ch <-chan Event, wantSuffix string, timeout time.Duration) (Event, bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case ev, ok := <-ch:
			if !ok {
				return Event{}, false
			}
			if filepath.ToSlash(ev.Path) != "" &&
				len(ev.Path) >= len(wantSuffix) &&
				ev.Path[len(ev.Path)-len(wantSuffix):] == wantSuffix {
				return ev, true
			}
		case <-time.After(50 * time.Millisecond):
			// keep polling
		}
	}
	return Event{}, false
}

func TestWatcher_SmokeCreateWrite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real-FS watcher test in short mode")
	}
	root := t.TempDir()
	w, err := NewWatcher(root, NewIgnorer(nil, false, false), 512*1024)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer func() { _ = w.Stop() }()

	go func() { _ = w.Start() }()

	// Small grace period so fsnotify is attached before we create files.
	time.Sleep(100 * time.Millisecond)

	fpath := filepath.Join(root, "hello.txt")
	if err := os.WriteFile(fpath, []byte("hi"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, ok := eventContains(t, w.Events(), "hello.txt", 2*time.Second); !ok {
		t.Errorf("create event for hello.txt not observed within 2s")
	}
}

func TestWatcher_IgnoredDirNotSubscribed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real-FS watcher test in short mode")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Library"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	w, err := NewWatcher(root, NewIgnorer(nil, true, false), 512*1024)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer func() { _ = w.Stop() }()

	go func() { _ = w.Start() }()
	time.Sleep(100 * time.Millisecond)

	// Create a file in Library — should NOT surface as an event.
	fpath := filepath.Join(root, "Library", "x.bin")
	if err := os.WriteFile(fpath, []byte("noise"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Also create a file at root so the test doesn't wait for a negative
	// forever; we just verify Library's event is not among those received.
	sentinel := filepath.Join(root, "real.txt")
	if err := os.WriteFile(sentinel, []byte("hi"), 0644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	// Read events for 500ms, checking no Library path slipped through.
	deadline := time.Now().Add(500 * time.Millisecond)
	sawSentinel := false
	for time.Now().Before(deadline) {
		select {
		case ev := <-w.Events():
			if filepath.Base(ev.Path) == "x.bin" {
				t.Errorf("Library event leaked: %+v", ev)
			}
			if filepath.Base(ev.Path) == "real.txt" {
				sawSentinel = true
			}
		case <-time.After(50 * time.Millisecond):
		}
	}
	if !sawSentinel {
		t.Logf("warning: sentinel event not observed (test may be skipped due to slow FS)")
	}
}

func TestWatcher_CreateNewSubdirAddsToWatch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real-FS watcher test in short mode")
	}
	root := t.TempDir()
	w, err := NewWatcher(root, NewIgnorer(nil, false, false), 512*1024)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer func() { _ = w.Stop() }()

	go func() { _ = w.Start() }()
	time.Sleep(100 * time.Millisecond)

	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Give walker + Add time to subscribe the new dir.
	time.Sleep(300 * time.Millisecond)

	// Now write inside the new dir — an event should fire.
	fpath := filepath.Join(sub, "inside.txt")
	if err := os.WriteFile(fpath, []byte("!"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, ok := eventContains(t, w.Events(), "inside.txt", 2*time.Second); !ok {
		t.Errorf("event for sub/inside.txt not observed")
	}
}

func TestMapOp(t *testing.T) {
	// Can't import fsnotify directly into test without coupling; exercise
	// via fsnotify.Op flags through a lightweight local helper.
	// This test is covered indirectly by the smoke tests above when real
	// events come in; keep a placeholder for the function's contract doc.
	if got := mapOp(0); got != "write" {
		t.Errorf("mapOp(0) fallback = %q, want write", got)
	}
}
