package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/momemoV01/udit/internal/client"
)

// ----------------------------------------------------------------------
// reconnectBackoff
// ----------------------------------------------------------------------

func TestReconnectBackoff_Sequence(t *testing.T) {
	b := newReconnectBackoff()
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 4 * time.Second}
	for i, w := range want {
		got := b.next()
		if got != w {
			t.Errorf("next() #%d = %s, want %s", i+1, got, w)
		}
	}
}

func TestReconnectBackoff_Reset(t *testing.T) {
	b := newReconnectBackoff()
	// Burn through two advancements.
	_ = b.next()
	_ = b.next()
	// reset() brings the next value back to 1s.
	b.reset()
	if got := b.next(); got != time.Second {
		t.Errorf("after reset, next() = %s, want 1s", got)
	}
}

// ----------------------------------------------------------------------
// logTypeTag / colorFor
// ----------------------------------------------------------------------

func TestLogTypeTag(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Error", "[E]"},
		{"Exception", "[E]"},
		{"Assert", "[E]"},
		{"Warning", "[W]"},
		{"Log", "[L]"},
		{"Info", "[Info]"}, // unknown — passthrough
		{"", "[]"},
	}
	for _, c := range cases {
		if got := logTypeTag(c.in); got != c.want {
			t.Errorf("logTypeTag(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestColorFor(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Error", ansiRed},
		{"Exception", ansiRed},
		{"Assert", ansiRed},
		{"Warning", ansiYellow},
		{"Log", ansiDim},
		{"Unknown", ""},
	}
	for _, c := range cases {
		if got := colorFor(c.in); got != c.want {
			t.Errorf("colorFor(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ----------------------------------------------------------------------
// sleepCtx
// ----------------------------------------------------------------------

func TestSleepCtx_Elapses(t *testing.T) {
	ctx := context.Background()
	start := time.Now()
	if ok := sleepCtx(ctx, 50*time.Millisecond); !ok {
		t.Fatal("expected ok=true when timer elapses")
	}
	if elapsed := time.Since(start); elapsed < 40*time.Millisecond {
		t.Errorf("returned too early: %s", elapsed)
	}
}

func TestSleepCtx_CancelWins(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	start := time.Now()
	if ok := sleepCtx(ctx, 10*time.Second); ok {
		t.Fatal("expected ok=false when ctx cancelled")
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("cancel didn't preempt timer: %s", elapsed)
	}
}

// ----------------------------------------------------------------------
// logFormatter helpers — JSON and text paths
// ----------------------------------------------------------------------

// newTestFormatter builds a logFormatter with byte-buffer sinks so tests
// can inspect exactly what the production code would have written. It
// bypasses newLogFormatter's TTY detection (always useAnsi=false).
func newTestFormatter(useJSON bool, rx *regexp.Regexp) (*logFormatter, *bytes.Buffer, *bytes.Buffer) {
	out := &bytes.Buffer{}
	errw := &bytes.Buffer{}
	f := &logFormatter{
		useJSON: useJSON,
		verbose: false,
		useAnsi: false,
		rx:      rx,
		out:     out,
		errw:    errw,
	}
	return f, out, errw
}

// unmarshalFirstLine pops the first NDJSON line off a buffer and returns
// it as a map. Fails the test on malformed input.
func unmarshalFirstLine(t *testing.T, b *bytes.Buffer) map[string]interface{} {
	t.Helper()
	line, err := b.ReadBytes('\n')
	if err != nil {
		t.Fatalf("ReadBytes: %v (buffer=%q)", err, b.String())
	}
	var m map[string]interface{}
	if err := json.Unmarshal(line, &m); err != nil {
		t.Fatalf("Unmarshal %q: %v", line, err)
	}
	return m
}

func TestEmitLogJSON_FullFields(t *testing.T) {
	f, out, _ := newTestFormatter(true, nil)
	frame := logFrame{
		T:       1700000000000,
		Type:    "Error",
		Message: "boom",
		Stack:   "at Foo()",
		File:    "Assets/Foo.cs",
		Line:    42,
	}
	if err := f.emitLogJSON(frame); err != nil {
		t.Fatalf("emitLogJSON: %v", err)
	}
	m := unmarshalFirstLine(t, out)
	if m["kind"] != "log" {
		t.Errorf("kind = %v", m["kind"])
	}
	if m["level"] != "error" {
		t.Errorf("level = %v (expected lowercased)", m["level"])
	}
	if m["message"] != "boom" {
		t.Errorf("message = %v", m["message"])
	}
	if m["stack"] != "at Foo()" {
		t.Errorf("stack = %v", m["stack"])
	}
	if m["file"] != "Assets/Foo.cs" {
		t.Errorf("file = %v", m["file"])
	}
	// JSON numbers decode as float64.
	if ln, ok := m["line"].(float64); !ok || ln != 42 {
		t.Errorf("line = %v", m["line"])
	}
	// "t" should be RFC3339Nano in UTC.
	ts, _ := m["t"].(string)
	if !strings.HasSuffix(ts, "Z") {
		t.Errorf("timestamp not UTC: %q", ts)
	}
	if _, err := time.Parse(time.RFC3339Nano, ts); err != nil {
		t.Errorf("bad timestamp %q: %v", ts, err)
	}
}

func TestEmitLogJSON_OmitsEmptyOptionals(t *testing.T) {
	f, out, _ := newTestFormatter(true, nil)
	frame := logFrame{T: 1, Type: "Log", Message: "hi"}
	if err := f.emitLogJSON(frame); err != nil {
		t.Fatalf("emitLogJSON: %v", err)
	}
	m := unmarshalFirstLine(t, out)
	for _, k := range []string{"stack", "file", "line"} {
		if _, has := m[k]; has {
			t.Errorf("unexpected key %q in output: %v", k, m)
		}
	}
}

func TestEmitLogText_WithStack(t *testing.T) {
	f, out, _ := newTestFormatter(false, nil)
	frame := logFrame{
		T:       time.Date(2025, 6, 1, 12, 34, 56, 0, time.UTC).UnixMilli(),
		Type:    "Warning",
		Message: "careful",
		Stack:   "line1\n\nline2", // blank line between — must be skipped
	}
	if err := f.emitLogText(frame); err != nil {
		t.Fatalf("emitLogText: %v", err)
	}
	got := out.String()
	// First line = "<HH:MM:SS> [W] careful"
	if !strings.Contains(got, "[W] careful") {
		t.Errorf("missing [W] prefix: %q", got)
	}
	if !strings.Contains(got, "  line1") || !strings.Contains(got, "  line2") {
		t.Errorf("stack not indented: %q", got)
	}
	if strings.Contains(got, "  \n") {
		t.Errorf("blank stack line was emitted: %q", got)
	}
}

func TestEmitLog_FilterDropsNonMatching(t *testing.T) {
	rx := regexp.MustCompile(`NullReference`)
	f, out, _ := newTestFormatter(true, rx)
	// Message that does NOT match — must be dropped.
	frame := logFrame{T: 1, Type: "Error", Message: "generic oops"}
	payload, _ := json.Marshal(frame)
	if err := f.emitLog(payload); err != nil {
		t.Fatalf("emitLog: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("expected drop, got: %q", out.String())
	}

	// Message that DOES match — should emit.
	frame.Message = "NullReferenceException in Foo"
	payload, _ = json.Marshal(frame)
	if err := f.emitLog(payload); err != nil {
		t.Fatalf("emitLog (matching): %v", err)
	}
	if out.Len() == 0 {
		t.Fatalf("expected emission, got empty buffer")
	}
	m := unmarshalFirstLine(t, out)
	if m["message"] != "NullReferenceException in Foo" {
		t.Errorf("message = %v", m["message"])
	}
}

func TestEmitLog_MalformedFrameIsDroppedNotFatal(t *testing.T) {
	f, out, _ := newTestFormatter(true, nil)
	if err := f.emitLog(json.RawMessage(`{not json}`)); err != nil {
		t.Errorf("malformed frame should NOT return error; got %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("malformed frame should emit nothing, got %q", out.String())
	}
}

// ----------------------------------------------------------------------
// Marker events + notice + reconnect
// ----------------------------------------------------------------------

func TestEmitMarker_JSON(t *testing.T) {
	f, out, _ := newTestFormatter(true, nil)
	if err := f.emitMarker("reload", json.RawMessage(`{"reason":"domain reload"}`)); err != nil {
		t.Fatalf("emitMarker: %v", err)
	}
	m := unmarshalFirstLine(t, out)
	if m["kind"] != "reload" {
		t.Errorf("kind = %v", m["kind"])
	}
	if m["reason"] != "domain reload" {
		t.Errorf("reason = %v", m["reason"])
	}
}

func TestEmitMarker_TextGoesToStderr(t *testing.T) {
	f, out, errw := newTestFormatter(false, nil)
	if err := f.emitMarker("dropped", json.RawMessage(`{"n":3}`)); err != nil {
		t.Fatalf("emitMarker: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("marker leaked to stdout: %q", out.String())
	}
	got := errw.String()
	if !strings.Contains(got, "[log] dropped") {
		t.Errorf("stderr = %q", got)
	}
}

func TestReconnect_JSONAndText(t *testing.T) {
	// JSON path
	f, out, _ := newTestFormatter(true, nil)
	f.reconnect(2*time.Second, "stream interrupted")
	m := unmarshalFirstLine(t, out)
	if m["kind"] != "reconnect" || m["reason"] != "stream interrupted" {
		t.Errorf("json = %v", m)
	}
	if ms, ok := m["in_ms"].(float64); !ok || ms != 2000 {
		t.Errorf("in_ms = %v", m["in_ms"])
	}

	// Text path
	f2, out2, errw2 := newTestFormatter(false, nil)
	f2.reconnect(500*time.Millisecond, "bad")
	if out2.Len() != 0 {
		t.Errorf("reconnect leaked to stdout: %q", out2.String())
	}
	got := errw2.String()
	if !strings.Contains(got, "disconnected (bad)") {
		t.Errorf("stderr = %q", got)
	}
}

func TestNotice_JSONAndText(t *testing.T) {
	f, out, _ := newTestFormatter(true, nil)
	f.notice("hello")
	m := unmarshalFirstLine(t, out)
	if m["kind"] != "notice" || m["message"] != "hello" {
		t.Errorf("json = %v", m)
	}

	f2, out2, errw2 := newTestFormatter(false, nil)
	f2.notice("world")
	if out2.Len() != 0 {
		t.Errorf("notice leaked to stdout: %q", out2.String())
	}
	if !strings.Contains(errw2.String(), "[log] world") {
		t.Errorf("stderr = %q", errw2.String())
	}
}

// ----------------------------------------------------------------------
// emit() dispatch table
// ----------------------------------------------------------------------

func TestEmit_DispatchesByEventName(t *testing.T) {
	logPayload, _ := json.Marshal(logFrame{T: 1, Type: "Log", Message: "ok"})

	cases := []struct {
		name    string
		ev      client.StreamEvent
		wantOut bool // JSON mode: stdout should receive a line
	}{
		{"log event", client.StreamEvent{Event: "log", Data: logPayload}, true},
		{"reload marker", client.StreamEvent{Event: "reload", Data: json.RawMessage(`{}`)}, true},
		{"dropped marker", client.StreamEvent{Event: "dropped", Data: json.RawMessage(`{}`)}, true},
		{"truncated marker", client.StreamEvent{Event: "truncated", Data: json.RawMessage(`{}`)}, true},
		{"unknown event is silently ignored", client.StreamEvent{Event: "mystery", Data: json.RawMessage(`{}`)}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, out, _ := newTestFormatter(true, nil)
			if err := f.emit(c.ev); err != nil {
				t.Fatalf("emit: %v", err)
			}
			if got := out.Len() > 0; got != c.wantOut {
				t.Errorf("stdout-written=%v, want %v (buf=%q)", got, c.wantOut, out.String())
			}
		})
	}
}
