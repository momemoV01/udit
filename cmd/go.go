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
		return nil, fmt.Errorf("usage: udit go <find|inspect|path>")
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

	default:
		return nil, fmt.Errorf("unknown go action: %s\nAvailable: find, inspect, path", action)
	}
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
