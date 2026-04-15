// Package watch implements `udit watch` — a long-running file-system watcher
// that fires pre-defined udit CLI sub-commands when files matching configured
// globs change. See docs/ROADMAP.md Phase 5.1 and plan humble-baking-peacock.md
// for the design rationale.
package watch

import (
	"fmt"
	"runtime"
	"time"
)

// WatchCfg is the on-disk shape of the `watch:` section inside `.udit.yaml`.
// Embedded into cmd.Config at parse time; empty (zero-value) == watch feature
// is not configured and `udit watch` exits with a user-friendly error.
type WatchCfg struct {
	// Debounce is the default per-file quiet-period before a hook is eligible
	// to fire. Events arriving within this window for the same path are
	// collapsed into one dispatch. Zero or negative ⇒ default 300ms.
	Debounce time.Duration `yaml:"debounce"`

	// OnBusy selects the concurrency policy when a hook receives new events
	// while a previous invocation is still running.
	//
	//   queue  (default) — accumulate events into a dedupe'd set; fire once
	//                      when the current run exits. Unity-safe.
	//   ignore           — drop events while a run is in progress.
	//
	// `restart` was considered and dropped from MVP; see plan D4.
	OnBusy string `yaml:"on_busy"`

	// BufferSize tunes fsnotify's internal ReadDirectoryChangesW buffer on
	// Windows. Unity projects can have thousands of sub-dirs; the default
	// 64KB can overflow during mass import. Zero ⇒ default 524288 (512KB).
	BufferSize int `yaml:"buffer_size"`

	// MaxParallel caps concurrent hook executions to guard against fork-bombs
	// when broad globs match all at once. Zero ⇒ default 4.
	MaxParallel int `yaml:"max_parallel"`

	// CaseInsensitive toggles glob matching case sensitivity. Pointer so an
	// unset field is distinguishable from explicit false — we default to
	// true on Windows (filesystem is case-preserving-but-insensitive) and
	// false elsewhere.
	CaseInsensitive *bool `yaml:"case_insensitive"`

	// Ignore is a list of glob patterns whose matching paths are skipped by
	// the watcher. Appended AFTER the built-in Unity defaults
	// (Library/, Temp/, Logs/, …) unless DefaultsIgnore is false.
	Ignore []string `yaml:"ignore"`

	// DefaultsIgnore controls inclusion of built-in Unity ignore patterns.
	// Pointer to distinguish unset (⇒ true) from explicit false.
	DefaultsIgnore *bool `yaml:"defaults_ignore"`

	// Hooks is the ordered list of user-defined rules. Evaluation stops
	// after all matching hooks are found (a single event may match many;
	// each hook fires independently).
	Hooks []Hook `yaml:"hooks"`
}

// Hook is one user-defined rule: "when a file matching any of `Paths` changes,
// run `Run`." All non-Run fields are optional.
type Hook struct {
	// Name is a human-readable identifier used in log prefixes. Required
	// (validated at config load). Two hooks with the same name is an error.
	Name string `yaml:"name"`

	// Paths is the list of glob patterns that trigger this hook. Patterns
	// use doublestar semantics (`**` = any depth). Forward slashes; paths
	// are normalized via filepath.ToSlash before matching.
	Paths []string `yaml:"paths"`

	// Run is the command string passed to the udit binary (same process
	// that is running `watch`). Resolved via os.Executable() and invoked
	// with exec.Command("udit", splitArgs(expanded)...). Variables:
	//   $FILE        — single absolute path; presence triggers per-file invocation
	//   $RELFILE     — project-relative forward-slash form (Assets/Scripts/Foo.cs)
	//   $FILES       — env-var UDIT_CHANGED_FILES (newline-separated absolute)
	//   $RELFILES    — env-var UDIT_CHANGED_RELFILES (newline-separated relative)
	//   $EVENT       — dominant event in the batch (create|write|remove|rename)
	//   $HOOK        — this hook's Name
	// Using both $FILE and $FILES in the same Run string is a config-load
	// error — pick one batching discipline.
	Run string `yaml:"run"`

	// Debounce overrides WatchCfg.Debounce for just this hook. Zero ⇒
	// inherit from WatchCfg. Useful for slow hooks (build) where a larger
	// window is appropriate.
	Debounce time.Duration `yaml:"debounce"`

	// OnBusy overrides WatchCfg.OnBusy for just this hook. Empty ⇒ inherit.
	OnBusy string `yaml:"on_busy"`

	// RunOnStart, when true, fires this hook once at `udit watch` startup
	// as if every matching existing file had just been created. Defaults to
	// false — clean start, react only to changes.
	RunOnStart bool `yaml:"run_on_start"`
}

// Defaults returns a WatchCfg with every optional field filled from
// platform-aware defaults. Called after YAML parse so user values survive.
func (c *WatchCfg) Defaults() WatchCfg {
	out := *c
	if out.Debounce <= 0 {
		out.Debounce = 300 * time.Millisecond
	}
	if out.OnBusy == "" {
		out.OnBusy = "queue"
	}
	if out.BufferSize <= 0 {
		out.BufferSize = 512 * 1024
	}
	if out.MaxParallel <= 0 {
		out.MaxParallel = 4
	}
	if out.CaseInsensitive == nil {
		b := runtime.GOOS == "windows"
		out.CaseInsensitive = &b
	}
	if out.DefaultsIgnore == nil {
		b := true
		out.DefaultsIgnore = &b
	}
	return out
}

// Validate checks the parsed config for user errors that cannot be caught by
// yaml unmarshalling alone: unknown policy values, duplicate hook names,
// $FILE/$FILES conflict, empty Run, etc. Returns a joined error listing every
// problem found (not just the first) so users can fix all in one pass.
func (c *WatchCfg) Validate() error {
	var errs []string

	switch c.OnBusy {
	case "", "queue", "ignore":
		// OK
	default:
		errs = append(errs, fmt.Sprintf("watch.on_busy: unknown value %q (accepted: queue, ignore)", c.OnBusy))
	}

	if len(c.Hooks) == 0 {
		errs = append(errs, "watch.hooks: at least one hook is required")
	}

	seen := map[string]int{}
	for i, h := range c.Hooks {
		prefix := fmt.Sprintf("watch.hooks[%d]", i)
		if h.Name == "" {
			errs = append(errs, prefix+": name is required")
		} else if prev, dup := seen[h.Name]; dup {
			errs = append(errs, fmt.Sprintf("%s: duplicate name %q (first seen at watch.hooks[%d])", prefix, h.Name, prev))
		} else {
			seen[h.Name] = i
		}
		if len(h.Paths) == 0 {
			errs = append(errs, prefix+": at least one pattern in `paths` is required")
		}
		if h.Run == "" {
			errs = append(errs, prefix+": run is required")
		}
		if hasPerFileVar(h.Run) && hasBatchVar(h.Run) {
			errs = append(errs,
				fmt.Sprintf("%s: cannot use both $FILE and $FILES in run (choose per-file invocation via $FILE or batch via $FILES)", prefix))
		}
		switch h.OnBusy {
		case "", "queue", "ignore":
			// OK
		default:
			errs = append(errs, fmt.Sprintf("%s: unknown on_busy %q", prefix, h.OnBusy))
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("invalid .udit.yaml watch config:\n  - %s", joinErrs(errs))
}

func joinErrs(xs []string) string {
	out := ""
	for i, s := range xs {
		if i > 0 {
			out += "\n  - "
		}
		out += s
	}
	return out
}
