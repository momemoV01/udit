package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/momemoV01/udit/internal/client"
)

// assetCmd dispatches `udit asset <subcommand>` to the manage_asset tool.
// Parallels sceneCmd / goCmd / componentCmd: positional args become named
// params, and integer flags are parsed (and rejected) up front so Unity never
// sees a string where it expects a number.
func assetCmd(args []string, send sendFn) (*client.CommandResponse, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("usage: udit asset <find|inspect|dependencies|references|guid|path>")
	}

	action := args[0]
	rest := args[1:]
	flags := parseSubFlags(rest)
	positional := collectPositional(rest)

	switch action {
	case "find":
		params := map[string]interface{}{"action": "find"}
		if v, ok := flags["type"]; ok {
			params["type"] = v
		}
		if v, ok := flags["label"]; ok {
			params["label"] = v
		}
		if v, ok := flags["name"]; ok {
			params["name"] = v
		}
		if v, ok := flags["folder"]; ok {
			params["folder"] = v
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
		return send("manage_asset", params)

	case "inspect":
		path := firstAssetPathPositional(positional)
		if path == "" {
			return nil, fmt.Errorf("usage: udit asset inspect <path>")
		}
		return send("manage_asset", map[string]interface{}{
			"action": "inspect",
			"path":   path,
		})

	case "dependencies":
		path := firstAssetPathPositional(positional)
		if path == "" {
			return nil, fmt.Errorf("usage: udit asset dependencies <path> [--recursive]")
		}
		params := map[string]interface{}{
			"action": "dependencies",
			"path":   path,
		}
		if _, recursive := flags["recursive"]; recursive {
			params["recursive"] = true
		}
		return send("manage_asset", params)

	case "references":
		path := firstAssetPathPositional(positional)
		if path == "" {
			return nil, fmt.Errorf("usage: udit asset references <path> [--limit N --offset M]")
		}
		params := map[string]interface{}{
			"action": "references",
			"path":   path,
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
		return send("manage_asset", params)

	case "guid":
		path := firstAssetPathPositional(positional)
		if path == "" {
			return nil, fmt.Errorf("usage: udit asset guid <path>")
		}
		return send("manage_asset", map[string]interface{}{
			"action": "guid",
			"path":   path,
		})

	case "path":
		// `asset path` takes a raw Unity GUID (32 hex chars, no dashes).
		// No go: / Assets/ prefix — so firstAssetPathPositional would miss
		// it. Accept the first positional that is not a flag value.
		if len(positional) == 0 {
			return nil, fmt.Errorf("usage: udit asset path <guid>")
		}
		return send("manage_asset", map[string]interface{}{
			"action": "path",
			"guid":   positional[0],
		})

	default:
		return nil, fmt.Errorf("unknown asset action: %s\nAvailable: find, inspect, dependencies, references, guid, path", action)
	}
}

// firstAssetPathPositional picks the first positional that looks like an
// asset path. Unity asset paths always start with "Assets/" or "Packages/"
// (the two root folders the AssetDatabase indexes), which lets us reject
// bare type names or GUIDs that a user might pass by mistake.
func firstAssetPathPositional(positional []string) string {
	for _, a := range positional {
		if strings.HasPrefix(a, "Assets/") || strings.HasPrefix(a, "Packages/") {
			return a
		}
	}
	// Fall back to the first positional. Letting the C# side emit a clearer
	// "not found" error message when the path is malformed gives a better
	// experience than a generic "missing path" here.
	if len(positional) > 0 {
		return positional[0]
	}
	return ""
}
