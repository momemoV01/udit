package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/momemoV01/udit/internal/client"
)

// componentCmd dispatches `udit component <subcommand>` to the manage_component
// tool. Parallels sceneCmd / goCmd: positional args become named params on the
// way into the JSON envelope so the C# side sees a consistent shape.
func componentCmd(args []string, send sendFn) (*client.CommandResponse, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("usage: udit component <list|get|schema>")
	}

	action := args[0]
	rest := args[1:]
	flags := parseSubFlags(rest)
	positional := collectPositional(rest)

	switch action {
	case "list":
		id := firstStableIdIn(positional)
		if id == "" {
			return nil, fmt.Errorf("usage: udit component list go:XXXXXXXX")
		}
		return send("manage_component", map[string]interface{}{
			"action": "list",
			"id":     id,
		})

	case "get":
		// Expected shape: component get <go:ID> <TypeName> [field] [--index N]
		id := firstStableIdIn(positional)
		if id == "" {
			return nil, fmt.Errorf("usage: udit component get go:XXXXXXXX <TypeName> [field] [--index N]")
		}
		typeName := firstNonIdPositional(positional, id)
		if typeName == "" {
			return nil, fmt.Errorf("component get: missing <TypeName> (after the go: id)")
		}
		field := nthNonIdPositional(positional, id, 1) // optional third positional

		params := map[string]interface{}{
			"action": "get",
			"id":     id,
			"type":   typeName,
		}
		if field != "" {
			params["field"] = field
		}
		if v, ok := flags["index"]; ok {
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("--index must be an integer, got %q", v)
			}
			params["index"] = n
		}
		return send("manage_component", params)

	case "schema":
		typeName := firstNonFlagNonId(positional)
		if typeName == "" {
			return nil, fmt.Errorf("usage: udit component schema <TypeName>")
		}
		return send("manage_component", map[string]interface{}{
			"action": "schema",
			"type":   typeName,
		})

	default:
		return nil, fmt.Errorf("unknown component action: %s\nAvailable: list, get, schema", action)
	}
}

// collectPositional extracts non-flag args, skipping known value flags the
// C# tool understands. We consider `--key value` pairs AND `--switch` cases,
// always dropping the flag (and its value when present) from the positional
// list. This way `component get go:abc --index 2 Transform position` still
// yields ["go:abc", "Transform", "position"].
func collectPositional(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "--") {
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				i++ // consume the value
			}
			continue
		}
		out = append(out, a)
	}
	return out
}

// firstStableIdIn returns the first "go:" token, or "".
func firstStableIdIn(positional []string) string {
	for _, a := range positional {
		if strings.HasPrefix(a, "go:") {
			return a
		}
	}
	return ""
}

// firstNonIdPositional returns the first positional that is not the given
// stable ID. Used for picking <TypeName> when the user might have typed
// arguments in either order (`component get <id> Transform` or `component get
// Transform <id>` — though we discourage the latter, it costs nothing to
// support).
func firstNonIdPositional(positional []string, id string) string {
	for _, a := range positional {
		if a == id {
			continue
		}
		return a
	}
	return ""
}

// nthNonIdPositional returns the n-th (zero-based) positional that is not the
// stable id. n=0 is the first (same as firstNonIdPositional), n=1 is the
// second, etc. Returns "" if there are not enough positionals.
func nthNonIdPositional(positional []string, id string, n int) string {
	seen := 0
	for _, a := range positional {
		if a == id {
			continue
		}
		if seen == n {
			return a
		}
		seen++
	}
	return ""
}

// firstNonFlagNonId returns the first positional that is not a go: ID. Used
// by `component schema` which takes a bare type name and no id.
func firstNonFlagNonId(positional []string) string {
	for _, a := range positional {
		if strings.HasPrefix(a, "go:") {
			continue
		}
		return a
	}
	return ""
}
