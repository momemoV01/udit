package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/momemoV01/udit/internal/client"
)

// sendFn is the function signature for sending a command to Unity.
// Injected into each command function so they can be tested without a real Unity connection.
type sendFn func(command string, params interface{}) (*client.CommandResponse, error)

// parseSubFlags parses --key value and --flag (boolean) pairs from subcommand args.
// Non-flag args (no "--" prefix) are silently ignored.
func parseSubFlags(args []string) map[string]string {
	flags := map[string]string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "--") {
			key := a[2:]
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				flags[key] = args[i+1]
				i++
			} else {
				flags[key] = "true"
			}
		}
	}
	return flags
}

// buildParams parses CLI args into a params map.
//
// Flag forms:
//
//	--key value   → string (or int if value parses as integer)
//	--key         → boolean true (a "switch" — no value follows)
//
// Distinguishing "switch" from "value flag" is critical: previously,
// `--filter true` was parsed as bool true because the value happened to
// match the literal "true". Now switches and value flags are tracked
// separately so a string value like "true" stays a string.
//
// Positional args (no -- prefix) are collected into params["args"].
// --params <json> overrides everything; remaining flags merge on top
// without clobbering values already present in base or in --params.
func buildParams(args []string, base map[string]interface{}) (map[string]interface{}, error) {
	params := map[string]interface{}{}
	for k, v := range base {
		params[k] = v
	}

	var positional []string
	valFlags := map[string]string{}  // --key value
	switchFlags := map[string]bool{} // --key (no value)
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "--") {
			key := a[2:]
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				valFlags[key] = args[i+1]
				i++
			} else {
				switchFlags[key] = true
			}
		} else {
			positional = append(positional, a)
		}
	}

	if raw, ok := valFlags["params"]; ok {
		if jsonErr := json.Unmarshal([]byte(raw), &params); jsonErr != nil {
			return nil, fmt.Errorf("invalid JSON in --params: %w", jsonErr)
		}
	}

	for k, v := range valFlags {
		if k == "params" {
			continue
		}
		if _, exists := params[k]; exists {
			continue
		}
		// Try int parse, otherwise keep as string. No bool coercion here —
		// switch-style booleans are handled below from switchFlags.
		if n, err := strconv.Atoi(v); err == nil {
			params[k] = n
		} else {
			params[k] = v
		}
	}

	for k := range switchFlags {
		if _, exists := params[k]; exists {
			continue
		}
		params[k] = true
	}

	if len(positional) > 0 {
		params["args"] = positional
	}

	return params, nil
}

// readStdinIfPiped reads stdin when piped and prepends it as the first positional arg.
func readStdinIfPiped(args []string) []string {
	info, err := os.Stdin.Stat()
	if err != nil {
		return args
	}
	if info.Mode()&os.ModeCharDevice != 0 {
		return args // interactive terminal, not piped
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil || len(data) == 0 {
		return args
	}
	code := strings.TrimRight(string(data), "\n\r")
	return append([]string{code}, args...)
}

// splitArgs separates global flags from subcommand args.
//
//	Value flags  : --port N, --project PATH, --timeout MS  (consume next arg)
//	Switch flags : --json                                  (no value)
//
// Anything else (including unknown flags, subcommand-local flags, and
// positional args) goes through to the subcommand. flag.CommandLine then
// parses just the global slice.
func splitArgs(args []string) (flags, commands []string) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--port", "--project", "--timeout":
			flags = append(flags, args[i])
			if i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		case "--json":
			flags = append(flags, args[i])
		default:
			commands = append(commands, args[i])
		}
	}
	return
}
