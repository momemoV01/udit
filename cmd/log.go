package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/momemoV01/udit/internal/client"
	"golang.org/x/term"
)

// logCmd dispatches `udit log <subcommand>`. Subcommands:
//
//	tail  — live SSE stream of Unity console messages
//	list  — snapshot alias; same Connector tool as `udit console`
//
// Keeping `udit console` working as a back-compat synonym is handled in
// cmd/root.go; this file only speaks in terms of the `log` namespace.
func logCmd(subArgs []string, globalJSON bool) error {
	if len(subArgs) == 0 {
		fmt.Fprint(os.Stderr, logHelp())
		return nil
	}
	sub := subArgs[0]
	rest := subArgs[1:]
	switch sub {
	case "tail":
		return logTailCmd(rest, globalJSON)
	case "list":
		return logListCmd(rest, globalJSON)
	case "-h", "--help", "help":
		fmt.Fprint(os.Stderr, logHelp())
		return nil
	default:
		return fmt.Errorf("unknown log subcommand %q (expected: tail, list)", sub)
	}
}

// logTailCmd implements `udit log tail` — long-lived SSE stream with
// reconnect loop, signal handling, color-coded output.
func logTailCmd(subArgs []string, globalJSON bool) error {
	fs := flag.NewFlagSet("log tail", flag.ContinueOnError)
	var (
		typesCsv    string
		stacktrace  string
		sinceStr    string
		filterRegex string
		localJSON   bool
		verbose     bool
		noColor     bool
	)
	fs.StringVar(&typesCsv, "type", "", "Comma-separated log types (error,warning,log,assert,exception). Default: all.")
	fs.StringVar(&stacktrace, "stacktrace", "", "Stack trace detail: none | user | full (default: user).")
	fs.StringVar(&sinceStr, "since", "", "Emit buffered history from the last N (e.g. 5m, 30s) before going live.")
	fs.StringVar(&filterRegex, "filter", "", "Client-side regex; drop events whose message doesn't match.")
	fs.BoolVar(&localJSON, "json", false, "Emit NDJSON on stdout (OR with global --json).")
	fs.BoolVar(&verbose, "verbose", false, "Extra diagnostic log on stderr.")
	fs.BoolVar(&noColor, "no-color", false, "Disable color even when stdout is a TTY.")
	fs.Usage = func() { fmt.Fprint(os.Stderr, logTailHelp()) }
	if err := fs.Parse(subArgs); err != nil {
		return err
	}

	useJSON := globalJSON || localJSON

	var types []string
	if typesCsv != "" {
		for _, t := range strings.Split(typesCsv, ",") {
			if tt := strings.TrimSpace(t); tt != "" {
				types = append(types, tt)
			}
		}
	}

	var sinceMs int64
	if sinceStr != "" {
		d, err := time.ParseDuration(sinceStr)
		if err != nil {
			return fmt.Errorf("invalid --since %q: %w (use Go duration: 5m, 30s, 1h30m)", sinceStr, err)
		}
		sinceMs = d.Milliseconds()
	}

	var rx *regexp.Regexp
	if filterRegex != "" {
		r, err := regexp.Compile(filterRegex)
		if err != nil {
			return fmt.Errorf("invalid --filter regex: %w", err)
		}
		rx = r
	}

	inst, err := client.DiscoverInstance(flagProject, flagPort)
	if err != nil {
		return err
	}

	// Signal handling: first Ctrl+C cancels ctx → graceful exit. Nothing
	// to drain (unlike watch), so a single signal is enough.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fmtr := newLogFormatter(useJSON, verbose, noColor, rx)
	filter := client.LogStreamFilter{
		Types:      types,
		Stacktrace: stacktrace,
		SinceMs:    sinceMs,
	}

	backoff := newReconnectBackoff()
	firstAttempt := true
	for {
		if ctx.Err() != nil {
			return nil
		}

		startedAt := time.Now()
		body, err := client.StreamLogs(ctx, inst, filter, 5*time.Second)
		if err != nil {
			// Version skew: non-retryable, abort immediately.
			if errors.Is(err, client.ErrConnectorTooOld) {
				return err
			}
			// No Unity running: retry once, then give up.
			if strings.Contains(err.Error(), "cannot connect to Unity") ||
				strings.Contains(err.Error(), "connection refused") {
				if firstAttempt {
					fmtr.notice("no Unity reachable yet — retrying in 1s…")
					firstAttempt = false
					if !sleepCtx(ctx, 1*time.Second) {
						return nil
					}
					continue
				}
				return fmt.Errorf("cannot reach Unity at port %d: %w", inst.Port, err)
			}
			// Bad filter: 400 from server. Non-retryable.
			if strings.Contains(err.Error(), "Unknown type value") ||
				strings.Contains(err.Error(), "Unknown stacktrace value") ||
				strings.Contains(err.Error(), "Invalid since_ms") {
				return err
			}
			// Anything else: back off and retry.
			d := backoff.next()
			fmtr.reconnect(d, err.Error())
			if !sleepCtx(ctx, d) {
				return nil
			}
			continue
		}
		firstAttempt = false

		if verbose {
			fmtr.notice(fmt.Sprintf("connected to port %d (filters: type=%v, stack=%s, since_ms=%d)",
				inst.Port, filter.Types, filter.Stacktrace, filter.SinceMs))
		}

		reachedStreaming := false
		parseErr := client.ParseSSEStream(ctx, body, func(ev client.StreamEvent) error {
			reachedStreaming = true
			return fmtr.emit(ev)
		})
		_ = body.Close()

		// Classify exit.
		if errors.Is(parseErr, context.Canceled) || ctx.Err() != nil {
			return nil
		}

		// Duration of the stream governs whether this is a "real"
		// successful connection (reset backoff) or a quick failure
		// (advance backoff). Plan D6.
		heldFor := time.Since(startedAt)
		if reachedStreaming && heldFor >= 5*time.Second {
			backoff.reset()
		}

		if errors.Is(parseErr, client.ErrStreamInterrupted) {
			d := backoff.next()
			fmtr.reconnect(d, "stream interrupted")
			if !sleepCtx(ctx, d) {
				return nil
			}
			continue
		}

		// Unexpected error — back off and retry.
		d := backoff.next()
		fmtr.reconnect(d, fmt.Sprintf("parse error: %v", parseErr))
		if !sleepCtx(ctx, d) {
			return nil
		}
	}
}

// sleepCtx sleeps for d or until ctx is cancelled. Returns false if
// cancelled (caller should exit).
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// logListCmd routes to the existing `console` Connector tool so vocabulary
// stays consistent: `log list` is the historical snapshot, `log tail` is
// live. Sub-args pass through unchanged — flags like --lines, --type,
// --stacktrace, --clear on the snapshot side remain handled Connector-side.
func logListCmd(subArgs []string, globalJSON bool) error {
	inst, err := client.DiscoverInstance(flagProject, flagPort)
	if err != nil {
		return err
	}
	if err := waitForAlive(inst.Port, flagTimeout); err != nil {
		return err
	}
	params, err := buildParams(subArgs, nil)
	if err != nil {
		return err
	}
	resp, err := client.Send(inst, "console", params, flagTimeout)
	if err != nil {
		return err
	}
	printResponse(resp, "console", inst, globalJSON)
	if !resp.Success {
		return fmt.Errorf("console command failed")
	}
	return nil
}

// ----------------------------------------------------------------------
// Reconnect backoff
// ----------------------------------------------------------------------

type reconnectBackoff struct {
	next_ time.Duration
}

func newReconnectBackoff() *reconnectBackoff { return &reconnectBackoff{next_: time.Second} }

func (b *reconnectBackoff) reset() { b.next_ = time.Second }

func (b *reconnectBackoff) next() time.Duration {
	cur := b.next_
	if cur < time.Second {
		cur = time.Second
	}
	b.next_ *= 2
	if b.next_ > 4*time.Second {
		b.next_ = 4 * time.Second
	}
	return cur
}

// ----------------------------------------------------------------------
// Output formatter
// ----------------------------------------------------------------------

type logFormatter struct {
	useJSON bool
	verbose bool
	useAnsi bool
	rx      *regexp.Regexp
	// out/errw are injection points for tests. In production they always
	// point at os.Stdout / os.Stderr via newLogFormatter.
	out  io.Writer
	errw io.Writer
}

func newLogFormatter(useJSON, verbose, noColor bool, rx *regexp.Regexp) *logFormatter {
	// Only enable color when: (1) caller didn't request plain text,
	// (2) --no-color not set, (3) stdout is a terminal. JSON output
	// is always colorless.
	useAnsi := !useJSON && !noColor && term.IsTerminal(int(os.Stdout.Fd()))
	return &logFormatter{
		useJSON: useJSON,
		verbose: verbose,
		useAnsi: useAnsi,
		rx:      rx,
		out:     os.Stdout,
		errw:    os.Stderr,
	}
}

// emit renders one SSE event. Returns an error only if an I/O problem on
// stdout would make further writes pointless.
func (f *logFormatter) emit(ev client.StreamEvent) error {
	switch ev.Event {
	case "log":
		return f.emitLog(ev.Data)
	case "reload":
		return f.emitMarker("reload", ev.Data)
	case "dropped":
		return f.emitMarker("dropped", ev.Data)
	case "truncated":
		return f.emitMarker("truncated", ev.Data)
	default:
		// Unknown event name — skip, don't error.
		return nil
	}
}

type logFrame struct {
	T       int64  `json:"t"`
	Type    string `json:"type"`
	Message string `json:"message"`
	Stack   string `json:"stack"`
	File    string `json:"file"`
	Line    int    `json:"line"`
}

func (f *logFormatter) emitLog(data json.RawMessage) error {
	var frame logFrame
	if err := json.Unmarshal(data, &frame); err != nil {
		// Drop malformed frame rather than tearing down the stream.
		return nil
	}
	if f.rx != nil && !f.rx.MatchString(frame.Message) {
		return nil
	}
	if f.useJSON {
		return f.emitLogJSON(frame)
	}
	return f.emitLogText(frame)
}

func (f *logFormatter) emitLogJSON(frame logFrame) error {
	obj := map[string]interface{}{
		"kind":    "log",
		"t":       time.UnixMilli(frame.T).UTC().Format(time.RFC3339Nano),
		"level":   strings.ToLower(frame.Type),
		"message": frame.Message,
	}
	if frame.Stack != "" {
		obj["stack"] = frame.Stack
	}
	if frame.File != "" {
		obj["file"] = frame.File
	}
	if frame.Line > 0 {
		obj["line"] = frame.Line
	}
	return f.writeNDJSON(obj)
}

func (f *logFormatter) emitLogText(frame logFrame) error {
	ts := time.UnixMilli(frame.T).Format("15:04:05")
	tag := logTypeTag(frame.Type)
	if f.useAnsi {
		tag = colorFor(frame.Type) + tag + ansiReset
	}
	first := fmt.Sprintf("%s %s %s", ts, tag, frame.Message)
	if _, err := fmt.Fprintln(f.out, first); err != nil {
		return err
	}
	if frame.Stack != "" {
		for _, line := range strings.Split(frame.Stack, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			if _, err := fmt.Fprintln(f.out, "  "+line); err != nil {
				return err
			}
		}
	}
	return nil
}

func (f *logFormatter) emitMarker(kind string, data json.RawMessage) error {
	if f.useJSON {
		var obj map[string]interface{}
		if err := json.Unmarshal(data, &obj); err != nil {
			obj = map[string]interface{}{}
		}
		obj["kind"] = kind
		return f.writeNDJSON(obj)
	}
	// Plain-text markers go to stderr so scripts piping stdout keep
	// seeing only log bodies.
	msg := fmt.Sprintf("[log] %s %s", kind, string(data))
	_, err := fmt.Fprintln(f.errw, msg)
	return err
}

func (f *logFormatter) reconnect(delay time.Duration, reason string) {
	if f.useJSON {
		obj := map[string]interface{}{
			"kind":   "reconnect",
			"in_ms":  delay.Milliseconds(),
			"reason": reason,
		}
		_ = f.writeNDJSON(obj)
		return
	}
	_, _ = fmt.Fprintf(f.errw, "[log] disconnected (%s) — reconnecting in %s…\n", reason, delay)
}

func (f *logFormatter) notice(msg string) {
	if f.useJSON {
		_ = f.writeNDJSON(map[string]interface{}{"kind": "notice", "message": msg})
		return
	}
	_, _ = fmt.Fprintln(f.errw, "[log] "+msg)
}

func (f *logFormatter) writeNDJSON(obj map[string]interface{}) error {
	b, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	if _, err := f.out.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

// ----------------------------------------------------------------------
// ANSI / tag helpers
// ----------------------------------------------------------------------

const (
	ansiReset  = "\x1b[0m"
	ansiRed    = "\x1b[31m"
	ansiYellow = "\x1b[33m"
	ansiDim    = "\x1b[2m"
)

func logTypeTag(t string) string {
	switch t {
	case "Error", "Exception", "Assert":
		return "[E]"
	case "Warning":
		return "[W]"
	case "Log":
		return "[L]"
	}
	return "[" + t + "]"
}

func colorFor(t string) string {
	switch t {
	case "Error", "Exception", "Assert":
		return ansiRed
	case "Warning":
		return ansiYellow
	case "Log":
		return ansiDim
	}
	return ""
}

// ----------------------------------------------------------------------
// Help text
// ----------------------------------------------------------------------

func logHelp() string {
	return `Usage: udit log <subcommand> [options]

Subcommands:
  tail       Live stream of Unity console messages (SSE; reconnects on domain reload).
  list       Snapshot of Unity's console (alias for ` + "`udit console`" + `).

Run ` + "`udit log <subcommand> --help`" + ` for per-subcommand flags.
`
}

func logTailHelp() string {
	return `Usage: udit log tail [options]

Stream Unity console messages as they happen. Connects to the same Unity
instance ` + "`udit status`" + ` reports; survives domain reloads with an
automatic reconnect.

Options:
  --type <csv>       error,warning,log,assert,exception  (default: all)
  --stacktrace MODE  none | user | full                  (default: user)
  --since DURATION   Backfill last N (e.g. 5m, 30s, 1h30m). Live-only when omitted.
  --filter REGEX     Client-side regex on message body (drop non-matching).
  --json             NDJSON on stdout (OR with global --json).
  --verbose          Extra diagnostic log on stderr (connection notices).
  --no-color         Disable ANSI even when stdout is a TTY.

Signals:
  Ctrl+C             Clean exit (no drain — nothing in flight).

Examples:
  udit log tail
  udit log tail --type error,warning
  udit log tail --since 5m --filter "NullReference"
  udit log tail --json | tee session.log
`
}

// Force the unused-but-present io import to remain visible — referenced
// indirectly via client.StreamLogs' body return type. Silences any future
// goimports trim pass that doesn't understand indirect package use.
var _ = io.EOF
