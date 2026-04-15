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
	case "asset":
		resp, err = assetCmd(subArgs, send)
	case "prefab":
		resp, err = prefabCmd(subArgs, send)
	case "tx":
		resp, err = txCmd(subArgs, send)
	case "project":
		resp, err = projectCmd(subArgs, send)
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
  go create --name N [--parent go:P] [--pos x,y,z]  Spawn a GameObject (Undo-safe)
  go destroy go:XXXXXXXX                           Destroy a GameObject + descendants
  go move go:XXX [--parent go:YYY]                 Reparent (omit --parent for scene root)
  go rename go:XXX <newname>                       Rename a GameObject
  go setactive go:XXX --active true|false          Toggle active state
  (any mutation accepts --dry-run to preview without changing the scene)

Components:
  component list go:XXXXXXXX                       Enumerate components on a GameObject
  component get go:XXXXXXXX <Type> [field]         Dump one component, optionally one field
  component get go:XXXXXXXX <Type> --index N       Pick Nth instance when multiple attached
  component schema <Type>                          Type schema (requires a live instance)
  component add go:XXXXXXXX --type <Type>          Add a component (Undo-safe)
  component remove go:XXXXXXXX <Type> [--index N]  Remove a component
  component set go:XXX <Type> <field> <value>      Write one field (Undo-safe)
  component copy go:SRC <Type> go:DST              Copy a component between GameObjects
  (mutations accept --dry-run)

Assets:
  asset find [--type Prefab] [--label X] [--name G] [--folder F]   Query project assets
  asset inspect <path>                              Asset metadata + type-specific details
  asset dependencies <path> [--recursive]           Direct (or recursive) dependencies
  asset references <path> [--limit N]               Who references this asset (full scan)
  asset guid <path>                                 Path -> Unity GUID
  asset path <guid>                                 Unity GUID -> path
  asset create --type <T> --path <p>                Create ScriptableObject (or --type Folder)
  asset move <src> <dst>                            Move/rename (keeps GUID, references survive)
  asset delete <path> [--permanent]                 Trash (default) or permanent delete
  asset label <add|remove|list|set|clear> <path> [labels]  Manage asset labels

Prefabs:
  prefab instantiate <path> [--parent go:P] [--pos x,y,z]   Spawn a prefab instance
  prefab unpack go:XXXXXXXX [--mode root|completely]        Convert instance -> plain GO
  prefab apply go:XXXXXXXX                                  Commit overrides to asset
  prefab find-instances <path>                              List scene instances

Transactions:
  tx begin [--name "My change"]    Start grouping mutations into one Undo entry
  tx commit [--name "..."]         Merge all mutations since begin into one group
  tx rollback                      Revert every mutation since begin
  tx status                        Report whether a transaction is active

Project:
  project info                     Unity version, packages, scenes, stats
  project validate [--include-packages]  Scan for missing scripts / build-settings issues
  project preflight                Validate + player-settings + compile state check

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
  test run [--mode X]             Run EditMode (default) or PlayMode tests
  test run --filter <name>        Filter by namespace, class, or full test name
  test run --output junit.xml     Also write JUnit XML report next to the results
  test list [--mode X]            Enumerate tests without running them
  test                            Back-compat alias for ` + "`test run`" + `

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
		fmt.Print(`Usage: udit go <find|inspect|path|create|destroy|move|rename|setactive> [options]

Query and mutate GameObjects in the loaded scenes. All results are keyed
by go:XXXXXXXX stable IDs — the same format emitted by ` + "`udit scene tree`" + `,
and stable across Editor restarts.

Read subcommands:
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

Mutation subcommands (all support --dry-run, all routed through Unity Undo):
  create --name <name> [--parent go:XXX] [--pos x,y,z]
                              Spawn a GameObject. Returns the new go: ID.
                              Without --parent, attached to scene root.
                              --pos is local position 'x,y,z' floats.
  destroy <go:XXXXXXXX>       Destroy a GameObject and every descendant.
                              Reports children_affected so the caller knows
                              the cascade size.
  move <go:XXXXXXXX> [--parent go:YYY]
                              Reparent. Omit --parent to move to scene root.
                              Cycles (parent under self/descendant) are
                              rejected with UCI-011.
  rename <go:XXXXXXXX> <newname>
                              Rename a GameObject in place.
  setactive <go:XXXXXXXX> --active true|false
                              Toggle activeSelf. Already-in-state calls
                              return success with no_change=true.

Common flags:
  --dry-run                   Report what would change (children_affected,
                              from/to fields, etc.) without mutating the
                              scene. Mutation routes through this flag
                              uniformly so agents can preview every change.

Examples:
  udit go find
  udit go find --name "Enemy*" --tag Enemy
  udit go inspect go:9598abb1
  udit go create --name Boss --pos 0,5,0
  udit go create --name Minion --parent go:abcd1234 --dry-run
  udit go destroy go:5678abcd
  udit go move go:5678abcd --parent go:abcd1234
  udit go move go:5678abcd                    # to scene root
  udit go rename go:5678abcd "Renamed_Boss"
  udit go setactive go:5678abcd --active false

Notes:
  - All mutations register with Unity Undo: Ctrl+Z in the Editor reverses
    create/destroy/move/rename/setactive. The active scene is marked dirty
    so the standard save prompt fires on close.
  - Mutations are blocked while Unity is in play mode.
  - Find results are sorted by hierarchy path so paginated queries are
    deterministic across calls.
  - Unknown / stale IDs return UCI-042 GameObjectNotFound (see
    docs/ERROR_CODES.md). Run ` + "`go find`" + ` or ` + "`scene tree`" + ` first to
    seed the stable-ID registry when an ID is from a previous session.
  - inspect truncates arrays at 20 elements; use ` + "`component get`" + ` for
    full array contents.
`)
	case "component":
		fmt.Print(`Usage: udit component <list|get|schema|add|remove|set|copy> [options]

Read and mutate component values. Field names mirror what ` + "`go inspect`" + `
emits, so the same vocabulary works end-to-end: find a GameObject, inspect
it, zoom in via ` + "`component get`" + `, then edit via ` + "`component set`" + `.

Read subcommands:
  list <go:XXXXXXXX>
      Enumerate components on a GameObject. Each entry has
      { index, type, full_type, enabled }.
  get <go:XXXXXXXX> <TypeName> [field]
      Dump one component. Without a field, returns every visible property.
      With a dotted field path ("position", "position.x", "m_Cameras.elements.0"),
      returns just the leaf value.
      --index N     Pick the Nth component when multiple of the same type
                    are attached (e.g. two BoxColliders). Default 0.
  schema <TypeName>
      Serialized-property schema for a type. Requires a live instance in
      the loaded scenes (v1 probes existing rather than spawning).

Mutation subcommands (all support --dry-run, all routed through Unity Undo):
  add <go:XXXXXXXX> --type <TypeName>
      Add a component. Respects DisallowMultipleComponent + RequireComponent.
      Transform cannot be re-added (every GameObject has one already).
  remove <go:XXXXXXXX> <TypeName> [--index N]
      Remove a component. Transform cannot be removed — destroy the
      GameObject with ` + "`go destroy`" + ` instead.
  set <go:XXXXXXXX> <TypeName> <field> <value> [--index N]
      Write one field. Value is parsed based on the field's
      SerializedPropertyType:
        Integer/LayerMask/ArraySize/Character : "42"
        Boolean                               : "true" / "false" / 1 / 0 / yes / no
        Float                                 : "3.14"
        String                                : anything
        Vector2                               : "x,y"
        Vector3                               : "x,y,z"
        Vector4 / Quaternion                  : "x,y,z,w"
        Color                                 : "r,g,b[,a]" (0..1 floats) or "#RRGGBB[AA]"
        Enum                                  : display name ("Solid Color") or value
      Transform has virtual fields that use Transform API directly for
      world-space: "position", "local_position", "rotation_euler",
      "local_rotation_euler", "local_scale" — all take "x,y,z".
      ObjectReference/Curve/Gradient/ManagedReference are read-only for now.
  copy <go:SRC> <TypeName> <go:DST> [--index N]
      Copy a component from source to destination via
      EditorUtility.CopySerialized. If destination lacks that type,
      AddComponent runs first. Transform cannot be copied this way.

Type name matching:
  - Case-insensitive.
  - Unqualified short names resolve against UnityEngine.* first.
  - Pass the full namespace ("MyGame.Transform") for project types that
    shadow a UnityEngine name.

Examples:
  udit component list go:9598abb1
  udit component get go:9598abb1 Transform position
  udit component schema Rigidbody
  udit component add go:9598abb1 --type Rigidbody
  udit component set go:9598abb1 Transform position 0,10,0
  udit component set go:9598abb1 Camera m_BackGroundColor "#FF8800"
  udit component set go:9598abb1 Camera m_ClearFlags "Solid Color"
  udit component remove go:9598abb1 Rigidbody
  udit component copy go:aaaa1111 Rigidbody go:bbbb2222

Notes:
  - Mutations are blocked in play mode and register with Unity Undo; each
    gets its own group so Ctrl+Z reverses one logical change at a time.
  - Unknown go: IDs -> UCI-042. Missing type, bad index, no live instance
    for schema -> UCI-043. Field not found or unsupported for writing ->
    UCI-011 with guidance (valid field names, supported property types).
`)
	case "asset":
		fmt.Print(`Usage: udit asset <find|inspect|dependencies|references|guid|path|create|move|delete|label> [options]

Query project assets. All paths are project-relative (Assets/... or
Packages/...), all GUIDs are Unity's 32-char hex identifiers.

Subcommands:
  find                      Query the AssetDatabase. Filters are ANDed and
                            results are sorted by path.
    --type <T>              Type filter, e.g. Prefab, Texture2D, Material,
                            AudioClip, ScriptableObject. Maps to Unity's
                            't:' filter syntax.
    --label <L>             Label filter. Maps to 'l:'.
    --name <glob>           Case-insensitive name glob ('*' wildcard). Applied
                            after the AssetDatabase filters because Unity's
                            free-text term is a substring match, not a glob.
    --folder <F1,F2,...>    Restrict search to these folders (comma-separated).
    --limit <N>             Max results per page (default 100, max 1000).
    --offset <N>            Skip first N matches (default 0).
  inspect <path>            Asset metadata (path, guid, type, labels) plus a
                            type-specific 'details' block for Prefab, Texture2D,
                            Material, AudioClip, ScriptableObject, TextAsset.
                            Other types return details=null with the common
                            header so agents still get name + guid + type.
  dependencies <path>       Paths of every asset this one depends on.
    --recursive             Walk the whole transitive tree. Default is direct
                            deps only, which matches what Unity's Inspector
                            shows and is usually what you want.
  references <path>         Paths of every asset that references this one.
                            Unity has no reverse-index, so this scans the
                            entire project — the response includes scan_ms
                            and scanned_assets so an agent knows the cost.
    --limit <N>             Max results per page (default 100, max 1000).
    --offset <N>            Skip first N matches (default 0).
  guid <path>               Path -> GUID lookup.
  path <guid>               GUID -> path lookup.

Mutation subcommands (all support --dry-run; NOT routed through Unity Undo —
AssetDatabase writes straight to disk. Safety nets are --dry-run preview and
'delete' defaulting to the OS trash):
  create --type <TypeName> --path <path>
      Create a new asset. ScriptableObject-derived types supported (pass a
      fully-qualified name like "MyGame.GameConfig" to disambiguate against
      UnityEngine types) plus the special sentinel "Folder" for directory
      creation. --path ending in '/' or resolving to an existing folder
      auto-appends "<TypeName>.asset".
  move <src> <dst>
      Rename or relocate. GUID is preserved so existing references keep
      resolving. Destination folder must exist.
  delete <path> [--permanent]
      Default sends the asset to the OS trash (AssetDatabase.
      MoveAssetToTrash — recoverable). --permanent uses
      AssetDatabase.DeleteAsset and scans the project first to report how
      many other assets reference this one, so the caller sees the blast
      radius.
  label <op> <path> [labels...]
      Manage AssetDatabase labels. Sub-ops:
        add     add one or more labels (union with current)
        remove  remove one or more labels
        list    read-only, returns current labels
        set     replace the whole label set
        clear   remove all labels
      Labels arrive from the CLI as a comma-joined string; the C# side
      splits them back.

Examples:
  udit asset find --type Prefab
  udit asset inspect Assets/Materials/Player.mat
  udit asset dependencies Assets/Scenes/Main.unity --recursive
  udit asset references Assets/Prefabs/Enemy.prefab
  udit asset guid Assets/Scenes/SampleScene.unity
  udit asset path 8c9cfa26abfee488c85f1582747f6a02
  udit asset create --type MyGame.GameConfig --path Assets/Config/
  udit asset create --type Folder --path Assets/NewFolder
  udit asset move Assets/Old.prefab Assets/New/Moved.prefab
  udit asset delete Assets/Unused.prefab
  udit asset delete Assets/Unused.prefab --permanent
  udit asset label add Assets/Prefabs/Boss.prefab boss_content critical
  udit asset label remove Assets/Prefabs/Boss.prefab boss_content
  udit asset label list Assets/Prefabs/Boss.prefab
  udit asset label set Assets/Prefabs/Boss.prefab final_content
  udit asset label clear Assets/Prefabs/Boss.prefab

Notes:
  - Unknown paths or GUIDs -> UCI-040 AssetNotFound (see docs/ERROR_CODES.md).
  - 'references' cost grows with project size. Use 'dependencies' in reverse
    when possible, and always set --limit on larger projects.
  - 'inspect' on a Prefab returns just a top-level summary (root components,
    child count). Use 'scene open' or 'scene tree' to walk the hierarchy.
  - Asset mutations do NOT register with Unity Undo. Ctrl+Z in the Editor
    will not reverse them. Always run with --dry-run first when you are
    uncertain, and prefer 'delete' over 'delete --permanent' when possible.
`)
	case "prefab":
		fmt.Print(`Usage: udit prefab <instantiate|unpack|apply|find-instances> [options]

Prefab operations. Asset paths are project-relative (` + "`Assets/...`" + ` or
` + "`Packages/...`" + `); stable IDs are go:XXXXXXXX issued by scene tree / go find.

Subcommands:
  instantiate <path>
      Spawn a scene instance of a prefab asset. Returns the new go: ID.
      Uses PrefabUtility.InstantiatePrefab so the instance keeps its link
      to the asset (unlike Object.Instantiate).
    --parent <go:P>   Attach under this GameObject. Omit for scene root.
    --pos x,y,z       localPosition of the new instance. Default 0,0,0.
                      Matches the convention of ` + "`go create --pos`" + `.

  unpack <go:XXXXXXXX>
      Convert a prefab instance into a plain GameObject. Breaks the link
      to the asset — the GO retains its current state but future changes
      to the prefab will not propagate.
    --mode root         (default) Only the outermost prefab root is
                        unpacked; nested prefab instances inside keep
                        their links.
    --mode completely   Unpack everything recursively, including nested
                        prefab instances.

  apply <go:XXXXXXXX>
      Commit the scene instance's overrides back to the prefab asset.
      Works on the outermost prefab root; passing a nested GO under an
      instance auto-resolves to the outermost root.

  find-instances <path>
      Walk every loaded scene and return all outermost instances of the
      given prefab. Response has { id, name, scene, path } per match.
      Read-only — no Undo entry.

All mutation subcommands support --dry-run to preview without touching
the scene.

Examples:
  udit prefab instantiate Assets/Prefabs/Enemy.prefab --pos 5,0,0
  udit prefab instantiate Assets/Prefabs/Enemy.prefab --parent go:abcd1234
  udit prefab unpack go:5678abcd
  udit prefab unpack go:5678abcd --mode completely
  udit prefab apply go:5678abcd
  udit prefab find-instances Assets/Prefabs/Enemy.prefab

Notes:
  - Path not a prefab asset -> UCI-040 AssetNotFound.
  - Path exists but is not a prefab (e.g. a model or plain
    GameObject asset) -> UCI-011 with a clear message.
  - Passing a non-instance go: to unpack/apply -> UCI-011
    "not a prefab instance".
  - Mutations are blocked in play mode and register with Unity Undo;
    Ctrl+Z in the Editor reverses each op independently.
`)
	case "tx", "transaction":
		fmt.Print(`Usage: udit tx <begin|commit|rollback|status> [options]

Group multiple mutations into a single Unity Undo entry. Without a
transaction, every mutation (go create, component set, prefab
instantiate, ...) is its own Undo group — reversing a multi-step agent
change takes N Ctrl+Z's. Inside a transaction, commit collapses all of
them into one group; rollback reverts the whole set in place.

Subcommands:
  begin [--name <label>]
      Start a transaction. Captures the current Unity Undo group index.
      Any subsequent mutation goes into the same logical transaction.
      --name sets the label that appears in Edit → Undo after commit
      (default "udit transaction").

  commit [--name <label>]
      Merges every Undo sub-group created since begin into one group
      via Undo.CollapseUndoOperations. Agent can override the label one
      last time with --name — handy when the final description of the
      change only crystallises after the work is done.

  rollback
      Undoes every mutation made since begin via
      Undo.RevertAllDownToGroup, returning the scene to its pre-begin
      state.

  status
      Reports whether a transaction is active. Response has
      { active: bool, group, name, duration_ms } when active, and just
      { active: false } when not.

Constraints:
  - One transaction at a time per Unity instance. begin during an
    active tx returns UCI-011 with the existing transaction's name.
  - State is torn down on domain reload (script compile). A reload
    mid-transaction leaves partial mutations on the Undo stack but
    drops the transaction handle — tx status will report "no active"
    afterward, and a fresh begin starts a new transaction.
  - Mutations are blocked in play mode regardless of transaction state.

Examples:
  udit tx begin --name "Spawn boss setup"
  udit go create --name Boss
  udit component add go:abcd1234 --type Rigidbody
  udit component set go:abcd1234 Rigidbody m_Mass 5.5
  udit tx commit                               # Ctrl+Z in Editor now reverses all 3 at once

  # Mid-transaction rollback
  udit tx begin --name "Try a layout"
  udit go create --name Candidate
  udit go move go:abcd1234 --parent go:5678abcd
  udit tx rollback                             # every change since begin is undone
`)
	case "project":
		fmt.Print(`Usage: udit project <info|validate|preflight> [options]

Project-level inspection. First Automate-phase block: agents use these
to answer "what is this project?" and "is it healthy enough to build?"
before kicking off heavier operations.

Subcommands:
  info
      Summary of the project:
        - unity_version, active build target, render pipeline
        - product_name, company_name, bundle_version
        - scripting_backend, color_space
        - scenes_in_build (with enabled flag + build index)
        - packages (declared versions from Packages/manifest.json)
        - stats: total_assets, cs_files_in_assets, scenes, prefabs,
          materials, textures
      Fast — no async PackageManager calls, just AssetDatabase counts.

  validate
      Scan the project for issues an agent should know about:
        - Prefab assets with missing script references (MonoBehaviour
          slots that point at deleted types)
        - Build Settings with no enabled scenes
      Response includes ok (bool), errors/warnings count, scan_ms, and
      an issues array with { severity, kind, path, message, ... } per
      finding.
    --include-packages    Also scan Packages/. Default is Assets-only
                          because Packages/ is usually clean + scan is
                          slower.
    --limit <N>           Max issues per severity (default 100).

  preflight
      validate + build-readiness checks:
        - Compile state (warn if actively compiling)
        - PlayerSettings.productName empty
        - PlayerSettings.companyName is "DefaultCompany"
      Use before ` + "`udit build player`" + ` to catch empty names /
      missing scenes / compile hiccups up front.

Examples:
  udit project info
  udit project info --json
  udit project validate
  udit project validate --include-packages
  udit project preflight

Notes:
  - Scans walk every prefab and load it — on large projects expect
    1-5s per 1000 prefabs. scan_ms is reported so agents can decide
    whether to cache results between runs.
  - info does NOT wait on PackageManager.Client.List; package info is
    read straight from manifest.json. For resolved package graphs
    (including transitive dependencies), fall back to exec with
    PackageManager.Client.List.
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
		fmt.Print(`Usage: udit test <run|list> [options]
       udit test [options]        (back-compat alias for ` + "`test run`" + `)

Run or enumerate Unity tests via the Test Runner API.

Subcommands:
  run [--mode X] [--filter P] [--output junit.xml]
      Execute tests. EditMode holds the connection open and returns
      results directly; PlayMode returns immediately and polls a status
      file (domain-reload safe).
      --mode <EditMode|PlayMode>    Test mode (default: EditMode)
      --filter <name>               Namespace / class / full test name.
                                    Must be the full path (e.g.
                                    MyNamespace.MyClass or
                                    MyNamespace.MyClass.SpecificTest).
      --output <path>               Also write a JUnit XML report.
                                    Path is absolute or project-root-
                                    relative. Written after the run so
                                    CI systems can consume it.

  list [--mode X]
      List every test in the requested mode without running any. Returns
      { mode, total, tests[] } where each test has { full_name, name,
      class_name, type_info, run_state }. Use to discover test names
      before running, or to pick one by full_name for --filter.

Requires the Unity Test Framework package (com.unity.test-framework).

Examples:
  udit test                                           # EditMode run (back-compat)
  udit test run --mode PlayMode
  udit test run --filter MyNamespace.MyTests
  udit test run --mode PlayMode --output test-results/playmode.xml
  udit test list                                      # EditMode discovery
  udit test list --mode PlayMode --json

Notes:
  - JUnit XML output is compatible with GitHub Actions / GitLab CI
    junit parsers out of the box. Failed tests include the Unity
    failure message + stack in <failure>, skipped/inconclusive in
    <skipped>.
  - list does NOT execute tests — it just walks the TestRunnerApi
    test tree. Safe to call on untrusted code that would otherwise
    misbehave when run.
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
