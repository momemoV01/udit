package cmd

import (
	"fmt"
	"strconv"

	"github.com/momemoV01/udit/internal/client"
)

// projectCmd dispatches `udit project <subcommand>` to the manage_project
// tool. Three subcommands share the same tool — info is a pure read,
// validate is a project-wide scan, preflight is validate + build-readiness.
func projectCmd(args []string, send sendFn) (*client.CommandResponse, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("usage: udit project <info|validate|preflight>")
	}

	action := args[0]
	flags := parseSubFlags(args[1:])

	switch action {
	case "info":
		return send("manage_project", map[string]interface{}{"action": "info"})

	case "validate", "preflight":
		params := map[string]interface{}{"action": action}
		// --include-packages widens the scan to Packages/. Agents default to
		// Assets-only because most validate use cases care about project-
		// authored content, and Packages/ scans are slower + usually clean.
		if _, inclPkgs := flags["include-packages"]; inclPkgs {
			params["assets_only"] = false
		}
		if v, ok := flags["limit"]; ok {
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("--limit must be an integer, got %q", v)
			}
			params["limit"] = n
		}
		return send("manage_project", params)

	default:
		return nil, fmt.Errorf("unknown project action: %s\nAvailable: info, validate, preflight", action)
	}
}
