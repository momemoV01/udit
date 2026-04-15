package cmd

import (
	"fmt"
	"strings"

	"github.com/momemoV01/udit/internal/client"
)

// BuildCfg is the on-disk shape of the `build:` section inside
// `.udit.yaml`. Parsed once at startup into the shared `loadedConfig`;
// the `udit build player --config <name>` flag looks up a named
// preset from Targets and merges it with per-call flags.
type BuildCfg struct {
	// Targets maps a preset name to its settings. Key is free-form
	// (e.g. "production", "dev", "demo_android"); lookup is via
	// --config <name>.
	Targets map[string]BuildPreset `yaml:"targets"`
}

// BuildPreset is one named `.udit.yaml` entry under `build.targets`.
// All fields are optional; pointer-to-bool so unset is distinguishable
// from explicit false. CLI flags always override preset values.
type BuildPreset struct {
	// Target is a BuildTarget alias accepted by `udit build player`:
	// win64 | win32 | mac | linux | android | ios | webgl, or the
	// full enum name (PS5, XboxSeries, Switch, etc.).
	Target string `yaml:"target"`

	// Output is the player output path. Relative paths land where the
	// caller typed `udit build`, matching the --output flag semantics.
	Output string `yaml:"output"`

	// Scenes overrides the Build Settings scene list. Empty slice ⇒
	// fall through to Build Settings (same as omitting --scenes).
	Scenes []string `yaml:"scenes"`

	// IL2CPP toggles the scripting backend. Connector captures the
	// previous backend, flips to IL2CPP, and restores it in finally.
	// Pointer so a preset can omit the flag to inherit the user's
	// current PlayerSettings.
	IL2CPP *bool `yaml:"il2cpp"`

	// Development adds BuildOptions.Development to the build flags
	// (debug symbols, script debugging enabled).
	Development *bool `yaml:"development"`
}

// buildCmd dispatches `udit build <subcommand>` to the manage_build tool.
// Four subcommands share the same tool: player / targets / addressables /
// cancel. The Go side just shapes the wire payload — Unity's BuildPipeline
// does the actual work.
//
// Build operations are long-running (player builds especially can run from
// 30 seconds to many minutes depending on platform + IL2CPP). The CLI
// dispatches with the dedicated buildSend wrapper in root.go that uses
// timeout=0 (infinite) so the agent doesn't hit the global 2-minute deadline.
func buildCmd(args []string, send sendFn) (*client.CommandResponse, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("usage: udit build <player|targets|addressables|cancel>")
	}

	action := args[0]
	flags := parseSubFlags(args[1:])

	params := map[string]interface{}{"action": action}

	switch action {
	case "targets", "cancel":
		// No flags, no positional. Keep the params map at just the action.

	case "player":
		// Layered merge: preset from .udit.yaml (if --config <name> is set)
		// supplies defaults; CLI flags override field-by-field.
		var preset *BuildPreset
		if presetName, ok := flags["config"]; ok && presetName != "" {
			p, err := resolveBuildPreset(presetName)
			if err != nil {
				return nil, err
			}
			preset = p
		}

		target := pickString(flags["target"], presetTarget(preset))
		if target == "" {
			return nil, fmt.Errorf("usage: udit build player --target <name> --output <dir> [--scenes a,b,c] [--development] [--il2cpp] [--config <preset>]")
		}
		out := pickString(flags["output"], presetOutput(preset))
		if out == "" {
			return nil, fmt.Errorf("--output is required for build player (provide via flag or preset `output:`)")
		}

		params["target"] = target
		// Match the convention from `test --output` and `screenshot
		// --output_path`: relative paths land where the caller typed the
		// command, not in Unity's project root.
		params["output"] = absolutizePath(out)

		if scenes, ok := flags["scenes"]; ok && scenes != "" {
			params["scenes"] = splitTrim(scenes, ",")
		} else if preset != nil && len(preset.Scenes) > 0 {
			params["scenes"] = preset.Scenes
		}

		if _, dev := flags["development"]; dev {
			params["development"] = true
		} else if _, explicitOff := flags["no-development"]; explicitOff {
			params["development"] = false
		} else if preset != nil && preset.Development != nil {
			params["development"] = *preset.Development
		}

		if _, il := flags["il2cpp"]; il {
			params["il2cpp"] = true
		} else if _, explicitOff := flags["no-il2cpp"]; explicitOff {
			params["il2cpp"] = false
		} else if preset != nil && preset.IL2CPP != nil {
			params["il2cpp"] = *preset.IL2CPP
		}

	case "addressables":
		if profile, ok := flags["profile"]; ok && profile != "" {
			params["profile"] = profile
		}

	default:
		return nil, fmt.Errorf("unknown build action: %s\nAvailable: player, targets, addressables, cancel", action)
	}

	return send("manage_build", params)
}

// splitTrim splits on sep, trims whitespace, drops empty entries. Used for
// `--scenes A, B ,C` style lists where humans may sprinkle whitespace.
func splitTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// resolveBuildPreset looks up a named entry under `build.targets:` in the
// user's .udit.yaml. Returns an error with actionable guidance when the
// config is missing or the preset name isn't registered — better than a
// silent null-preset fallback that would surprise the user with "missing
// --target" downstream.
func resolveBuildPreset(name string) (*BuildPreset, error) {
	if loadedConfig == nil {
		return nil, fmt.Errorf("--config %q requires a .udit.yaml — run `udit init --watch` first, or drop the flag", name)
	}
	if loadedConfig.Build.Targets == nil {
		return nil, fmt.Errorf("--config %q: no `build.targets:` section in .udit.yaml", name)
	}
	preset, ok := loadedConfig.Build.Targets[name]
	if !ok {
		available := make([]string, 0, len(loadedConfig.Build.Targets))
		for k := range loadedConfig.Build.Targets {
			available = append(available, k)
		}
		if len(available) == 0 {
			return nil, fmt.Errorf("--config %q: no build presets defined", name)
		}
		return nil, fmt.Errorf("--config %q: no such preset. Available: %s",
			name, strings.Join(available, ", "))
	}
	return &preset, nil
}

// pickString returns the first non-empty string from the candidates.
// Used for layered preset+flag resolution where an explicit CLI value
// should beat a preset default.
func pickString(candidates ...string) string {
	for _, s := range candidates {
		if s != "" {
			return s
		}
	}
	return ""
}

func presetTarget(p *BuildPreset) string {
	if p == nil {
		return ""
	}
	return p.Target
}

func presetOutput(p *BuildPreset) string {
	if p == nil {
		return ""
	}
	return p.Output
}
