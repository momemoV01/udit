package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// newSSEServer wraps an httptest.Server so the test can push frames
// on demand and observe client disconnect.
type sseServer struct {
	*httptest.Server
	mu       sync.Mutex
	writer   http.ResponseWriter
	flusher  http.Flusher
	attached chan struct{}
}

func newSSEServer(t *testing.T, contentType string) *sseServer {
	t.Helper()
	s := &sseServer{attached: make(chan struct{}, 1)}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/logs/stream" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(200)
		f, ok := w.(http.Flusher)
		if !ok {
			t.Errorf("response writer does not support Flush")
			return
		}
		f.Flush()
		s.mu.Lock()
		s.writer = w
		s.flusher = f
		s.mu.Unlock()
		s.attached <- struct{}{}
		<-r.Context().Done() // hold connection until client cancels
	})
	s.Server = httptest.NewServer(handler)
	return s
}

func (s *sseServer) push(frame string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.writer == nil {
		return
	}
	_, _ = io.WriteString(s.writer, frame)
	s.flusher.Flush()
}

func (s *sseServer) port() int {
	addr := s.Listener.Addr().(*net.TCPAddr)
	return addr.Port
}

func TestStreamLogs_ParsesFrames(t *testing.T) {
	s := newSSEServer(t, "text/event-stream")
	defer s.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	inst := &Instance{Port: s.port()}
	body, err := StreamLogs(ctx, inst, LogStreamFilter{}, 2*time.Second)
	if err != nil {
		t.Fatalf("StreamLogs: %v", err)
	}
	defer func() { _ = body.Close() }()

	<-s.attached

	// Push two frames.
	s.push(`event: log
data: {"t":1,"type":"Error","message":"oops"}

event: log
data: {"t":2,"type":"Log","message":"hi"}

`)

	received := make([]StreamEvent, 0, 2)
	parseCtx, parseCancel := context.WithCancel(ctx)
	go func() {
		_ = ParseSSEStream(parseCtx, body, func(ev StreamEvent) error {
			received = append(received, ev)
			if len(received) >= 2 {
				parseCancel()
			}
			return nil
		})
	}()

	deadline := time.Now().Add(2 * time.Second)
	for len(received) < 2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if len(received) != 2 {
		t.Fatalf("expected 2 events, got %d", len(received))
	}
	if received[0].Event != "log" {
		t.Errorf("event[0]: %q", received[0].Event)
	}
	var first map[string]interface{}
	if err := json.Unmarshal(received[0].Data, &first); err != nil {
		t.Fatalf("parse data: %v", err)
	}
	if first["message"] != "oops" {
		t.Errorf("message: %v", first["message"])
	}
}

func TestStreamLogs_ConnectorTooOld(t *testing.T) {
	// Server returns 200 OK but Content-Type: application/json →
	// older connector responding "Expected POST /command".
	s := newSSEServer(t, "application/json")
	defer s.Close()

	inst := &Instance{Port: s.port()}
	_, err := StreamLogs(context.Background(), inst, LogStreamFilter{}, 2*time.Second)
	if !errors.Is(err, ErrConnectorTooOld) {
		t.Fatalf("expected ErrConnectorTooOld, got %v", err)
	}
}

func TestStreamLogs_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = io.WriteString(w, `{"error":"bad filter"}`)
	}))
	defer srv.Close()

	port := srv.Listener.Addr().(*net.TCPAddr).Port
	inst := &Instance{Port: port}
	_, err := StreamLogs(context.Background(), inst, LogStreamFilter{}, 2*time.Second)
	if err == nil {
		t.Fatalf("expected error on 400")
	}
	if !strings.Contains(err.Error(), "bad filter") {
		t.Errorf("expected error message to include body: %v", err)
	}
}

func TestParseSSEStream_KeepAliveIgnored(t *testing.T) {
	body := strings.NewReader(`: ping
: ping
event: log
data: {"t":1}

`)
	ctx := context.Background()
	count := 0
	err := ParseSSEStream(ctx, body, func(ev StreamEvent) error {
		count++
		return nil
	})
	// EOF after one frame → ErrStreamInterrupted (caller reconnects).
	if !errors.Is(err, ErrStreamInterrupted) {
		t.Errorf("expected ErrStreamInterrupted on EOF, got %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 event (keep-alives skipped), got %d", count)
	}
}

func TestParseSSEStream_MultilineData(t *testing.T) {
	// Two data: lines merge into one multi-line payload.
	body := strings.NewReader(`event: log
data: line1
data: line2

`)
	count := 0
	var seen string
	_ = ParseSSEStream(context.Background(), body, func(ev StreamEvent) error {
		count++
		seen = string(ev.Data)
		return nil
	})
	if count != 1 {
		t.Fatalf("count: %d", count)
	}
	if seen != "line1\nline2" {
		t.Errorf("multi-line data: %q", seen)
	}
}

func TestParseSSEStream_EmptyEventsIgnored(t *testing.T) {
	// An event with no data: between blank lines should not call onEvent.
	body := strings.NewReader("\n\n: ping\n\n")
	count := 0
	_ = ParseSSEStream(context.Background(), body, func(ev StreamEvent) error {
		count++
		return nil
	})
	if count != 0 {
		t.Errorf("expected zero events, got %d", count)
	}
}

func TestParseSSEStream_CallbackError(t *testing.T) {
	// Propagates the callback's error verbatim.
	body := strings.NewReader(`event: log
data: {"t":1}

event: log
data: {"t":2}

`)
	stop := fmt.Errorf("sentinel")
	called := 0
	err := ParseSSEStream(context.Background(), body, func(ev StreamEvent) error {
		called++
		return stop
	})
	if !errors.Is(err, stop) {
		t.Errorf("expected sentinel, got %v", err)
	}
	if called != 1 {
		t.Errorf("callback should fire once then stop, got %d", called)
	}
}

func TestParseSSEStream_ContextCancellation(t *testing.T) {
	// Stream that blocks forever; context cancel must unblock.
	pr, pw := io.Pipe()
	defer func() { _ = pw.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- ParseSSEStream(ctx, pr, func(StreamEvent) error { return nil })
	}()
	// Push one frame then cancel.
	_, _ = io.WriteString(pw, "event: log\ndata: {}\n\n")
	time.Sleep(50 * time.Millisecond)
	cancel()
	_ = pw.Close()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) && !errors.Is(err, ErrStreamInterrupted) {
			t.Errorf("unexpected err: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ParseSSEStream did not return on cancel")
	}
}
