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
		return nil, fmt.Errorf("usage: udit component <list|get|schema|add|remove|set|copy>")
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

	case "add":
		// `udit component add go:XXX --type T [--dry-run]`
		id := firstStableIdIn(positional)
		if id == "" {
			return nil, fmt.Errorf("usage: udit component add go:XXXXXXXX --type <TypeName> [--dry-run]")
		}
		typeName, ok := flags["type"]
		if !ok {
			return nil, fmt.Errorf("component add: --type <TypeName> is required")
		}
		params := map[string]interface{}{
			"action": "add",
			"id":     id,
			"type":   typeName,
		}
		if _, dry := flags["dry-run"]; dry {
			params["dry_run"] = true
		}
		return send("manage_component", params)

	case "remove":
		// `udit component remove go:XXX <TypeName> [--index N] [--dry-run]`
		id := firstStableIdIn(positional)
		if id == "" {
			return nil, fmt.Errorf("usage: udit component remove go:XXXXXXXX <TypeName> [--index N] [--dry-run]")
		}
		typeName := firstNonIdPositional(positional, id)
		if typeName == "" {
			return nil, fmt.Errorf("component remove: missing <TypeName> (after the go: id)")
		}
		params := map[string]interface{}{
			"action": "remove",
			"id":     id,
			"type":   typeName,
		}
		if v, ok := flags["index"]; ok {
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("--index must be an integer, got %q", v)
			}
			params["index"] = n
		}
		if _, dry := flags["dry-run"]; dry {
			params["dry_run"] = true
		}
		return send("manage_component", params)

	case "set":
		// `udit component set go:XXX <TypeName> <field> <value> [--index N] [--dry-run]`
		// Value can contain spaces if the shell passed one token (quoted), or
		// be the last positional. We take positionals in order: id/typeName/
		// field/value after skipping the go: id wherever it appeared.
		id := firstStableIdIn(positional)
		if id == "" {
			return nil, fmt.Errorf("usage: udit component set go:XXXXXXXX <TypeName> <field> <value> [--index N] [--dry-run]")
		}
		typeName := nthNonIdPositional(positional, id, 0)
		field := nthNonIdPositional(positional, id, 1)
		value := nthNonIdPositional(positional, id, 2)
		if typeName == "" || field == "" || value == "" {
			return nil, fmt.Errorf("component set: expected <TypeName> <field> <value> after the go: id")
		}
		params := map[string]interface{}{
			"action": "set",
			"id":     id,
			"type":   typeName,
			"field":  field,
			"value":  value,
		}
		if v, ok := flags["index"]; ok {
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("--index must be an integer, got %q", v)
			}
			params["index"] = n
		}
		if _, dry := flags["dry-run"]; dry {
			params["dry_run"] = true
		}
		return send("manage_component", params)

	case "copy":
		// `udit component copy go:SRC <TypeName> go:DST [--index N] [--dry-run]`
		// The two go: IDs are distinguishable by position — the first one
		// after "copy" is the source, the second is the destination. Agents
		// sometimes flip those, so we disambiguate by sequence rather than
		// flag prefix.
		srcId := ""
		dstId := ""
		for _, a := range positional {
			if strings.HasPrefix(a, "go:") {
				if srcId == "" {
					srcId = a
				} else if dstId == "" {
					dstId = a
				}
			}
		}
		if srcId == "" || dstId == "" {
			return nil, fmt.Errorf("usage: udit component copy go:SRC <TypeName> go:DST [--index N] [--dry-run]")
		}
		typeName := ""
		for _, a := range positional {
			if a == srcId || a == dstId {
				continue
			}
			typeName = a
			break
		}
		if typeName == "" {
			return nil, fmt.Errorf("component copy: missing <TypeName> between the two go: ids")
		}
		params := map[string]interface{}{
			"action": "copy",
			"id":     srcId,
			"type":   typeName,
			"dst_id": dstId,
		}
		if v, ok := flags["index"]; ok {
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("--index must be an integer, got %q", v)
			}
			params["index"] = n
		}
		if _, dry := flags["dry-run"]; dry {
			params["dry_run"] = true
		}
		return send("manage_component", params)

	default:
		return nil, fmt.Errorf("unknown component action: %s\nAvailable: list, get, schema, add, remove, set, copy", action)
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
