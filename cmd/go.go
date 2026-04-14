package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/momemoV01/udit/internal/client"
)

// goCmd dispatches `udit go <subcommand>` to the manage_game_object tool.
// Parallels sceneCmd: thin pass-through that translates CLI surface
// (positional go: IDs, flags) into the tool's JSON parameters. Per-action
// validation lives on the C# side so Unity sees the same error handling
// regardless of client.
func goCmd(args []string, send sendFn) (*client.CommandResponse, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("usage: udit go <find|inspect|path|create|destroy|move|rename|setactive>")
	}

	action := args[0]
	rest := args[1:]
	flags := parseSubFlags(rest)

	switch action {
	case "find":
		params := map[string]interface{}{"action": "find"}
		if v, ok := flags["name"]; ok {
			params["name"] = v
		}
		if v, ok := flags["tag"]; ok {
			params["tag"] = v
		}
		if v, ok := flags["component"]; ok {
			params["component"] = v
		}
		if _, activeOnly := flags["active-only"]; activeOnly {
			params["include_inactive"] = false
		}
		if v, ok := flags["limit"]; ok {
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("--limit must be an integer, got %q", v)
			}
			params["limit"] = n
		}
		if v, ok := flags["offset"]; ok {
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("--offset must be an integer, got %q", v)
			}
			params["offset"] = n
		}
		return send("manage_game_object", params)

	case "inspect":
		id := firstStableId(rest)
		if id == "" {
			return nil, fmt.Errorf("usage: udit go inspect go:XXXXXXXX")
		}
		return send("manage_game_object", map[string]interface{}{
			"action": "inspect",
			"id":     id,
		})

	case "path":
		id := firstStableId(rest)
		if id == "" {
			return nil, fmt.Errorf("usage: udit go path go:XXXXXXXX")
		}
		return send("manage_game_object", map[string]interface{}{
			"action": "path",
			"id":     id,
		})

	case "create":
		// `udit go create --name X [--parent go:Y] [--pos x,y,z] [--dry-run]`
		params := map[string]interface{}{"action": "create"}
		if v, ok := flags["name"]; ok {
			params["name"] = v
		} else {
			return nil, fmt.Errorf("usage: udit go create --name <name> [--parent go:XXX] [--pos x,y,z] [--dry-run]")
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
		return send("manage_game_object", params)

	case "destroy":
		id := firstStableId(rest)
		if id == "" {
			return nil, fmt.Errorf("usage: udit go destroy go:XXXXXXXX [--dry-run]")
		}
		params := map[string]interface{}{"action": "destroy", "id": id}
		if _, dry := flags["dry-run"]; dry {
			params["dry_run"] = true
		}
		return send("manage_game_object", params)

	case "move":
		id := firstStableId(rest)
		if id == "" {
			return nil, fmt.Errorf("usage: udit go move go:XXXXXXXX [--parent go:YYY] [--dry-run]")
		}
		params := map[string]interface{}{"action": "move", "id": id}
		if v, ok := flags["parent"]; ok {
			params["parent"] = v
		}
		// Omitting --parent is intentional: the C# side treats absent parent
		// as "move to scene root", which is the most common use of this flag
		// in the destroy/cleanup workflows we want to support.
		if _, dry := flags["dry-run"]; dry {
			params["dry_run"] = true
		}
		return send("manage_game_object", params)

	case "rename":
		// `udit go rename go:XXX <newname> [--dry-run]`
		id := firstStableId(rest)
		if id == "" {
			return nil, fmt.Errorf("usage: udit go rename go:XXXXXXXX <newname> [--dry-run]")
		}
		newName := firstNonStableIdPositional(rest, id)
		if newName == "" {
			return nil, fmt.Errorf("rename: missing <newname> (after the go: id)")
		}
		params := map[string]interface{}{
			"action":   "rename",
			"id":       id,
			"new_name": newName,
		}
		if _, dry := flags["dry-run"]; dry {
			params["dry_run"] = true
		}
		return send("manage_game_object", params)

	case "setactive":
		// `udit go setactive go:XXX --active true|false [--dry-run]`
		id := firstStableId(rest)
		if id == "" {
			return nil, fmt.Errorf("usage: udit go setactive go:XXXXXXXX --active true|false [--dry-run]")
		}
		v, ok := flags["active"]
		if !ok {
			return nil, fmt.Errorf("setactive: --active true|false is required")
		}
		// Accept the common spellings; reject everything else with a clear
		// message instead of silently coercing to false.
		var b bool
		switch strings.ToLower(v) {
		case "true", "1", "yes", "on":
			b = true
		case "false", "0", "no", "off":
			b = false
		default:
			return nil, fmt.Errorf("setactive: --active must be true or false, got %q", v)
		}
		params := map[string]interface{}{
			"action": "setactive",
			"id":     id,
			"active": b,
		}
		if _, dry := flags["dry-run"]; dry {
			params["dry_run"] = true
		}
		return send("manage_game_object", params)

	default:
		return nil, fmt.Errorf("unknown go action: %s\nAvailable: find, inspect, path, create, destroy, move, rename, setactive", action)
	}
}

// firstNonStableIdPositional returns the first positional that is not the
// supplied stable ID and not a flag name/value. Used by rename which expects
// `go rename <id> <newname>`.
func firstNonStableIdPositional(args []string, id string) string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "--") {
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				i++
			}
			continue
		}
		if a == id {
			continue
		}
		return a
	}
	return ""
}

// firstStableId returns the first positional argument that looks like a
// stable ID (go:XXXXXXXX). We do not parse a flag's value by accident —
// e.g. `go inspect go:abcd1234 --extra foo` still picks the positional.
// If no positional ID is present, returns "".
func firstStableId(args []string) string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "--") {
			// Conservatively skip a potential flag value: if the next token is
			// not itself a flag, it is a value and we advance past it.
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				i++
			}
			continue
		}
		if strings.HasPrefix(a, "go:") {
			return a
		}
	}
	return ""
}
