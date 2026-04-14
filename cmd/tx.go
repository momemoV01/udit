package cmd

import (
	"fmt"

	"github.com/momemoV01/udit/internal/client"
)

// txCmd dispatches `udit tx <subcommand>` to the manage_transaction tool.
// Transactions group mutations into a single Unity Undo entry — one active
// transaction per Unity instance, state lives on the Connector side and is
// torn down with the domain on script reload.
func txCmd(args []string, send sendFn) (*client.CommandResponse, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("usage: udit tx <begin|commit|rollback|status>")
	}

	action := args[0]
	flags := parseSubFlags(args[1:])

	switch action {
	case "begin":
		params := map[string]interface{}{"action": "begin"}
		// --name sets the label visible in Edit → Undo. Optional — server
		// defaults to "udit transaction" if omitted.
		if v, ok := flags["name"]; ok {
			params["name"] = v
		}
		return send("manage_transaction", params)

	case "commit":
		params := map[string]interface{}{"action": "commit"}
		// --name at commit overrides the one given at begin. Handy when the
		// final description of the change only crystallises after the work
		// is done.
		if v, ok := flags["name"]; ok {
			params["name"] = v
		}
		return send("manage_transaction", params)

	case "rollback":
		return send("manage_transaction", map[string]interface{}{"action": "rollback"})

	case "status":
		return send("manage_transaction", map[string]interface{}{"action": "status"})

	default:
		return nil, fmt.Errorf("unknown tx action: %s\nAvailable: begin, commit, rollback, status", action)
	}
}
