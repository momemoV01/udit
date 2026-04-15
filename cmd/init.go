package cmd

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// initCmd writes a `.udit.yaml` scaffold to the current working directory
// (or --output). Handles --watch (embed a watch: section with example
// hooks) and --force (overwrite existing). Parallels `git init` /
// `npm init` / `go mod init` — a tiny ceremony that unblocks first-time
// users of `udit watch` who otherwise hit a bare "no .udit.yaml found"
// error with no guidance.
func initCmd(subArgs []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	var (
		output    string
		withWatch bool
		force     bool
	)
	fs.StringVar(&output, "output", ".udit.yaml", "Target path (default: .udit.yaml in cwd)")
	fs.BoolVar(&withWatch, "watch", false, "Include a watch: section with example hooks")
	fs.BoolVar(&force, "force", false, "Overwrite an existing file")
	fs.Usage = func() { fmt.Fprint(os.Stderr, initHelp()) }
	if err := fs.Parse(subArgs); err != nil {
		return err
	}

	abs, err := filepath.Abs(output)
	if err != nil {
		return fmt.Errorf("resolve --output: %w", err)
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
		return fmt.Errorf("write %s: %w", abs, err)
	}

	fmt.Printf("Wrote %s\n", abs)
	if withWatch {
		fmt.Println("Next: `udit watch` to start the file-change automation loop.")
	} else {
		fmt.Println("Next: edit the file to taste. `udit init --force --watch` drops a watch: section with sample hooks.")
	}
	return nil
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

Scaffold a ` + "`.udit.yaml`" + ` in the current directory (or at --output).

Options:
  --output PATH    Target path (default: .udit.yaml in cwd)
  --watch          Include a watch: section with example hooks
                   (compile_cs + reserialize_yaml)
  --force          Overwrite an existing file

Examples:
  udit init                      # minimal scaffold with commented-out sections
  udit init --watch              # same + concrete watch hooks
  udit init --output ./my.yaml
  udit init --force --watch      # rewrite existing config with watch hooks
`
}
