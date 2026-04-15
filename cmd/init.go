package cmd

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// initCmd writes a `.udit.yaml` scaffold. When --output is omitted, the
// target resolves to the Unity project root discovered by walking up
// from cwd (same heuristic `udit watch` uses: a directory with both
// `Assets/` and `ProjectSettings/`). Falls back to cwd when no Unity
// project is found — useful for non-Unity contexts or tests.
//
// Flags: --watch embeds a watch: section with sample hooks; --force
// overwrites an existing file.
func initCmd(subArgs []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	var (
		output    string
		withWatch bool
		force     bool
	)
	fs.StringVar(&output, "output", "", "Target path (default: Unity project root / cwd fallback)")
	fs.BoolVar(&withWatch, "watch", false, "Include a watch: section with example hooks")
	fs.BoolVar(&force, "force", false, "Overwrite an existing file")
	fs.Usage = func() { fmt.Fprint(os.Stderr, initHelp()) }
	if err := fs.Parse(subArgs); err != nil {
		return err
	}

	abs, source, err := resolveInitTarget(output)
	if err != nil {
		return err
	}

	if _, statErr := os.Stat(abs); statErr == nil && !force {
		return fmt.Errorf("%s already exists — pass --force to overwrite", abs)
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("stat %s: %w", abs, statErr)
	}

	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return fmt.Errorf("mkdir parent: %w", err)
	}

	content := scaffoldTemplate(withWatch)
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		// Shape this into actionable guidance — the most common hit is
		// running from a write-protected directory (System32, read-only
		// share) expecting Unity-project behavior. lint (ST1005) forbids
		// trailing punctuation/newline on error strings, so the guidance
		// prints to stderr separately.
		fmt.Fprintln(os.Stderr,
			"  `udit init` writes to the Unity project root if detected, otherwise cwd.\n"+
				"  Try running inside your Unity project directory, or pass --output <path>.")
		return fmt.Errorf("write %s: %w", abs, err)
	}

	fmt.Printf("Wrote %s (%s)\n", abs, source)
	if withWatch {
		fmt.Println("Next: `udit watch` to start the file-change automation loop.")
	} else {
		fmt.Println("Next: edit the file to taste. `udit init --force --watch` drops a watch: section with sample hooks.")
	}
	return nil
}

// resolveInitTarget picks the scaffold destination. Explicit --output wins;
// otherwise walks up from cwd looking for a Unity project root and
// falls back to cwd. Returns the absolute target path plus a human
// phrase describing how it was chosen (printed back to the user so they
// don't guess where the file landed).
func resolveInitTarget(explicitOutput string) (string, string, error) {
	if explicitOutput != "" {
		abs, err := filepath.Abs(explicitOutput)
		if err != nil {
			return "", "", fmt.Errorf("resolve --output: %w", err)
		}
		return abs, "from --output", nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("resolve cwd: %w", err)
	}
	if root, rerr := detectProjectRoot(cwd); rerr == nil {
		return filepath.Join(root, ".udit.yaml"), "Unity project root detected", nil
	}
	return filepath.Join(cwd, ".udit.yaml"), "no Unity project detected — using cwd", nil
}

// scaffoldTemplate composes the yaml scaffold. Always includes commented-
// out placeholders for the globals (default_port / default_timeout_ms /
// exec.usings). The watch: block is commented-out by default and becomes
// a concrete example when --watch is passed.
func scaffoldTemplate(withWatch bool) string {
	head := `# .udit.yaml — udit project configuration.
#
# udit walks upward from cwd looking for this file (stops at $HOME
# exclusive). Every field is optional — omitted ones fall back to the
# built-in default. See README.md for the full schema.

# Global Unity connection defaults.
# default_port: 8590            # skip auto-discovery and use a specific port
# default_timeout_ms: 120000    # HTTP request timeout in ms

# Usings auto-prepended to every ` + "`udit exec`" + ` invocation.
# exec:
#   usings:
#     - Unity.Entities
#     - Unity.Mathematics
`
	if !withWatch {
		head += `
# File-change automation (populate via ` + "`udit init --force --watch`" + `).
# watch:
#   debounce: 300ms
#   on_busy: queue             # queue | ignore
#   hooks:
#     - name: compile
#       paths: [Assets/**/*.cs]
#       run: refresh --compile
`
		return head
	}

	head += `
# File-change automation — ` + "`udit watch`" + ` reads this section.
# Each hook fires its ` + "`run`" + ` command (dispatched to the same udit
# binary) when a file matching any of its ` + "`paths`" + ` globs changes.
watch:
  debounce: 300ms
  on_busy: queue               # queue (default) | ignore
  max_parallel: 4              # global concurrent hook cap
  # case_insensitive: true     # default true on Windows, false elsewhere
  # ignore:                    # appended to built-in Unity defaults
  #   - "**/*.generated.cs"
  # defaults_ignore: true      # Library/Temp/Logs/… auto-ignored
  hooks:
    - name: compile_cs
      paths:
        - Assets/**/*.cs
        - Packages/**/*.cs
      run: refresh --compile

    - name: reserialize_yaml
      paths:
        - Assets/**/*.prefab
        - Assets/**/*.unity
        - Assets/**/*.asset
      run: reserialize $RELFILE
`
	return head
}

// initHelp is the --help text for ` + "`udit init`" + `.
func initHelp() string {
	return `Usage: udit init [options]

Scaffold a ` + "`.udit.yaml`" + `. Default target resolution:
  1. walk up from cwd looking for a Unity project root
     (a directory containing BOTH Assets/ and ProjectSettings/)
  2. fall back to cwd if no Unity project is found
  3. --output overrides both

Options:
  --output PATH    Target path (skips autodetect)
  --watch          Include a watch: section with example hooks
                   (compile_cs + reserialize_yaml)
  --force          Overwrite an existing file

Examples:
  udit init                      # from anywhere inside a Unity project
  udit init --watch              # + concrete watch hooks
  udit init --output ./my.yaml
  udit init --force --watch      # rewrite existing config
`
}
