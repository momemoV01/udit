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
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"
)

// RunCfg is the on-disk shape of the `run:` section inside `.udit.yaml`.
// Parsed once at startup into the shared `loadedConfig`; `udit run <name>`
// looks up a task by key and executes its `steps:` list.
type RunCfg struct {
	// Tasks maps a task name to its definition. Keys are free-form
	// identifiers used on the CLI (e.g. "verify", "release_win64").
	Tasks map[string]RunTask `yaml:"tasks"`
}

// RunTask is one named entry under `run.tasks`. `make` / `npm run` feel —
// a curated sequence of udit sub-commands invoked in order.
type RunTask struct {
	// Description surfaces in `udit run` (no-arg list mode). Optional.
	Description string `yaml:"description"`

	// Steps is the ordered list of sub-commands to execute. Each step
	// is a single shell-like string parsed with POSIX-ish rules (quotes
	// + backslash) and dispatched to the same `udit` binary via
	// `exec.Command(os.Executable(), argv...)`. Prefix `run <other>`
	// to recurse — that's the hand-rolled alternative to first-class
	// `depends_on:` (see plan / CHANGELOG for the trade-off).
	Steps []string `yaml:"steps"`

	// ContinueOnError controls the failure mode when a step exits
	// non-zero. Default false ⇒ fail-fast (remaining steps skipped).
	// true ⇒ log the failure and proceed (CI nightly pattern).
	ContinueOnError bool `yaml:"continue_on_error"`
}

// Recursion-guard env vars. Every `udit run` invocation passes these
// forward so nested calls (a step of the form `run <other>`) can detect
// cycles + cap depth.
const (
	envRunDepth = "UDIT_RUN_DEPTH"
	envRunStack = "UDIT_RUN_STACK"
	maxRunDepth = 8
)

// runCmd dispatches `udit run [task] [flags]`.
//
// Forms:
//
//	udit run                — list tasks (printing descriptions + step count)
//	udit run <name>         — execute task
//	udit run <name> --dry-run  — print steps, don't execute
//	udit run <name> --json     — NDJSON progress for agents
func runCmd(subArgs []string, globalJSON bool) error {
	// Split positional args from flags up front — Go's flag package
	// stops at the first non-flag token, which would make
	// `udit run verify --dry-run` silently ignore the trailing flag.
	// We explicitly separate so both orders work:
	//   udit run verify --dry-run
	//   udit run --dry-run verify
	var positional []string
	var flagArgs []string
	for _, a := range subArgs {
		if strings.HasPrefix(a, "-") {
			flagArgs = append(flagArgs, a)
		} else {
			positional = append(positional, a)
		}
	}

	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	var (
		dryRun    bool
		localJSON bool
	)
	fs.BoolVar(&dryRun, "dry-run", false, "Print steps without executing them")
	fs.BoolVar(&localJSON, "json", false, "Emit NDJSON progress on stdout (OR with global --json)")
	fs.Usage = func() { fmt.Fprint(os.Stderr, runHelp()) }
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}

	useJSON := globalJSON || localJSON
	args := positional

	if loadedConfig == nil || len(loadedConfig.Run.Tasks) == 0 {
		return fmt.Errorf("no tasks defined — add a `run.tasks` section to .udit.yaml or run `udit init`")
	}

	if len(args) == 0 {
		return listTasks(loadedConfig.Run.Tasks, useJSON)
	}
	taskName := args[0]

	task, ok := loadedConfig.Run.Tasks[taskName]
	if !ok {
		available := taskNames(loadedConfig.Run.Tasks)
		return fmt.Errorf("no such task %q. Available: %s", taskName, strings.Join(available, ", "))
	}
	if len(task.Steps) == 0 {
		return fmt.Errorf("task %q has no steps", taskName)
	}

	// Recursion guard. Honor cap + detect cycles before we fork.
	depth, stack, err := checkRecursion(taskName)
	if err != nil {
		return err
	}

	// Signal handling: Ctrl+C cancels the context → current step's
	// exec.CommandContext kills the child, loop breaks.
	ctx, stopSig := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSig()

	return executeTask(ctx, taskName, &task, runOptions{
		DryRun:  dryRun,
		UseJSON: useJSON,
		Depth:   depth,
		Stack:   stack,
	})
}

type runOptions struct {
	DryRun  bool
	UseJSON bool
	Depth   int
	Stack   []string
}

// listTasks prints the available tasks. Default form is a small table;
// with --json, NDJSON so agents can enumerate without parsing prose.
func listTasks(tasks map[string]RunTask, useJSON bool) error {
	names := taskNames(tasks)
	if useJSON {
		for _, n := range names {
			t := tasks[n]
			obj := map[string]interface{}{
				"kind":              "task",
				"name":              n,
				"description":       t.Description,
				"steps":             len(t.Steps),
				"continue_on_error": t.ContinueOnError,
			}
			b, _ := json.Marshal(obj)
			fmt.Println(string(b))
		}
		return nil
	}

	fmt.Println("Available tasks:")
	maxNameLen := 4
	for _, n := range names {
		if len(n) > maxNameLen {
			maxNameLen = len(n)
		}
	}
	for _, n := range names {
		t := tasks[n]
		desc := t.Description
		if desc == "" {
			desc = fmt.Sprintf("(%d step%s)", len(t.Steps), plural(len(t.Steps)))
		}
		flags := ""
		if t.ContinueOnError {
			flags = "  [continue-on-error]"
		}
		fmt.Printf("  %-*s  %s%s\n", maxNameLen, n, desc, flags)
	}
	fmt.Println()
	fmt.Println("Run `udit run <name>` to execute, or add --dry-run to preview.")
	return nil
}

// executeTask runs the task's steps, respecting fail-fast vs continue-
// on-error. Returns an error (non-nil) when a step fails in fail-fast
// mode or when all-steps-ran-but-some-failed in continue mode — the
// caller surfaces these as non-zero exit codes.
func executeTask(ctx context.Context, name string, task *RunTask, opts runOptions) error {
	printer := newRunPrinter(opts.UseJSON)
	printer.taskStart(name, task, opts.DryRun)

	// Resolve udit binary once up front. Tests set UDIT_RUN_EXEC to
	// inject a stub binary; production code relies on os.Executable().
	binary := os.Getenv("UDIT_RUN_EXEC")
	if binary == "" {
		var err error
		binary, err = os.Executable()
		if err != nil || binary == "" {
			binary = "udit"
		}
	}

	// Extend env so nested `run` invocations see depth + stack.
	nestedDepth := opts.Depth + 1
	nestedStack := append(append([]string{}, opts.Stack...), name)
	nestedEnv := []string{
		envRunDepth + "=" + strconv.Itoa(nestedDepth),
		envRunStack + "=" + strings.Join(nestedStack, ":"),
	}

	started := time.Now()
	var anyFailed bool

	for i, step := range task.Steps {
		argv, err := splitRunStep(step)
		if err != nil {
			printer.stepError(i, len(task.Steps), step, err)
			anyFailed = true
			if !task.ContinueOnError {
				printer.taskEnd(name, started, false)
				return fmt.Errorf("parse step %d of task %q: %w", i+1, name, err)
			}
			continue
		}

		printer.stepStart(i, len(task.Steps), step)

		if opts.DryRun {
			printer.stepDry(i, len(task.Steps))
			continue
		}

		stepStart := time.Now()
		cmd := exec.CommandContext(ctx, binary, argv...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = append(os.Environ(), nestedEnv...)

		code := 0
		runErr := cmd.Run()
		dur := time.Since(stepStart)

		if runErr != nil {
			var ee *exec.ExitError
			if errors.As(runErr, &ee) {
				code = ee.ExitCode()
			} else {
				code = -1
			}
		}
		printer.stepExit(i, len(task.Steps), code, dur)

		if ctx.Err() != nil {
			printer.taskEnd(name, started, false)
			return ctx.Err()
		}

		if code != 0 {
			anyFailed = true
			if !task.ContinueOnError {
				printer.taskEnd(name, started, false)
				return fmt.Errorf("task %q: step %d (%q) failed with exit code %d",
					name, i+1, step, code)
			}
		}
	}

	printer.taskEnd(name, started, !anyFailed)
	if anyFailed {
		return fmt.Errorf("task %q: one or more steps failed (continue_on_error was set)", name)
	}
	return nil
}

// checkRecursion reads the env-var recursion markers and rejects cycles
// or excessive depth before we start forking. Returns the caller's
// current depth and stack (empty slices when this is the top-level call).
func checkRecursion(taskName string) (int, []string, error) {
	depth := 0
	if raw := os.Getenv(envRunDepth); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			depth = n
		}
	}
	if depth >= maxRunDepth {
		return 0, nil, fmt.Errorf("udit run recursion too deep (%d) — check for cycles or reduce nesting", depth)
	}

	var stack []string
	if raw := os.Getenv(envRunStack); raw != "" {
		stack = strings.Split(raw, ":")
	}
	for _, prev := range stack {
		if prev == taskName {
			chain := append(append([]string{}, stack...), taskName)
			return 0, nil, fmt.Errorf("cycle detected in `udit run` chain: %s", strings.Join(chain, " → "))
		}
	}

	return depth, stack, nil
}

// splitRunStep converts a single step string into exec.Command argv.
// POSIX-ish: whitespace separates, single/double quotes group, backslash
// escapes the next char outside quotes.
//
// We deliberately don't depend on watch's splitArgs (internal package)
// to keep run.go self-contained; semantics are identical.
func splitRunStep(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	inSingle := false
	inDouble := false
	escape := false
	emitted := false

	for i := 0; i < len(s); i++ {
		c := s[i]
		if escape {
			cur.WriteByte(c)
			escape = false
			emitted = true
			continue
		}
		if c == '\\' && !inSingle {
			escape = true
			continue
		}
		if c == '\'' && !inDouble {
			inSingle = !inSingle
			emitted = true
			continue
		}
		if c == '"' && !inSingle {
			inDouble = !inDouble
			emitted = true
			continue
		}
		if (c == ' ' || c == '\t') && !inSingle && !inDouble {
			if emitted {
				out = append(out, cur.String())
				cur.Reset()
				emitted = false
			}
			continue
		}
		cur.WriteByte(c)
		emitted = true
	}
	if inSingle || inDouble {
		return nil, fmt.Errorf("unterminated quote in step")
	}
	if escape {
		return nil, fmt.Errorf("trailing backslash in step")
	}
	if emitted {
		out = append(out, cur.String())
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty step")
	}
	return out, nil
}

// taskNames returns map keys in a deterministic (sorted) order so list
// output + error messages are reproducible across runs.
func taskNames(tasks map[string]RunTask) []string {
	out := make([]string, 0, len(tasks))
	for k := range tasks {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// ----------------------------------------------------------------------
// Printer — plain text with optional color, or NDJSON
// ----------------------------------------------------------------------

type runPrinter struct {
	useJSON bool
	useAnsi bool
}

func newRunPrinter(useJSON bool) *runPrinter {
	useAnsi := !useJSON && term.IsTerminal(int(os.Stdout.Fd()))
	return &runPrinter{useJSON: useJSON, useAnsi: useAnsi}
}

func (p *runPrinter) taskStart(name string, task *RunTask, dryRun bool) {
	if p.useJSON {
		obj := map[string]interface{}{
			"kind":              "task_start",
			"task":              name,
			"steps":             len(task.Steps),
			"continue_on_error": task.ContinueOnError,
			"dry_run":           dryRun,
		}
		p.emitJSON(obj)
		return
	}
	tag := "Running"
	if dryRun {
		tag = "DRY-RUN"
	}
	_, _ = fmt.Fprintf(os.Stdout, "%s task %q (%d step%s)\n", tag, name, len(task.Steps), plural(len(task.Steps)))
}

func (p *runPrinter) stepStart(i, n int, step string) {
	if p.useJSON {
		p.emitJSON(map[string]interface{}{
			"kind": "step_start",
			"i":    i + 1,
			"n":    n,
			"cmd":  step,
		})
		return
	}
	_, _ = fmt.Fprintf(os.Stdout, "[%d/%d] %s\n", i+1, n, step)
}

func (p *runPrinter) stepExit(i, n int, code int, dur time.Duration) {
	if p.useJSON {
		p.emitJSON(map[string]interface{}{
			"kind":        "step_exit",
			"i":           i + 1,
			"n":           n,
			"code":        code,
			"duration_ms": dur.Milliseconds(),
		})
		return
	}
	mark, color := "OK", ansiGreen
	if code != 0 {
		mark, color = fmt.Sprintf("FAIL exit=%d", code), ansiRed
	}
	if p.useAnsi {
		mark = color + mark + ansiReset
	}
	_, _ = fmt.Fprintf(os.Stdout, "      %s (%s)\n", mark, fmtDuration(dur))
}

func (p *runPrinter) stepDry(i, n int) {
	if p.useJSON {
		p.emitJSON(map[string]interface{}{
			"kind": "step_dry",
			"i":    i + 1,
			"n":    n,
		})
		return
	}
	_, _ = fmt.Fprintln(os.Stdout, "      (dry-run — not executed)")
}

func (p *runPrinter) stepError(i, n int, step string, err error) {
	if p.useJSON {
		p.emitJSON(map[string]interface{}{
			"kind":  "step_error",
			"i":     i + 1,
			"n":     n,
			"cmd":   step,
			"error": err.Error(),
		})
		return
	}
	_, _ = fmt.Fprintf(os.Stderr, "[%d/%d] parse error: %v (step: %q)\n", i+1, n, err, step)
}

func (p *runPrinter) taskEnd(name string, started time.Time, ok bool) {
	dur := time.Since(started)
	if p.useJSON {
		p.emitJSON(map[string]interface{}{
			"kind":        "task_complete",
			"task":        name,
			"duration_ms": dur.Milliseconds(),
			"success":     ok,
		})
		return
	}
	if ok {
		_, _ = fmt.Fprintf(os.Stdout, "Task %q completed in %s\n", name, fmtDuration(dur))
	} else {
		_, _ = fmt.Fprintf(os.Stdout, "Task %q failed after %s\n", name, fmtDuration(dur))
	}
}

func (p *runPrinter) emitJSON(obj map[string]interface{}) {
	b, _ := json.Marshal(obj)
	_, _ = os.Stdout.Write(append(b, '\n'))
}

func fmtDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
}

const (
	ansiGreen = "\x1b[32m"
)

// ----------------------------------------------------------------------
// Help text
// ----------------------------------------------------------------------

func runHelp() string {
	return `Usage: udit run [task] [options]

Execute a pre-defined workflow from .udit.yaml's ` + "`run.tasks`" + ` section.
With no arguments, lists available tasks.

Options:
  --dry-run        Print the step list without executing anything
  --json           NDJSON progress on stdout (OR with global --json)

Task schema (.udit.yaml):
  run:
    tasks:
      verify:
        description: "Full pre-commit verification"
        steps:
          - editor refresh --compile
          - test run --output test-results.xml
          - project validate
      release:
        description: "Production Windows build"
        steps:
          - run verify               # recurse into another task
          - build player --config prod_win64
      nightly:
        continue_on_error: true     # log failures, keep going
        steps:
          - test run --mode EditMode
          - test run --mode PlayMode
          - build player --config prod_win64

Behavior:
  - Steps run sequentially via exec of the same udit binary
    (os.Executable()). Each step inherits stdin/stdout/stderr.
  - Fail-fast by default: a non-zero exit code aborts the task.
    continue_on_error: true logs the failure and proceeds.
  - Recursion via ` + "`run <other>`" + ` as a step is supported.
    Depth capped at 8; cycles detected + rejected with the full chain.
  - Ctrl+C cancels the current step (SIGINT forwarded via context)
    and stops the task.

Examples:
  udit run                         # list tasks
  udit run verify                  # execute
  udit run verify --dry-run        # preview
  udit run nightly --json | jq     # NDJSON progress for agents
`
}
