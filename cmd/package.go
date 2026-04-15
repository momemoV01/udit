package cmd

import (
	"fmt"
	"strings"

	"github.com/momemoV01/udit/internal/client"
)

// packageCmd dispatches `udit package <subcommand>` to the manage_package
// tool. Six subcommands share the same tool: list / add / remove / info /
// search / resolve. The Go side just shapes the wire payload — Unity's
// PackageManager.Client owns all the registry semantics and async polling.
func packageCmd(args []string, send sendFn) (*client.CommandResponse, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("usage: udit package <list|add|remove|info|search|resolve>")
	}

	action := args[0]
	flags := parseSubFlags(args[1:])
	positional := packageFirstPositional(args[1:])

	params := map[string]interface{}{"action": action}

	switch action {
	case "list":
		// Default lists declared deps from manifest.json (sub-second).
		// --resolved switches to PackageManager.Client.List which walks the
		// resolved graph (transitive deps + source kind), 1-3s on a cold
		// registry.
		if _, ok := flags["resolved"]; ok {
			params["resolved"] = true
		}

	case "resolve":
		// No positional, no flags.

	case "add", "remove", "info":
		if positional == "" {
			return nil, fmt.Errorf("usage: udit package %s <name>", action)
		}
		params["name"] = positional

	case "search":
		if positional == "" {
			return nil, fmt.Errorf("usage: udit package search <keyword>")
		}
		params["query"] = positional

	default:
		return nil, fmt.Errorf("unknown package action: %s\nAvailable: list, add, remove, info, search, resolve", action)
	}

	return send("manage_package", params)
}

// packageFirstPositional returns the first non-flag argument from args,
// mirroring the way parseSubFlags consumes flag values (so a `--key value`
// pair won't be mistaken for a positional). The package commands take at
// most one positional, so a single string return is sufficient.
//
// Distinct from scene.go's firstPositional, which intentionally does NOT
// skip value-flag values — scene only ever passes the boolean --force, so
// it can stay simple. Merging the two would require teaching scene's
// helper about which flags take values; not worth the coupling for now.
func packageFirstPositional(args []string) string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "--") {
			// Skip the value of `--key value`. parseSubFlags treats next
			// arg as a value when it doesn't start with `--`, so we mirror
			// that to stay consistent.
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				i++
			}
			continue
		}
		return a
	}
	return ""
}
