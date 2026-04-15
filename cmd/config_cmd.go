package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// configCmd dispatches `udit config <subcommand>`. Surface:
//
//	config show      effective config (global flags + yaml merge)
//	config validate  schema check + watch/build/run sanity
//	config path      absolute path of the loaded .udit.yaml
//	config edit      open the config file in $EDITOR / $VISUAL
//
// No --help collision with `udit help config` — the subcommand surface
// keeps its own per-action help via `udit config <sub> --help`.
func configCmd(subArgs []string, globalJSON bool) error {
	if len(subArgs) == 0 {
		fmt.Fprint(os.Stderr, configHelp())
		return nil
	}
	sub := subArgs[0]
	rest := subArgs[1:]
	switch sub {
	case "show":
		return configShow(rest, globalJSON)
	case "validate":
		return configValidate(rest, globalJSON)
	case "path":
		return configPath(rest, globalJSON)
	case "edit":
		return configEdit(rest)
	case "-h", "--help", "help":
		fmt.Fprint(os.Stderr, configHelp())
		return nil
	default:
		return fmt.Errorf("unknown config subcommand %q (expected: show, validate, path, edit)", sub)
	}
}

// ----------------------------------------------------------------------
// config show — effective config as pretty text / JSON / raw yaml
// ----------------------------------------------------------------------

func configShow(subArgs []string, globalJSON bool) error {
	var (
		asJSON bool
		asYAML bool
	)
	for _, a := range subArgs {
		switch a {
		case "--json":
			asJSON = true
		case "--yaml":
			asYAML = true
		case "-h", "--help":
			fmt.Fprint(os.Stderr, `Usage: udit config show [--json|--yaml]

Print the effective configuration: the values udit is actually using
after merging CLI flags, environment, and `+"`.udit.yaml`"+`. Default
output is a human-readable layout; --json / --yaml emit machine
formats.
`)
			return nil
		}
	}
	if globalJSON {
		asJSON = true
	}

	if loadedConfig == nil {
		if asJSON {
			return writeJSON(map[string]interface{}{
				"path":   "",
				"loaded": false,
			})
		}
		fmt.Println("No .udit.yaml loaded. (Walk-up from cwd found nothing.)")
		fmt.Println("Run `udit init` to scaffold one.")
		return nil
	}

	if asJSON {
		return writeJSON(configJSON())
	}
	if asYAML {
		data, err := yaml.Marshal(loadedConfig)
		if err != nil {
			return fmt.Errorf("yaml marshal: %w", err)
		}
		if loadedConfigPath != "" {
			fmt.Printf("# loaded from: %s\n", loadedConfigPath)
		}
		fmt.Print(string(data))
		return nil
	}

	return configShowPretty()
}

// configJSON builds a plain map for JSON output — skips zero values so
// machine consumers see only what's actually configured.
func configJSON() map[string]interface{} {
	out := map[string]interface{}{
		"path":   loadedConfigPath,
		"loaded": true,
	}
	if loadedConfig.DefaultPort != 0 {
		out["default_port"] = loadedConfig.DefaultPort
	}
	if loadedConfig.DefaultTimeoutMs != 0 {
		out["default_timeout_ms"] = loadedConfig.DefaultTimeoutMs
	}
	if len(loadedConfig.Exec.Usings) > 0 {
		out["exec"] = map[string]interface{}{"usings": loadedConfig.Exec.Usings}
	}
	if len(loadedConfig.Watch.Hooks) > 0 {
		hooks := make([]map[string]interface{}, 0, len(loadedConfig.Watch.Hooks))
		for _, h := range loadedConfig.Watch.Hooks {
			hooks = append(hooks, map[string]interface{}{
				"name":  h.Name,
				"paths": h.Paths,
				"run":   h.Run,
			})
		}
		out["watch"] = map[string]interface{}{
			"hooks":    hooks,
			"on_busy":  loadedConfig.Watch.OnBusy,
			"debounce": loadedConfig.Watch.Debounce.String(),
		}
	}
	if len(loadedConfig.Build.Targets) > 0 {
		targets := map[string]interface{}{}
		for name, p := range loadedConfig.Build.Targets {
			entry := map[string]interface{}{
				"target": p.Target,
				"output": p.Output,
			}
			if len(p.Scenes) > 0 {
				entry["scenes"] = p.Scenes
			}
			if p.IL2CPP != nil {
				entry["il2cpp"] = *p.IL2CPP
			}
			if p.Development != nil {
				entry["development"] = *p.Development
			}
			targets[name] = entry
		}
		out["build"] = map[string]interface{}{"targets": targets}
	}
	if len(loadedConfig.Run.Tasks) > 0 {
		tasks := map[string]interface{}{}
		for name, t := range loadedConfig.Run.Tasks {
			tasks[name] = map[string]interface{}{
				"description":       t.Description,
				"steps":             t.Steps,
				"continue_on_error": t.ContinueOnError,
			}
		}
		out["run"] = map[string]interface{}{"tasks": tasks}
	}
	return out
}

func configShowPretty() error {
	if loadedConfigPath != "" {
		fmt.Printf("Config loaded from: %s\n", loadedConfigPath)
	}
	fmt.Println()

	if loadedConfig.DefaultPort != 0 || loadedConfig.DefaultTimeoutMs != 0 {
		fmt.Println("Global:")
		if loadedConfig.DefaultPort != 0 {
			fmt.Printf("  default_port:       %d\n", loadedConfig.DefaultPort)
		}
		if loadedConfig.DefaultTimeoutMs != 0 {
			fmt.Printf("  default_timeout_ms: %d\n", loadedConfig.DefaultTimeoutMs)
		}
		fmt.Println()
	}

	if len(loadedConfig.Exec.Usings) > 0 {
		fmt.Printf("Exec usings (%d):\n", len(loadedConfig.Exec.Usings))
		for _, u := range loadedConfig.Exec.Usings {
			fmt.Printf("  - %s\n", u)
		}
		fmt.Println()
	}

	if len(loadedConfig.Watch.Hooks) > 0 {
		w := loadedConfig.Watch
		fmt.Printf("Watch (%d hook%s):\n", len(w.Hooks), plural(len(w.Hooks)))
		if w.Debounce > 0 {
			fmt.Printf("  debounce:      %s\n", w.Debounce)
		}
		if w.OnBusy != "" {
			fmt.Printf("  on_busy:       %s\n", w.OnBusy)
		}
		if w.MaxParallel > 0 {
			fmt.Printf("  max_parallel:  %d\n", w.MaxParallel)
		}
		for _, h := range w.Hooks {
			flags := ""
			if h.RunOnStart {
				flags += " [run_on_start]"
			}
			if h.OnBusy != "" && h.OnBusy != w.OnBusy {
				flags += " [on_busy=" + h.OnBusy + "]"
			}
			fmt.Printf("  - %s: %s%s\n", h.Name, h.Run, flags)
			for _, p := range h.Paths {
				fmt.Printf("      %s\n", p)
			}
		}
		fmt.Println()
	}

	if len(loadedConfig.Build.Targets) > 0 {
		names := make([]string, 0, len(loadedConfig.Build.Targets))
		for n := range loadedConfig.Build.Targets {
			names = append(names, n)
		}
		sort.Strings(names)
		fmt.Printf("Build targets (%d):\n", len(names))
		for _, n := range names {
			p := loadedConfig.Build.Targets[n]
			flags := ""
			if p.IL2CPP != nil && *p.IL2CPP {
				flags += " il2cpp"
			}
			if p.Development != nil && *p.Development {
				flags += " dev"
			}
			target := p.Target
			if target == "" {
				target = "(no target)"
			}
			fmt.Printf("  %s: %s → %s%s\n", n, target, p.Output, flags)
		}
		fmt.Println()
	}

	if len(loadedConfig.Run.Tasks) > 0 {
		names := taskNames(loadedConfig.Run.Tasks)
		fmt.Printf("Run tasks (%d):\n", len(names))
		for _, n := range names {
			t := loadedConfig.Run.Tasks[n]
			flags := ""
			if t.ContinueOnError {
				flags = " [continue-on-error]"
			}
			desc := t.Description
			if desc == "" {
				desc = fmt.Sprintf("%d step%s", len(t.Steps), plural(len(t.Steps)))
			}
			fmt.Printf("  %s: %s%s\n", n, desc, flags)
		}
		fmt.Println()
	}

	return nil
}

// ----------------------------------------------------------------------
// config validate
// ----------------------------------------------------------------------

func configValidate(subArgs []string, globalJSON bool) error {
	asJSON := globalJSON
	for _, a := range subArgs {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprint(os.Stderr, `Usage: udit config validate [--json]

Check `+"`.udit.yaml`"+` for schema / semantic errors:
- watch hooks ($FILE vs $FILES conflict, missing name, empty paths)
- build presets (missing target/output)
- run tasks (empty steps)

Exits non-zero if any error is found.
`)
			return nil
		}
	}

	if loadedConfig == nil {
		if asJSON {
			_ = writeJSON(map[string]interface{}{
				"ok":     false,
				"errors": []string{"no .udit.yaml loaded"},
			})
			return errors.New("no .udit.yaml loaded")
		}
		return errors.New("no .udit.yaml loaded — run `udit init` to scaffold one")
	}

	var issues []string

	// Watch uses its own validator (already implemented for commit 1 of Phase 5.1).
	// Skip when the section is empty — it's optional.
	if len(loadedConfig.Watch.Hooks) > 0 || loadedConfig.Watch.Debounce > 0 {
		if err := loadedConfig.Watch.Validate(); err != nil {
			// Validate() returns a multi-line error; split and tag each line.
			for _, line := range strings.Split(err.Error(), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasSuffix(line, ":") {
					continue
				}
				issues = append(issues, "watch: "+strings.TrimPrefix(line, "- "))
			}
		}
	}

	// Build presets — each must have target + output at minimum.
	for name, p := range loadedConfig.Build.Targets {
		if p.Target == "" {
			issues = append(issues, fmt.Sprintf("build.targets.%s: missing `target`", name))
		}
		if p.Output == "" {
			issues = append(issues, fmt.Sprintf("build.targets.%s: missing `output`", name))
		}
	}

	// Run tasks — steps must exist.
	for name, t := range loadedConfig.Run.Tasks {
		if len(t.Steps) == 0 {
			issues = append(issues, fmt.Sprintf("run.tasks.%s: no steps", name))
		}
	}

	if asJSON {
		return writeJSON(map[string]interface{}{
			"ok":     len(issues) == 0,
			"path":   loadedConfigPath,
			"errors": issues,
		})
	}

	if len(issues) == 0 {
		fmt.Printf("OK — %s\n", loadedConfigPath)
		return nil
	}
	fmt.Fprintln(os.Stderr, "Config errors:")
	for _, i := range issues {
		fmt.Fprintln(os.Stderr, "  ✗ "+i)
	}
	return fmt.Errorf("%d config error%s", len(issues), plural(len(issues)))
}

// ----------------------------------------------------------------------
// config path
// ----------------------------------------------------------------------

func configPath(subArgs []string, globalJSON bool) error {
	asJSON := globalJSON
	for _, a := range subArgs {
		if a == "--json" {
			asJSON = true
		}
		if a == "-h" || a == "--help" {
			fmt.Fprint(os.Stderr, `Usage: udit config path [--json]

Print the absolute path of the `+"`.udit.yaml`"+` that udit loaded for
this invocation (via walk-up from cwd). Exit code 1 if none was found.
`)
			return nil
		}
	}
	if loadedConfigPath == "" {
		if asJSON {
			return writeJSON(map[string]interface{}{"path": "", "loaded": false})
		}
		return errors.New("no .udit.yaml found walking up from cwd")
	}
	if asJSON {
		return writeJSON(map[string]interface{}{"path": loadedConfigPath, "loaded": true})
	}
	fmt.Println(loadedConfigPath)
	return nil
}

// ----------------------------------------------------------------------
// config edit
// ----------------------------------------------------------------------

func configEdit(subArgs []string) error {
	for _, a := range subArgs {
		if a == "-h" || a == "--help" {
			fmt.Fprint(os.Stderr, `Usage: udit config edit

Open the loaded `+"`.udit.yaml`"+` in the editor named by $VISUAL or
$EDITOR. On Windows, falls back to "notepad" when neither is set.

Fails if no .udit.yaml was found — run `+"`udit init`"+` first.
`)
			return nil
		}
	}
	if loadedConfigPath == "" {
		return errors.New("no .udit.yaml to edit — run `udit init` to scaffold one")
	}

	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		if runtime.GOOS == "windows" {
			editor = "notepad"
		} else {
			return errors.New("no $VISUAL or $EDITOR set; export one and retry")
		}
	}

	// Parse the editor value as a shell-like command so users can set
	// `EDITOR="code --wait"` style env vars without surprises. Empty
	// after split ⇒ treat as a bare command.
	argv, err := splitRunStep(editor + " " + loadedConfigPath)
	if err != nil {
		return fmt.Errorf("parse EDITOR: %w", err)
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ----------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------

func writeJSON(obj interface{}) error {
	b, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(append(b, '\n'))
	return err
}

func configHelp() string {
	return `Usage: udit config <subcommand> [options]

Inspect and manage the loaded .udit.yaml.

Subcommands:
  show        Print the effective config (CLI flags + yaml merge).
  validate    Check the config for schema / semantic errors.
  path        Print the absolute path of the loaded .udit.yaml.
  edit        Open the config in $VISUAL / $EDITOR.

Run ` + "`udit config <subcommand> --help`" + ` for per-subcommand flags.

Examples:
  udit config show              # human-readable layout
  udit config show --json       # machine format
  udit config validate          # exit non-zero on any error
  udit config path              # just the path, for scripting
  udit config edit              # open in $EDITOR (or notepad on Windows)
`
}
