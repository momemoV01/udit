package cmd

import (
	"fmt"

	"github.com/momemoV01/udit/internal/client"
)

// prefabCmd dispatches `udit prefab <subcommand>` to the manage_prefab tool.
// instantiate and find-instances take an asset path; unpack and apply take a
// stable ID. Keeping them in one dispatcher mirrors the Unity menu grouping
// and avoids splitting prefab concepts across tools.
func prefabCmd(args []string, send sendFn) (*client.CommandResponse, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("usage: udit prefab <instantiate|unpack|apply|find-instances>")
	}

	action := args[0]
	rest := args[1:]
	flags := parseSubFlags(rest)
	positional := collectPositional(rest)

	switch action {
	case "instantiate":
		// `udit prefab instantiate <path> [--parent go:P] [--pos x,y,z] [--dry-run]`
		path := firstAssetPathPositional(positional)
		if path == "" {
			return nil, fmt.Errorf("usage: udit prefab instantiate <path> [--parent go:P] [--pos x,y,z] [--dry-run]")
		}
		params := map[string]interface{}{
			"action": "instantiate",
			"path":   path,
		}
		if v, ok := flags["parent"]; ok {
			params["parent"] = v
		}
		if v, ok := flags["pos"]; ok {
			params["pos"] = v
		}
		if _, dry := flags["dry-run"]; dry {
			params["dry_run"] = true
		}
		return send("manage_prefab", params)

	case "unpack":
		// `udit prefab unpack go:XXX [--mode root|completely] [--dry-run]`
		id := firstStableIdIn(positional)
		if id == "" {
			return nil, fmt.Errorf("usage: udit prefab unpack go:XXXXXXXX [--mode root|completely] [--dry-run]")
		}
		params := map[string]interface{}{
			"action": "unpack",
			"id":     id,
		}
		if v, ok := flags["mode"]; ok {
			params["mode"] = v
		}
		if _, dry := flags["dry-run"]; dry {
			params["dry_run"] = true
		}
		return send("manage_prefab", params)

	case "apply":
		// `udit prefab apply go:XXX [--dry-run]`
		id := firstStableIdIn(positional)
		if id == "" {
			return nil, fmt.Errorf("usage: udit prefab apply go:XXXXXXXX [--dry-run]")
		}
		params := map[string]interface{}{
			"action": "apply",
			"id":     id,
		}
		if _, dry := flags["dry-run"]; dry {
			params["dry_run"] = true
		}
		return send("manage_prefab", params)

	case "find-instances", "find_instances":
		// Accept both spellings — dash matches the rest of the CLI surface
		// (e.g. `component get --dry-run`), underscore matches the C# action
		// name. We normalize to the dashed form for the CLI, but dispatch to
		// the underscored server-side action key.
		path := firstAssetPathPositional(positional)
		if path == "" {
			return nil, fmt.Errorf("usage: udit prefab find-instances <path>")
		}
		return send("manage_prefab", map[string]interface{}{
			"action": "find_instances",
			"path":   path,
		})

	default:
		return nil, fmt.Errorf("unknown prefab action: %s\nAvailable: instantiate, unpack, apply, find-instances", action)
	}
}
