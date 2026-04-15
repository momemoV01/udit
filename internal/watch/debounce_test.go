package watch

import (
	"runtime"
	"sync"
	"testing"
	"time"
)

// collector is a thread-safe callback target for Debouncer tests.
type collector struct {
	mu   sync.Mutex
	list []Event
}

func (c *collector) add(e Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.list = append(c.list, e)
}

func (c *collector) snapshot() []Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Event, len(c.list))
	copy(out, c.list)
	return out
}

func TestDebouncer_SingleEventFires(t *testing.T) {
	clk := newFakeClock()
	c := &collector{}
	d := NewDebouncer(clk, 300*time.Millisecond, c.add, func(string) bool { return true })

	d.Enqueue(Event{Path: "/a/Foo.cs", Kind: "write"})
	if got := c.snapshot(); len(got) > 0 {
		t.Errorf("callback fired before debounce elapsed: %+v", got)
	}

	clk.Advance(300 * time.Millisecond)
	got := c.snapshot()
	if len(got) != 1 {
		t.Fatalf("expected 1 fire, got %d", len(got))
	}
	if got[0].Path != "/a/Foo.cs" {
		t.Errorf("path: %q", got[0].Path)
	}
}

func TestDebouncer_BurstCollapses(t *testing.T) {
	clk := newFakeClock()
	c := &collector{}
	d := NewDebouncer(clk, 300*time.Millisecond, c.add, func(string) bool { return true })

	// 3 events at 0, 100ms, 200ms — should collapse to 1 fire at 500ms.
	d.Enqueue(Event{Path: "/a/Foo.cs", Kind: "write"})
	clk.Advance(100 * time.Millisecond)
	d.Enqueue(Event{Path: "/a/Foo.cs", Kind: "write"})
	clk.Advance(100 * time.Millisecond)
	d.Enqueue(Event{Path: "/a/Foo.cs", Kind: "write"})

	// At t=200ms, still pending. Advance past the last-event + 300ms window.
	clk.Advance(299 * time.Millisecond)
	if len(c.snapshot()) != 0 {
		t.Errorf("fired early")
	}
	clk.Advance(1 * time.Millisecond)
	if len(c.snapshot()) != 1 {
		t.Errorf("expected 1 fire after debounce settles, got %d", len(c.snapshot()))
	}
}

func TestDebouncer_EventMergePriority(t *testing.T) {
	clk := newFakeClock()
	c := &collector{}
	d := NewDebouncer(clk, 100*time.Millisecond, c.add, func(string) bool { return true })

	// write, then create, then remove — remove wins (priority: remove>create>write).
	d.Enqueue(Event{Path: "/a/Foo.cs", Kind: "write"})
	d.Enqueue(Event{Path: "/a/Foo.cs", Kind: "create"})
	d.Enqueue(Event{Path: "/a/Foo.cs", Kind: "remove"})
	clk.Advance(100 * time.Millisecond)
	got := c.snapshot()
	if len(got) != 1 || got[0].Kind != "remove" {
		t.Errorf("expected remove winner, got %+v", got)
	}
}

func TestDebouncer_MetaCollapse_SiblingPending(t *testing.T) {
	clk := newFakeClock()
	c := &collector{}
	// sibling exists on disk (irrelevant here; sibling is already pending)
	d := NewDebouncer(clk, 100*time.Millisecond, c.add, func(string) bool { return true })

	d.Enqueue(Event{Path: "/a/Foo.cs", Kind: "write"})
	// Now a meta event arrives — should collapse into the pending sibling.
	d.Enqueue(Event{Path: "/a/Foo.cs.meta", Kind: "write"})

	// Only one pending entry expected.
	if p := d.Pending(); p != 1 {
		t.Errorf("pending: %d, want 1", p)
	}
	clk.Advance(100 * time.Millisecond)
	got := c.snapshot()
	if len(got) != 1 {
		t.Errorf("expected 1 fire, got %d: %+v", len(got), got)
	}
	if got[0].Path != "/a/Foo.cs" {
		t.Errorf("path: %q", got[0].Path)
	}
}

func TestDebouncer_MetaCollapse_MetaFirstThenSibling(t *testing.T) {
	// .meta arrives first (realistic when Unity rewrites meta before the
	// asset itself on import), then the real file event follows.
	clk := newFakeClock()
	c := &collector{}
	// sibling exists on disk.
	exists := func(p string) bool { return p == "/a/Foo.cs" }
	d := NewDebouncer(clk, 100*time.Millisecond, c.add, exists)

	d.Enqueue(Event{Path: "/a/Foo.cs.meta", Kind: "write"})
	// Meta upgraded to sibling "write"; one pending.
	if p := d.Pending(); p != 1 {
		t.Fatalf("pending after meta: %d, want 1", p)
	}
	d.Enqueue(Event{Path: "/a/Foo.cs", Kind: "write"})
	if p := d.Pending(); p != 1 {
		t.Fatalf("pending after sibling: %d, want 1 (should dedupe to same key)", p)
	}
	clk.Advance(100 * time.Millisecond)
	got := c.snapshot()
	if len(got) != 1 || got[0].Path != "/a/Foo.cs" {
		t.Errorf("result: %+v", got)
	}
}

func TestDebouncer_MetaOrphan_SignalsRemove(t *testing.T) {
	// Meta event but sibling no longer exists — surface as remove.
	clk := newFakeClock()
	c := &collector{}
	exists := func(p string) bool { return false } // sibling missing
	d := NewDebouncer(clk, 100*time.Millisecond, c.add, exists)

	d.Enqueue(Event{Path: "/a/Foo.cs.meta", Kind: "write"})
	clk.Advance(100 * time.Millisecond)
	got := c.snapshot()
	if len(got) != 1 {
		t.Fatalf("expected 1 fire, got %d", len(got))
	}
	if got[0].Path != "/a/Foo.cs" || got[0].Kind != "remove" {
		t.Errorf("orphan-meta result: %+v (want /a/Foo.cs remove)", got[0])
	}
}

func TestDebouncer_Flush(t *testing.T) {
	clk := newFakeClock()
	c := &collector{}
	d := NewDebouncer(clk, time.Hour, c.add, func(string) bool { return true })

	d.Enqueue(Event{Path: "/a/B.cs", Kind: "write"})
	d.Enqueue(Event{Path: "/a/A.cs", Kind: "write"})
	if got := c.snapshot(); len(got) > 0 {
		t.Errorf("fired early: %+v", got)
	}
	d.Flush()
	got := c.snapshot()
	if len(got) != 2 {
		t.Fatalf("Flush fired %d events, want 2", len(got))
	}
	// Sorted ascending by path
	if got[0].Path != "/a/A.cs" || got[1].Path != "/a/B.cs" {
		t.Errorf("flush order: %+v", got)
	}
	if d.Pending() != 0 {
		t.Errorf("pending after flush: %d, want 0", d.Pending())
	}
}

func TestDebouncer_DifferentPathsIndependent(t *testing.T) {
	clk := newFakeClock()
	c := &collector{}
	d := NewDebouncer(clk, 100*time.Millisecond, c.add, func(string) bool { return true })

	d.Enqueue(Event{Path: "/a/A.cs", Kind: "write"})
	clk.Advance(50 * time.Millisecond)
	d.Enqueue(Event{Path: "/a/B.cs", Kind: "write"})
	clk.Advance(60 * time.Millisecond)
	// A should have fired at t=100 (50+60 past A's enqueue).
	got := c.snapshot()
	if len(got) != 1 || got[0].Path != "/a/A.cs" {
		t.Errorf("A did not fire independently: %+v", got)
	}
	clk.Advance(100 * time.Millisecond)
	// B fires at t=110+100=210; above Advance sets now to 210 total.
	got = c.snapshot()
	if len(got) != 2 {
		t.Fatalf("after B fires: %d events, want 2", len(got))
	}
}

func TestDebouncer_BackslashNormalized(t *testing.T) {
	// Windows-only: filepath.ToSlash rewrites backslashes only on Windows.
	// On Unix, backslash is a valid filename character and must be
	// preserved exactly.
	if runtime.GOOS != "windows" {
		t.Skip("windows-only: backslash is a valid Unix filename character")
	}
	clk := newFakeClock()
	c := &collector{}
	d := NewDebouncer(clk, 100*time.Millisecond, c.add, func(string) bool { return true })

	d.Enqueue(Event{Path: `C:\proj\Assets\Foo.cs`, Kind: "write"})
	clk.Advance(100 * time.Millisecond)
	got := c.snapshot()
	if len(got) != 1 {
		t.Fatalf("got %d events", len(got))
	}
	if got[0].Path != "C:/proj/Assets/Foo.cs" {
		t.Errorf("backslash not normalized: %q", got[0].Path)
	}
}
