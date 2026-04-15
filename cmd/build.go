package cmd

import (
	"fmt"
	"strings"

	"github.com/momemoV01/udit/internal/client"
)

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
		target, ok := flags["target"]
		if !ok || target == "" {
			return nil, fmt.Errorf("usage: udit build player --target <name> --output <dir> [--scenes a,b,c] [--development]")
		}
		out, ok := flags["output"]
		if !ok || out == "" {
			return nil, fmt.Errorf("--output is required for build player")
		}
		params["target"] = target
		// Match the convention from `test --output` and `screenshot
		// --output_path`: relative paths land where the caller typed the
		// command, not in Unity's project root.
		params["output"] = absolutizePath(out)
		if scenes, ok := flags["scenes"]; ok && scenes != "" {
			params["scenes"] = splitTrim(scenes, ",")
		}
		if _, dev := flags["development"]; dev {
			params["development"] = true
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
