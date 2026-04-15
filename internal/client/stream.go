package client

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// StreamEvent is one parsed SSE frame from the connector. `Event` is the
// named event type ("log", "reload", "dropped", "truncated"); `Data` is
// the raw JSON payload unparsed so callers can unmarshal into the shape
// appropriate for each event kind.
type StreamEvent struct {
	Event string
	Data  json.RawMessage
}

// LogStreamFilter is the input to StreamLogs. Nil-friendly; each zero
// field means "server default" and is omitted from the query string.
type LogStreamFilter struct {
	// Types restricts the emitted LogType values. Empty slice → all
	// types (connector's default). Accepted: error, warning, log,
	// assert, exception.
	Types []string

	// Stacktrace is one of "none", "user", "full". Empty → server default.
	Stacktrace string

	// SinceMs requests backfill from this many milliseconds before the
	// connection. Zero → live-only, no backfill.
	SinceMs int64
}

// ErrConnectorTooOld is returned by StreamLogs when the connector
// doesn't speak SSE on /logs/stream. Classified as UCI-007 upstream.
var ErrConnectorTooOld = errors.New("connector too old: log tail requires connector >= 0.8.0")

// ErrStreamInterrupted indicates the stream was live, then the
// connection died mid-session. Retryable — classified as UCI-004.
var ErrStreamInterrupted = errors.New("stream interrupted")

// StreamLogs opens an SSE connection to /logs/stream and returns the
// response body so the caller can run a bufio.Scanner loop (or use
// ParseSSEStream). Caller is responsible for closing the body.
//
// Lifecycle:
//   - `ctx` owns the stream. Cancel it to shut down reading.
//   - `connectTimeout` bounds only the initial handshake (up to response
//     headers). Once the body starts streaming there is no idle timeout —
//     keep-alive ping frames ensure intermediate proxies don't prune.
//
// Fails fast (before returning the body) on:
//   - non-2xx status → regular error wrapping the response body
//   - Content-Type not text/event-stream → ErrConnectorTooOld, so the
//     caller can skip the retry loop (version-skew, non-retryable).
func StreamLogs(ctx context.Context, inst *Instance, filter LogStreamFilter, connectTimeout time.Duration) (io.ReadCloser, error) {
	if connectTimeout <= 0 {
		connectTimeout = 5 * time.Second
	}

	q := url.Values{}
	if len(filter.Types) > 0 {
		q.Set("types", strings.Join(filter.Types, ","))
	}
	if filter.Stacktrace != "" {
		q.Set("stacktrace", filter.Stacktrace)
	}
	if filter.SinceMs > 0 {
		q.Set("since_ms", fmt.Sprintf("%d", filter.SinceMs))
	}

	endpoint := fmt.Sprintf("http://127.0.0.1:%d/logs/stream", inst.Port)
	if encoded := q.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot build stream request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	// Client-side signal: disable gzip/keep-alive coalescing so frames
	// arrive promptly (Go's default Transport does not enable gzip for
	// SSE, but the explicit Identity is defensive).
	req.Header.Set("Accept-Encoding", "identity")

	// A dedicated Transport + Client gives us a handshake-only timeout
	// via ResponseHeaderTimeout. Client.Timeout would also cap body
	// reads which is exactly what we don't want for a long-lived stream.
	transport := &http.Transport{
		ResponseHeaderTimeout: connectTimeout,
		// Disable the default idle-connection reuse: each StreamLogs
		// call opens a fresh socket, so a reconnect after domain
		// reload never accidentally lands on a stale keep-alive slot.
		DisableKeepAlives: true,
	}
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   0, // body reads bounded by ctx, not clock.
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		// Separate path: handshake header timeout → stream never opened.
		if errors.Is(err, context.DeadlineExceeded) || isHeaderTimeout(err) {
			return nil, fmt.Errorf("cannot connect to Unity at port %d: connect timeout", inst.Port)
		}
		return nil, fmt.Errorf("cannot connect to Unity at port %d: %w", inst.Port, err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if len(body) > 0 {
			return nil, fmt.Errorf("HTTP %d from stream endpoint: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		return nil, fmt.Errorf("HTTP %d from stream endpoint", resp.StatusCode)
	}

	// Content-Type check — older connectors return JSON here ("Expected
	// POST /command, got GET /logs/stream"). Must not be treated as SSE.
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/event-stream") {
		_ = resp.Body.Close()
		return nil, ErrConnectorTooOld
	}

	return resp.Body, nil
}

// isHeaderTimeout matches the error net/http emits when
// Transport.ResponseHeaderTimeout trips. The error is unexported so we
// string-match; future Go versions may formalize this.
func isHeaderTimeout(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "timeout awaiting response headers") ||
		strings.Contains(msg, "net/http: timeout")
}

// ParseSSEStream reads the response body frame-by-frame and invokes
// `onEvent` for each parsed event. Returns when:
//   - the body reaches EOF (returns ErrStreamInterrupted so callers
//     reconnect),
//   - the context is cancelled (returns ctx.Err()),
//   - onEvent returns a non-nil error (returned as-is).
//
// SSE frame format (what the connector emits):
//
//	event: <name>
//	data: <single-line JSON>
//	<blank line>
//
// Keep-alive comment lines (starting with ":") are skipped silently.
func ParseSSEStream(ctx context.Context, body io.Reader, onEvent func(StreamEvent) error) error {
	scanner := bufio.NewScanner(body)
	// 128KB max — matches plan D5 (16KB message + 16KB stack + overhead).
	scanner.Buffer(make([]byte, 0, 4096), 128*1024)

	var currentEvent string
	var currentData strings.Builder

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := scanner.Text()
		switch {
		case line == "":
			// End of frame.
			if currentData.Len() > 0 {
				data := currentData.String()
				name := currentEvent
				currentEvent = ""
				currentData.Reset()
				if err := onEvent(StreamEvent{Event: name, Data: json.RawMessage(data)}); err != nil {
					return err
				}
			}
		case strings.HasPrefix(line, ":"):
			// Comment / keep-alive; ignore.
		case strings.HasPrefix(line, "event:"):
			currentEvent = strings.TrimSpace(line[len("event:"):])
		case strings.HasPrefix(line, "data:"):
			// Per SSE spec, a single leading space after "data:" is
			// stripped; anything further is preserved.
			val := strings.TrimPrefix(line[len("data:"):], " ")
			if currentData.Len() > 0 {
				currentData.WriteByte('\n')
			}
			currentData.WriteString(val)
		default:
			// Unknown field — SSE spec says ignore.
		}
	}

	if err := scanner.Err(); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return fmt.Errorf("%w: %v", ErrStreamInterrupted, err)
	}
	// Graceful EOF — wrap so callers reconnect uniformly.
	return ErrStreamInterrupted
}
