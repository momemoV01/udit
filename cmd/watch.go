package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/momemoV01/udit/internal/watch"
	"gopkg.in/yaml.v3"
)

// runWatch is the entry point for `udit watch`. It's called from root.go
// *before* the DiscoverInstance / waitForAlive path, so watch can start
// even when Unity isn't running — hooks that don't depend on Unity still
// fire, and hooks that do get surface-level errors from their sub-command.
//
// globalJSON is root.go's --json flag value; watch OR's it with its own
// --json local flag.
func runWatch(subArgs []string, globalJSON bool) error {
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	var (
		configPath string
		localJSON  bool
		verbose    bool
		noExec     bool
	)
	fs.StringVar(&configPath, "config", "", "Path to .udit.yaml (default: walk up from cwd)")
	fs.BoolVar(&localJSON, "json", false, "Emit NDJSON event log on stdout")
	fs.BoolVar(&verbose, "verbose", false, "Emit verbose diagnostic log on stderr")
	fs.BoolVar(&noExec, "no-exec", false, "Print what would run; don't execute hooks")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, watchHelp())
	}
	if err := fs.Parse(subArgs); err != nil {
		return err
	}

	useJSON := globalJSON || localJSON

	cfg, cfgPath, err := loadWatchConfig(configPath)
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	cfg = cfg.Defaults()

	projectRoot, err := detectProjectRoot(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[watch] could not detect Unity project root: %v (using CLI cwd for $RELFILE)\n", err)
		projectRoot, _ = os.Getwd()
	}

	caseInsensitive := false
	if cfg.CaseInsensitive != nil {
		caseInsensitive = *cfg.CaseInsensitive
	}
	defaultsIgnore := true
	if cfg.DefaultsIgnore != nil {
		defaultsIgnore = *cfg.DefaultsIgnore
	}

	ign := watch.NewIgnorer(cfg.Ignore, defaultsIgnore, caseInsensitive)
	matcher := watch.NewMatcher(cfg.Hooks, caseInsensitive)

	// Build the executor — real process unless --no-exec.
	var exe watch.Executor = newUditExecutor()
	if noExec {
		exe = &dryRunExecutor{logger: func(format string, args ...interface{}) {
			stderrLog(fmt.Sprintf(format, args...))
		}}
	}

	// Logger honors --json (NDJSON on stdout) and --verbose (richer
	// human log on stderr). We wrap the runner's simple format-string
	// logger with a structured version.
	logger := newLogger(useJSON, verbose)

	runner := watch.NewRunner(cfg, projectRoot, exe, watch.RealClock{}, logger.forRunner())

	// Watcher is rooted at the project root (or cwd if no project root).
	// Fallback to cwd is deliberate — non-Unity users might want watch too.
	watchRoot := projectRoot
	if watchRoot == "" {
		watchRoot, _ = os.Getwd()
	}
	w, err := watch.NewWatcher(watchRoot, ign, cfg.BufferSize)
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}

	// Debouncer sits between Watcher and Runner. Events → debounce → on fire:
	// match hooks, dispatch each.
	deb := watch.NewDebouncer(watch.RealClock{}, cfg.Debounce, func(ev watch.Event) {
		rel, _ := filepath.Rel(watchRoot, ev.Path)
		rel = filepath.ToSlash(rel)
		hooks := matcher.Match(rel)
		for _, h := range hooks {
			runner.Dispatch(h, ev)
		}
	}, watch.DefaultFileExists)

	// Run-on-start hooks — fire the user-opted-in hooks once at startup
	// against existing files. Done before Watcher.Start so the initial
	// fire doesn't race with live events.
	if err := fireRunOnStart(watchRoot, cfg.Hooks, caseInsensitive, ign, runner); err != nil {
		logger.info("[watch] run_on_start error: %v", err)
	}

	// Log the startup context so users know we're alive and what's loaded.
	logger.info("[watch] root=%s project=%s hooks=%d debounce=%s on_busy=%s max_parallel=%d",
		watchRoot, projectRoot, len(cfg.Hooks), cfg.Debounce, cfg.OnBusy, cfg.MaxParallel)
	if cfgPath != "" {
		logger.info("[watch] config=%s", cfgPath)
	}

	// Start watcher in its own goroutine — Start() is blocking.
	go func() {
		if err := w.Start(); err != nil {
			logger.info("[watch] watcher stopped: %v", err)
		}
	}()

	// Error pump: fsnotify can emit errors (buffer overflow, add failures)
	// which we log non-fatally.
	go func() {
		for err := range w.Errors() {
			logger.info("[watch] fsnotify: %v", err)
		}
	}()

	// Signal handling: first Ctrl+C ⇒ drain + exit; second within 2s ⇒
	// force-kill. Windows os.Interrupt covers Ctrl+C + Ctrl+Break.
	sigChan := make(chan os.Signal, 2)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Event pump: forward watcher → debouncer. Runs in the main goroutine
	// so it exits cleanly on shutdown.
	eventsCh := w.Events()
	firstSignalAt := time.Time{}
loop:
	for {
		select {
		case ev, ok := <-eventsCh:
			if !ok {
				break loop
			}
			deb.Enqueue(ev)
		case sig := <-sigChan:
			now := time.Now()
			if !firstSignalAt.IsZero() && now.Sub(firstSignalAt) < 2*time.Second {
				logger.info("[watch] force quit on second %s", sig)
				_ = w.Stop()
				os.Exit(130)
			}
			firstSignalAt = now
			logger.info("[watch] %s received — draining; press again within 2s to force quit", sig)
			_ = w.Stop()
		}
	}

	// Flush any pending debounce entries so in-flight hooks fire.
	deb.Flush()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	runner.Shutdown(shutdownCtx)

	if errors.Is(shutdownCtx.Err(), context.DeadlineExceeded) {
		logger.info("[watch] shutdown timeout — some hooks may still be running")
	}
	logger.info("[watch] exited cleanly")
	return nil
}

// loadWatchConfig reads a .udit.yaml either at an explicit path (via
// --config) or via the existing walk-up discovery. Returns the parsed
// WatchCfg + discovered config path. Fails with a user-friendly error if
// no config is found (watch is mandatory config-driven in v0.6.0 MVP;
// ad-hoc mode deferred to v0.6.x).
func loadWatchConfig(explicitPath string) (watch.WatchCfg, string, error) {
	if explicitPath != "" {
		abs, err := filepath.Abs(explicitPath)
		if err != nil {
			return watch.WatchCfg{}, "", fmt.Errorf("resolve --config path: %w", err)
		}
		cfg, err := loadConfigFile(abs)
		if err != nil {
			return watch.WatchCfg{}, "", err
		}
		return cfg.Watch, abs, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return watch.WatchCfg{}, "", err
	}
	cfg, cfgPath := LoadConfig(cwd)
	if cfg == nil {
		return watch.WatchCfg{}, "", fmt.Errorf("no .udit.yaml found walking up from %s — `watch` requires a config in v0.6.0 (ad-hoc mode ships in v0.6.x)", cwd)
	}
	return cfg.Watch, cfgPath, nil
}

// detectProjectRoot heuristic: walk up from `start` looking for a
// directory that has BOTH `Assets/` and `ProjectSettings/` as children —
// the canonical Unity project root. `start` may be a file path (e.g.
// the discovered `.udit.yaml`) or a directory (e.g. cwd for `init`);
// files are resolved to their parent before walking.
func detectProjectRoot(startHint string) (string, error) {
	start := startHint
	if start == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		start = cwd
	}
	// Normalize: if start points at a file, use its containing directory.
	// If it's a directory we walk from there directly.
	dir := start
	if info, statErr := os.Stat(start); statErr == nil && !info.IsDir() {
		dir = filepath.Dir(start)
	}
	for {
		assetsInfo, errA := os.Stat(filepath.Join(dir, "Assets"))
		psInfo, errP := os.Stat(filepath.Join(dir, "ProjectSettings"))
		if errA == nil && assetsInfo.IsDir() && errP == nil && psInfo.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no Unity project root (Assets+ProjectSettings) found walking up")
		}
		dir = parent
	}
}

// fireRunOnStart walks the project root looking for files that match a
// run_on_start=true hook, enqueueing one synthetic create event per match.
// Silently skipped for hooks that don't opt in.
func fireRunOnStart(root string, hooks []watch.Hook, caseInsensitive bool, ign *watch.Ignorer, runner *watch.Runner) error {
	any := false
	for _, h := range hooks {
		if h.RunOnStart {
			any = true
			break
		}
	}
	if !any {
		return nil
	}
	matcher := watch.NewMatcher(hooks, caseInsensitive)
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil {
			return nil
		}
		if d.IsDir() {
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			if ign != nil && ign.Match(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if ign != nil && ign.Match(rel) {
			return nil
		}
		for _, h := range matcher.Match(rel) {
			if h.RunOnStart {
				runner.Dispatch(h, watch.Event{Path: filepath.ToSlash(path), Kind: "create"})
			}
		}
		return nil
	})
}

// loadConfigFile reads a specific config path (as opposed to LoadConfig's
// walk-up behavior) and parses it. Used for `udit watch --config foo.yaml`.
func loadConfigFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &cfg, nil
}

// uditExecutor is the real Executor — forks exec.Command with the udit
// binary resolved via os.Executable() so nested invocations use the same
// build as the running watch process.
type uditExecutor struct {
	binary string
}

func newUditExecutor() *uditExecutor {
	bin, err := os.Executable()
	if err != nil || bin == "" {
		// Fall back to PATH lookup — user is on a weird setup.
		bin = "udit"
	}
	return &uditExecutor{binary: bin}
}

func (e *uditExecutor) Run(ctx context.Context, argv []string, env []string) (int, error) {
	cmd := exec.CommandContext(ctx, e.binary, argv...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	// exec.ExitError carries the child's exit code; other errors are
	// infrastructure failures (spawn, lost pipe).
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil
	}
	return -1, err
}

// dryRunExecutor prints what would run but doesn't fork anything. Used
// when --no-exec is passed — useful for validating .udit.yaml patterns.
type dryRunExecutor struct {
	logger func(format string, args ...interface{})
}

func (e *dryRunExecutor) Run(ctx context.Context, argv []string, env []string) (int, error) {
	e.logger("[watch] DRY-RUN argv=%v env=%v", argv, env)
	return 0, nil
}

// logger wraps the raw format-string Logger used by the runner so we can
// route output based on --json and --verbose.
type watchLogger struct {
	useJSON bool
	verbose bool
}

func newLogger(useJSON, verbose bool) *watchLogger {
	return &watchLogger{useJSON: useJSON, verbose: verbose}
}

func (l *watchLogger) info(format string, args ...interface{}) {
	line := fmt.Sprintf(format, args...)
	if l.useJSON {
		l.emitJSON("info", line)
		return
	}
	stderrLog(l.stamp() + " " + line)
}

func (l *watchLogger) forRunner() watch.Logger {
	// The runner calls logger(format, args...) for every state transition.
	// Route through our wrapper so JSON mode sees structured events.
	return func(format string, args ...interface{}) {
		line := fmt.Sprintf(format, args...)
		if l.useJSON {
			l.emitJSON("hook", line)
			return
		}
		if l.verbose || !strings.Contains(line, "DRY-RUN") {
			stderrLog(l.stamp() + " " + line)
		}
	}
}

func (l *watchLogger) emitJSON(kind, line string) {
	obj := map[string]interface{}{
		"t":    time.Now().UTC().Format(time.RFC3339Nano),
		"kind": kind,
		"line": line,
	}
	data, _ := json.Marshal(obj)
	_, _ = fmt.Fprintln(os.Stdout, string(data))
}

func (l *watchLogger) stamp() string {
	return time.Now().Format("15:04:05")
}

func stderrLog(s string) {
	fmt.Fprintln(os.Stderr, s)
}

// watchHelp is the --help text. Kept concise; agents can read with
// `udit help watch`.
func watchHelp() string {
	return `Usage: udit watch [options]

Run pre-defined hooks from .udit.yaml when matching files change.

Options:
  --config PATH     Path to .udit.yaml (default: walk up from cwd)
  --json            Emit NDJSON event log on stdout
  --verbose         Emit verbose diagnostic log on stderr
  --no-exec         Print what would run; don't execute hooks

Example .udit.yaml:
  watch:
    debounce: 300ms
    hooks:
      - name: compile
        paths: [Assets/**/*.cs]
        run: refresh --compile
      - name: reserialize
        paths: [Assets/**/*.prefab, Assets/**/*.unity]
        run: reserialize $RELFILE

Signals:
  Ctrl+C once    — drain in-flight hooks + exit cleanly
  Ctrl+C twice   — force quit (within 2s of first)

See https://github.com/momemoV01/udit for full documentation.
`
}
