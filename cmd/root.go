package cmd

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/momemoV01/udit/internal/client"
)

var Version = "dev"

var (
	flagPort    int
	flagProject string
	flagTimeout int
	flagJSON    bool
)

func Execute() error {
	flag.IntVar(&flagPort, "port", 0, "Override Unity instance port")
	flag.StringVar(&flagProject, "project", "", "Select Unity instance by project path")
	flag.IntVar(&flagTimeout, "timeout", 120000, "Request timeout in milliseconds")
	flag.BoolVar(&flagJSON, "json", false, "Emit machine-readable JSON envelope to stdout/stderr")

	flag.Usage = func() { printHelp() }

	args := os.Args[1:]
	flagArgs, cmdArgs := splitArgs(args)
	if err := flag.CommandLine.Parse(flagArgs); err != nil {
		fmt.Fprintf(os.Stderr, "flag parse error: %v\n", err)
		os.Exit(1)
	}

	// Load .udit.yaml (walk up from cwd). Apply only fields that the user
	// did NOT set on the CLI, so explicit flags always win.
	if cwd, err := os.Getwd(); err == nil {
		if cfg, _ := LoadConfig(cwd); cfg != nil {
			applyConfig(cfg)
		}
	}

	if len(cmdArgs) == 0 {
		printHelp()
		return nil
	}

	category := cmdArgs[0]
	subArgs := cmdArgs[1:]

	// --help / -h on any command
	for _, a := range subArgs {
		if a == "--help" || a == "-h" {
			printTopicHelp(category)
			return nil
		}
	}

	switch category {
	case "help", "--help", "-h":
		if len(subArgs) > 0 {
			printTopicHelp(subArgs[0])
		} else {
			printHelp()
		}
		return nil
	case "version", "--version", "-v":
		fmt.Println("udit " + Version)
		return nil
	case "update":
		return updateCmd(subArgs)
	case "completion":
		return completionCmd(subArgs)
	case "status":
		inst, err := client.DiscoverInstance(flagProject, flagPort)
		if err != nil {
			reportError(err, "status", nil, flagJSON)
			os.Exit(1)
		}
		statusErr := statusCmd(inst, flagJSON)
		printUpdateNotice()
		if statusErr != nil {
			reportError(statusErr, "status", inst, flagJSON)
			os.Exit(1)
		}
		return nil
	}

	inst, err := client.DiscoverInstance(flagProject, flagPort)
	if err != nil {
		reportError(err, category, nil, flagJSON)
		os.Exit(1)
	}

	if err := waitForAlive(inst.Port, flagTimeout); err != nil {
		reportError(err, category, inst, flagJSON)
		os.Exit(1)
	}

	timeout := flagTimeout
	send := func(command string, params interface{}) (*client.CommandResponse, error) {
		return client.Send(inst, command, params, timeout)
	}

	var resp *client.CommandResponse

	switch category {
	case "editor":
		resp, err = editorCmd(subArgs, send, inst.Port)
	case "scene":
		resp, err = sceneCmd(subArgs, send)
	case "go":
		resp, err = goCmd(subArgs, send)
	case "component":
		resp, err = componentCmd(subArgs, send)
	case "test":
		testSend := func(command string, params interface{}) (*client.CommandResponse, error) {
			return client.Send(inst, command, params, 0)
		}
		resp, err = testCmd(subArgs, testSend, inst.Port)
	case "exec":
		subArgs = readStdinIfPiped(subArgs)
		var params map[string]interface{}
		params, err = buildParams(subArgs, nil)
		if err == nil {
			// Merge config-level default usings with whatever the call already
			// provided. CLI --usings (parsed by buildParams above) wins for
			// duplicates because it lands on top.
			params = mergeExecUsings(params, loadedConfig)
			resp, err = send("exec", params)
		}
	default:
		var params map[string]interface{}
		params, err = buildParams(subArgs, nil)
		if err == nil {
			resp, err = send(category, params)
		}
	}

	if err != nil {
		reportError(err, category, inst, flagJSON)
		os.Exit(1)
	}

	printResponse(resp, category, inst, flagJSON)

	printUpdateNotice()

	if !resp.Success {
		os.Exit(1)
	}

	return nil
}

// loadedConfig is set by Execute() once at startup so subcommand handlers
// (e.g. exec usings injection) can see project-wide settings without being
// passed an extra parameter through every call site.
var loadedConfig *Config

// applyConfig pushes config defaults into the global flag variables when the
// CLI did not override them. CLI flags > config > built-in defaults.
func applyConfig(cfg *Config) {
	loadedConfig = cfg
	if flagPort == 0 && cfg.DefaultPort != 0 {
		flagPort = cfg.DefaultPort
	}
	// 120000 is the built-in default for --timeout. Treat it as "unset" so
	// the config can replace it; an explicit `--timeout 120000` is
	// indistinguishable but harmless (same value).
	if flagTimeout == 120000 && cfg.DefaultTimeoutMs != 0 {
		flagTimeout = cfg.DefaultTimeoutMs
	}
}

// sendFn is the function signature for sending a command to Unity.
// Injected into each command function so they can be tested without a real Unity connection.
type sendFn func(command string, params interface{}) (*client.CommandResponse, error)

// printResponse renders a CommandResponse to stdout/stderr.
//
//	useJSON=true: uniform jsonOutput envelope (see cmd/output.go)
//	useJSON=false: legacy text — pretty-printed data on success, "Error: ..."
//	               on failure (preserves newlines for tree-style output)
func printResponse(resp *client.CommandResponse, command string, inst *client.Instance, useJSON bool) {
	if useJSON {
		emitJSONResponse(resp, command, inst)
		return
	}

	if !resp.Success {
		msg := resp.Message
		if msg == "" {
			msg = "unknown error"
		}
		if len(resp.Data) > 0 && string(resp.Data) != "null" {
			fmt.Fprintf(os.Stderr, "Error: %s\nDetails: %s\n", msg, string(resp.Data))
		} else {
			fmt.Fprintf(os.Stderr, "Error: %s\n", msg)
		}
		return
	}

	if len(resp.Data) > 0 && string(resp.Data) != "null" {
		var pretty interface{}
		if json.Unmarshal(resp.Data, &pretty) == nil {
			// If data is a plain string, print it raw (preserves newlines for tree output etc.)
			if s, ok := pretty.(string); ok {
				fmt.Println(s)
			} else {
				b, _ := json.MarshalIndent(pretty, "", "  ")
				fmt.Println(string(b))
			}
		} else {
			fmt.Println(string(resp.Data))
		}
	} else if resp.Message != "" {
		fmt.Println(resp.Message)
	}
}

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

func printHelp() {
	fmt.Print(`udit ` + Version + ` — Control Unity Editor from the command line

Usage: udit <command> [subcommand] [options]

Editor Control:
  editor play [--wait]          Enter play mode (--wait blocks until fully entered)
  editor stop                   Exit play mode
  editor pause                  Toggle pause/resume (play mode only)
  editor refresh                Refresh asset database
  editor refresh --compile      Recompile scripts and wait until done

Scene:
  scene list                    List every scene asset in the project
  scene active                  Describe the currently active scene
  scene open <path> [--force]   Open a scene (use --force to discard unsaved changes)
  scene save                    Save all open scenes that are dirty
  scene reload [--force]        Reload the active scene (use --force to discard changes)
  scene tree [--depth N]        Dump active scene hierarchy as a JSON tree with go: IDs
  scene tree --active-only      Skip inactive GameObjects

GameObjects:
  go find [--name PAT] [--tag T] [--component C]   Query GameObjects; returns go: IDs
  go find --limit 50 --offset 0                    Paginate (default limit 100)
  go inspect go:XXXXXXXX                           Dump components + serialized values
  go path go:XXXXXXXX                              Hierarchy path string (Root/Child/...)

Components:
  component list go:XXXXXXXX                       Enumerate components on a GameObject
  component get go:XXXXXXXX <Type> [field]         Dump one component, optionally one field
  component get go:XXXXXXXX <Type> --index N       Pick Nth instance when multiple attached
  component schema <Type>                          Type schema (requires a live instance)

Console:
  console                       Read error & warning logs (default)
  console --lines 20            Limit to N entries
  console --type error,warning,log   Filter by log types (comma-separated)
  console --stacktrace full     Stack trace: none, user (default), full
  console --clear               Clear console

Execute C#:
  exec "<code>"                 Run C# code in Unity (return required for output)
  echo '<code>' | exec          Pipe code via stdin (avoids shell escaping)
  exec "<code>" --usings x,y    Add extra using directives

  Examples:
    exec "Time.time"
    exec "GameObject.FindObjectsOfType<Camera>().Length"
    exec "var go = new GameObject(\"Test\"); return go.name;"

Menu:
  menu "<path>"                 Execute Unity menu item by path

  Examples:
    menu "File/Save Project"
    menu "Assets/Refresh"

Screenshot:
  screenshot                          Capture scene view (default)
  screenshot --view game              Capture game view
  screenshot --output_path <path>     Custom output path

Reserialize:
  reserialize [path...]          Force reserialize (no args = entire project)

  Examples:
    reserialize                                                    Reserialize entire project
    reserialize Assets/Scenes/Main.unity
    reserialize Assets/Prefabs/A.prefab Assets/Prefabs/B.prefab

Tests:
  test                            Run EditMode tests (default)
  test --mode PlayMode            Run PlayMode tests
  test --filter <name>            Filter by namespace, class, or full test name

Profiler:
  profiler hierarchy              Top-level profiler samples (last frame)
  profiler hierarchy --depth 5    Recursive drill-down (0=unlimited)
  profiler hierarchy --root Name  Set root by name (substring match)
  profiler hierarchy --frames 30  Average over last 30 frames
  profiler hierarchy --parent 5   Drill into item by ID
  profiler hierarchy --min 0.5    Filter items below 0.5ms
  profiler hierarchy --sort self  Sort by self time
  profiler enable                Start profiler recording
  profiler disable               Stop profiler recording
  profiler status                Show profiler state
  profiler clear                 Clear all captured frames

Custom Tools:
  list                          List all registered tools with parameter schemas
  <name>                        Call a custom tool directly
  <name> --params '{"k":"v"}'   Call with JSON parameters

Status:
  status                        Show Unity Editor state (ready, compiling, etc.)

Update:
  update                        Update to the latest version
  update --check                Check for updates without installing

Completion:
  completion <shell>            Print shell completion script (bash, zsh,
                                powershell, fish). Source it or pipe to a
                                profile/completion file.

Global Options:
  --port <N>          Connect to specific Unity port (skip auto-discovery)
  --project <path>    Select Unity instance by project path
  --timeout <ms>      Request timeout in ms (default: 120000)
  --json              Emit machine-readable JSON envelope (success → stdout,
                      error → stderr) with stable error_code field.
                      See docs/ERROR_CODES.md for the UCI-xxx registry.

Use "udit <command> --help" for more information about a command.

Notes:
  - Unity must be open with the Connector package installed
  - Multiple Unity instances: use --port or --project to select
  - Custom tools: any [UditTool] class is auto-discovered
  - Run 'list' to see all available tools
`)
}

func printTopicHelp(topic string) {
	switch topic {
	case "editor":
		fmt.Print(`Usage: udit editor <play|stop|pause|refresh> [options]

Subcommands:
  play [--wait]       Enter play mode
                      --wait blocks until Unity fully enters play mode.
                      Without --wait, returns immediately after requesting.
  stop                Exit play mode. No effect if not playing.
  pause               Toggle pause. Only works during play mode.
  refresh             Refresh AssetDatabase (reimport changed assets).
    --compile         Recompile scripts and wait until compilation finishes.

Examples:
  udit editor play --wait
  udit editor stop
  udit editor refresh --compile
`)
	case "scene":
		fmt.Print(`Usage: udit scene <list|active|open|save|reload|tree> [options]

Subcommands:
  list                Enumerate every scene asset in the project, including
                      build-settings membership and build index.
  active              Describe the currently active scene (path, guid, dirty
                      state, root GameObject count, build index).
  open <path>         Open a scene asset as the single active scene.
    --force           Discard unsaved changes in the current scene first.
                      Without --force, the call fails when the scene is dirty.
  save                Save every open scene that is currently dirty. Reports
                      which scenes were actually written.
  reload              Re-open the active scene, discarding unsaved edits.
    --force           Required when the active scene has unsaved changes.
  tree                Dump the active scene hierarchy as a JSON tree. Each
                      node has { id (go:XXXXXXXX), name, active, components,
                      children }. The id is stable across Editor restarts and
                      can be passed to later commands (scene tree -> go inspect
                      once the go namespace ships).
    --depth N         Max recursion depth. 0 = roots only, any negative value
                      or omitted = unlimited. Default: unlimited.
    --active-only     Skip inactive GameObjects and their whole subtrees.
                      Default: include inactive (flagged via "active": false).

Examples:
  udit scene list
  udit scene active
  udit scene open Assets/Scenes/Main.unity
  udit scene open Assets/Scenes/Menu.unity --force
  udit scene save
  udit scene reload --force
  udit scene tree --depth 3
  udit scene tree --active-only --json

Notes:
  - Scene paths are project-relative (Assets/... or Packages/...).
  - Open / reload are blocked while Unity is in play mode.
  - Build index comes from Build Settings; -1 means the scene is not enabled.
  - tree response size grows with hierarchy — use --depth on large scenes.
`)
	case "go", "gameobject":
		fmt.Print(`Usage: udit go <find|inspect|path> [options]

Query GameObjects in the loaded scenes. All results are keyed by
go:XXXXXXXX stable IDs — the same format emitted by ` + "`udit scene tree`" + `,
and stable across Editor restarts.

Subcommands:
  find                        Search loaded scenes for GameObjects matching
                              every provided filter (AND). Returns compact
                              entries: { id, name, active, tag, layer, path }.
    --name <glob>             Name filter. '*' is a wildcard (case-insensitive).
    --tag <tag>               Exact tag match.
    --component <type>        Must have a component whose type name matches
                              (case-insensitive, exact).
    --active-only             Skip inactive GameObjects. Default: include.
    --limit <N>               Max results per page. Default: 100, max: 1000.
    --offset <N>              Skip first N matches. Default: 0.
  inspect <go:XXXXXXXX>       Full dump of one GameObject: scene, path,
                              parent_id, children_ids, and every component
                              with its serialized properties.
  path <go:XXXXXXXX>          Hierarchy path string ("Root/Child/Leaf").

Examples:
  udit go find
  udit go find --name "Enemy*" --tag Enemy
  udit go find --component Rigidbody --active-only --limit 20
  udit go inspect go:9598abb1
  udit go path go:9598abb1

Notes:
  - Find results are sorted by hierarchy path so paginated queries are
    deterministic across calls.
  - Unknown / stale IDs return UCI-042 GameObjectNotFound (see
    docs/ERROR_CODES.md). Run ` + "`go find`" + ` or ` + "`scene tree`" + ` first to
    seed the stable-ID registry when an ID is from a previous session.
  - inspect truncates arrays at 20 elements; use ` + "`component get`" + ` (coming
    in a later slice) for full array contents.
`)
	case "component":
		fmt.Print(`Usage: udit component <list|get|schema> [options]

Read component values and type schemas. Field names mirror what ` + "`go inspect`" + `
emits, so the same vocabulary works end-to-end: find a GameObject, inspect
it, zoom in on one field via ` + "`component get`" + `.

Subcommands:
  list <go:XXXXXXXX>
      Enumerate components on a GameObject. Each entry has
      { index, type, full_type, enabled }. Lighter than ` + "`go inspect`" + ` when
      you only need the attached types.

  get <go:XXXXXXXX> <TypeName> [field]
      Dump one component. Without a field, returns every visible property
      (same shape as the matching entry in ` + "`go inspect`" + `). With a dotted
      field path ("position", "position.x", "m_Cameras.elements.0"), returns
      just the leaf value.
      --index N     Pick the Nth component when multiple of the same type
                    are attached (e.g. two BoxColliders). Default 0.

  schema <TypeName>
      Emit the serialized-property schema for a component type. Requires a
      live instance of the type in the loaded scenes (v1 probes an existing
      instance rather than spawning one, because AddComponent has side
      effects like RequireComponent chains). Fields include
      { name, display_name, property_type, is_array, has_children }.

Type name matching:
  - Case-insensitive.
  - Unqualified short names resolve against UnityEngine.* first, so
    ` + "`Transform`" + ` and ` + "`UnityEngine.Transform`" + ` are equivalent.
  - For project-local types that shadow a UnityEngine name, pass the full
    namespace ("MyGame.Transform") to disambiguate.

Examples:
  udit component list go:9598abb1
  udit component get go:9598abb1 Transform
  udit component get go:9598abb1 Transform position
  udit component get go:9598abb1 Transform local_position.z
  udit component get go:abcd1234 BoxCollider --index 1
  udit component schema Rigidbody
  udit component schema MyGame.PlayerController

Notes:
  - Unknown go: IDs -> UCI-042 (see docs/ERROR_CODES.md).
  - Missing type, bad index, or schema-without-instance -> UCI-043.
  - A field path that does not exist -> UCI-011 with the list of top-level
    fields so the agent can pick a valid one.
`)
	case "console":
		fmt.Print(`Usage: udit console [options]

Read Unity console log entries.

Options:
  --lines <N>          Limit to N entries
  --type <types>       Comma-separated log types: error, warning, log (default: error,warning,log)
  --stacktrace <mode>  none: first line only
                        user: with stack trace, internal frames filtered (default)
                        full: raw message including all frames
  --clear              Clear console

Examples:
  udit console
  udit console --lines 20 --type error,warning,log
  udit console --stacktrace user
  udit console --type error --stacktrace full
  udit console --clear
`)
	case "exec":
		fmt.Print(`Usage: udit exec "<code>" [options]

Execute C# code inside Unity Editor. Full access to UnityEngine,
UnityEditor, and all loaded assemblies.

Use 'return' to get output. Add --usings for types outside default namespaces.

Options:
  --usings <ns1,ns2>   Add extra using directives
  --csc <path>         Path to csc compiler (csc.dll or csc.exe). Auto-detected if omitted.
  --dotnet <path>      Path to dotnet runtime. Auto-detected if omitted.

Default usings: System, System.Collections.Generic, System.IO, System.Linq,
  System.Reflection, System.Threading.Tasks, UnityEngine,
  UnityEngine.SceneManagement, UnityEditor, UnityEditor.SceneManagement,
  UnityEditorInternal

Examples:
  udit exec "return 1+1;"
  udit exec "return Application.dataPath;"
  echo 'return EditorSceneManager.GetActiveScene().name;' | udit exec
  echo 'Debug.Log("hello"); return null;' | udit exec
  udit exec "return World.All.Count;" --usings Unity.Entities

Stdin:
  Pipe code via stdin to avoid shell escaping issues.
  echo '<code>' | udit exec [--usings ns1,ns2]

Notes:
  - Use 'return' for output, 'return null;' for void operations
`)
	case "menu":
		fmt.Print(`Usage: udit menu "<path>"

Execute a Unity menu item by its path.

Examples:
  udit menu "File/Save Project"
  udit menu "Assets/Refresh"
  udit menu "Window/General/Console"

Note: File/Quit is blocked for safety.
`)
	case "screenshot":
		fmt.Print(`Usage: udit screenshot [options]

Capture a screenshot of the Unity editor.

Options:
  --view <mode>      scene (default), game
  --width <N>        Image width in pixels (default: 1920)
  --height <N>       Image height in pixels (default: 1080)
  --output_path <path>  Output path, absolute or relative to project root
                        (default: Screenshots/screenshot.png)

Examples:
  udit screenshot
  udit screenshot --view game
  udit screenshot --view scene --width 3840 --height 2160
  udit screenshot --output_path captures/my_scene.png
`)
	case "reserialize":
		fmt.Print(`Usage: udit reserialize [path...]

Force Unity to reserialize assets through its own YAML serializer.
Run after editing .prefab, .unity, .asset, or .mat files as text.
No arguments = reserialize the entire project.

Examples:
  udit reserialize
  udit reserialize Assets/Prefabs/Player.prefab
  udit reserialize Assets/Scenes/Main.unity Assets/Scenes/Lobby.unity
`)
	case "profiler":
		fmt.Print(`Usage: udit profiler <subcommand> [options]

Subcommands:
  hierarchy             Top-level profiler samples (last frame)
    --depth <N>         Recursive depth (0=unlimited, default: 1)
    --root <name>       Set root by name (substring match, searches full tree)
    --frames <N>        Average over last N frames (flat output, sorted by time)
    --from <N>          Start frame index for range average
    --to <N>            End frame index for range average
    --parent <ID>       Drill into item by ID
    --min <ms>          Filter items below threshold
    --sort <col>        Sort by: total (default), self, calls
    --max <N>           Max children per level (default: 30)
    --frame <N>         Specific frame index
    --thread <N>        Thread index (0=main)
  enable                Start profiler recording
  disable               Stop profiler recording
  status                Show profiler state
  clear                 Clear all captured frames

Examples:
  udit profiler hierarchy --depth 3
  udit profiler hierarchy --root SimulationSystem --depth 3
  udit profiler hierarchy --frames 30 --min 0.5 --sort self
  udit profiler enable
`)
	case "test":
		fmt.Print(`Usage: udit test [options]

Run Unity tests via the Test Runner API.

Options:
  --mode <EditMode|PlayMode>    Test mode (default: EditMode)
  --filter <name>               Filter by namespace, class, or full test name
                                Must be the full path (e.g. MyNamespace.MyClass)

EditMode tests hold the connection open and return results directly.
PlayMode tests return immediately and poll a results file (domain reload safe).

Requires the Unity Test Framework package (com.unity.test-framework).

Examples:
  udit test
  udit test --mode PlayMode
  udit test --filter MyNamespace.MyTests
  udit test --mode EditMode --filter MyNamespace.MyTests.SpecificTest
`)
	case "list":
		fmt.Print(`Usage: udit list

List all registered tools (built-in + custom) with parameter schemas.

Example:
  udit list
`)
	case "status":
		fmt.Print(`Usage: udit status

Show the current Unity Editor state: port, project path, version, PID.
Reports "not responding" if heartbeat is older than 3 seconds.

Example:
  udit status
`)
	case "update":
		fmt.Print(`Usage: udit update [options]

Update the CLI binary to the latest release from GitHub.

Options:
  --check              Check for updates without installing

Examples:
  udit update
  udit update --check
`)
	case "custom-tools", "custom", "tools":
		fmt.Print(`How to write custom tools for udit

Custom tools are C# classes that run inside Unity Editor. The CLI
discovers them automatically via reflection.

Create a static class with [UditTool] in any Editor assembly:

    using UditConnector;
    using Newtonsoft.Json.Linq;

    [UditTool(Description = "Spawn an enemy at a position")]
    public static class SpawnEnemy
    {
        public class Parameters
        {
            [ToolParameter("X world position", Required = true)]
            public float X { get; set; }
        }

        public static object HandleCommand(JObject parameters)
        {
            float x = parameters["x"]?.Value<float>() ?? 0;
            var go = Object.Instantiate(prefab, new Vector3(x, 0, 0), Quaternion.identity);
            return new SuccessResponse("Spawned", new { name = go.name });
        }
    }

Rules:
  - Class must be static
  - Must have: public static object HandleCommand(JObject parameters)
  - Return SuccessResponse(message, data) or ErrorResponse(message)
  - Add Parameters class with [ToolParameter] for discoverability
  - Class name auto-converts to snake_case (SpawnEnemy → spawn_enemy)
  - Override name: [UditTool(Name = "my_name")]
  - Runs on Unity main thread — all Unity APIs are safe
  - Discovered on Editor start and after every script recompilation
  - Duplicate tool names are detected and logged as errors (first wins)
`)
	case "setup", "install":
		fmt.Print(`Installation and Unity setup

CLI Installation:
  # Linux / macOS
  curl -fsSL https://raw.githubusercontent.com/momemoV01/udit/master/install.sh | sh

  # Windows (PowerShell)
  irm https://raw.githubusercontent.com/momemoV01/udit/master/install.ps1 | iex

  # Go install (any platform)
  go install github.com/momemoV01/udit@latest

Unity Setup:
  1. Window → Package Manager → + → Add package from git URL
  2. Paste: https://github.com/momemoV01/udit.git?path=udit-connector
  The Connector starts automatically when Unity opens.

Verify:
  udit list
`)
	default:
		fmt.Printf("Unknown help topic: %s\n\nUse \"udit --help\" for available commands.\n", topic)
	}
}
