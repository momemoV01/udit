package cmd

import (
	"fmt"
	"strings"

	"github.com/momemoV01/udit/internal/client"
)

// sceneCmd dispatches `udit scene <subcommand>` to the manage_scene tool.
// All actions round-trip through the same tool; per-action validation
// (e.g. "path required for open") lives on the C# side so this handler
// stays a thin pass-through.
func sceneCmd(args []string, send sendFn) (*client.CommandResponse, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("usage: udit scene <list|active|open|save|reload>")
	}

	action := args[0]
	rest := args[1:]
	flags := parseSubFlags(rest)

	switch action {
	case "list":
		return send("manage_scene", map[string]interface{}{"action": "list"})

	case "active":
		return send("manage_scene", map[string]interface{}{"action": "active"})

	case "open":
		path := firstPositional(rest)
		if path == "" {
			return nil, fmt.Errorf("usage: udit scene open <path> [--force]")
		}
		_, force := flags["force"]
		return send("manage_scene", map[string]interface{}{
			"action": "open",
			"path":   path,
			"force":  force,
		})

	case "save":
		return send("manage_scene", map[string]interface{}{"action": "save"})

	case "reload":
		_, force := flags["force"]
		return send("manage_scene", map[string]interface{}{
			"action": "reload",
			"force":  force,
		})

	default:
		return nil, fmt.Errorf("unknown scene action: %s\nAvailable: list, active, open, save, reload", action)
	}
}

// firstPositional returns the first non-flag token, or "".
// Kept local to scene.go because the existing parseSubFlags intentionally
// drops positional args — we need the positional for `scene open <path>`.
func firstPositional(args []string) string {
	skipNext := false
	for _, a := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if strings.HasPrefix(a, "--") {
			// This may or may not consume a value; we don't know which flags
			// take values here, so conservatively skip nothing — the caller
			// invokes this only for `scene open`, whose only flag is --force
			// (a switch, no value). If we grow value flags here later we will
			// need to teach firstPositional about them.
			continue
		}
		return a
	}
	return ""
}
