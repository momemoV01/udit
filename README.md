# udit

[English](README.md) | [한국어](README.ko.md)

> Unity editor, from the command line. Built for AI agents, works with anything.
>
> Udit (उदित) — Sanskrit for *risen*. A fork of [unity-cli](https://github.com/youngwoocho02/unity-cli) by [DevBookOfArray](https://github.com/youngwoocho02), extended for agent-first game development workflows.

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**No server to run. No config to write. No process to manage. Just type a command.**

## Why this exists

I wanted to control Unity from the terminal. The existing MCP-based integrations required Python runtimes, WebSocket relays, JSON-RPC protocol layers, config files, server processes that need to be started and stopped, tool registration ceremonies, and tens of thousands of lines of over-engineered code. All just to send a simple command to Unity.

On top of that, every AI agent that wanted to use it needed its own MCP config and integration setup. The CLI doesn't care — any agent that can run a shell command can use it immediately.

That felt wrong. If I can `curl` a URL, why do I need all that?

So I built the opposite: a single binary that talks directly to Unity via HTTP. No server to run — the Unity package listens automatically. No config to write — it discovers Unity instances on its own. No tool registration — just call by name. No caching, no protocol layers, no ceremony.

The entire CLI is ~800 lines of Go (plus ~300 lines of help text). The Unity-side connector is ~2,300 lines of C#. It's just a thin layer that lets you control Unity from the shell — nothing more. You install the binary, add the Unity package, and it works.

## Install

### Linux / macOS

```bash
curl -fsSL https://raw.githubusercontent.com/momemoV01/udit/main/install.sh | sh
```

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/momemoV01/udit/main/install.ps1 | iex
```

### Other options

```bash
# Go install (any platform with Go)
go install github.com/momemoV01/udit@latest

# Manual download (pick your platform)
# Linux amd64 / Linux arm64 / macOS amd64 / macOS arm64 / Windows amd64
curl -fsSL https://github.com/momemoV01/udit/releases/latest/download/udit-linux-amd64 -o udit
chmod +x udit && sudo mv udit /usr/local/bin/
```

Supported platforms: Linux (amd64, arm64), macOS (Intel, Apple Silicon), Windows (amd64).

### Update

```bash
# Update to the latest version
udit update

# Check for updates without installing
udit update --check
```

## Unity Setup

Add the Unity Connector package via **Package Manager → Add package from git URL**:

```
https://github.com/momemoV01/udit.git?path=udit-connector
```

Or add directly to `Packages/manifest.json`:
```json
"com.momemov01.udit-connector": "https://github.com/momemoV01/udit.git?path=udit-connector"
```

To pin a specific version, append a tag to the URL (e.g. `#v0.2.21`).

Once added, the Connector starts automatically when Unity opens. No configuration needed.

### Recommended: Disable Editor Throttling

By default, Unity throttles editor updates when the window is unfocused. This means CLI commands may not execute until you click back into Unity.

To fix this, go to **Edit → Preferences → General → Interaction Mode** and set it to **No Throttling**.

This ensures CLI commands are processed immediately, even when Unity is in the background.

## Quick Start

```bash
# Check Unity connection
udit status

# Enter play mode and wait
udit editor play --wait

# Run C# code inside Unity
udit exec "return Application.dataPath;"

# Read console logs
udit console --type error,warning,log
```

## How It Works

```
Terminal                              Unity Editor
────────                              ────────────
$ udit editor play --wait
    │
    ├─ scans ~/.udit/instances/*.json
    │  → finds Unity on port 8590
    │
    ├─ POST http://127.0.0.1:8590/command
    │  { "command": "manage_editor",
    │    "params": { "action": "play",
    │                "wait_for_completion": true }}
    │                                      │
    │                                  HttpServer receives
    │                                      │
    │                                  CommandRouter dispatches
    │                                      │
    │                                  ManageEditor.HandleCommand()
    │                                  → EditorApplication.isPlaying = true
    │                                  → waits for PlayModeStateChange
    │                                      │
    ├─ receives JSON response  ←───────────┘
    │  { "success": true,
    │    "message": "Entered play mode (confirmed)." }
    │
    └─ prints: Entered play mode (confirmed).
```

The Unity Connector:
1. Opens an HTTP server on `localhost:8590` when the Editor starts
2. Writes a per-project instance file to `~/.udit/instances/` so the CLI knows where to connect
3. Updates the instance file every 0.5s with the current state (heartbeat)
4. Discovers all `[UditTool]` classes via reflection on each request
5. Routes incoming commands to the matching handler on the main thread
6. Survives domain reloads (script recompilation)

Before compiling or reloading, the Connector records the state (`compiling`, `reloading`) to the instance file. When the main thread freezes, the timestamp stops updating. The CLI detects this and waits for a fresh timestamp before sending commands.

## Built-in Commands

| Command | Description |
|---------|-------------|
| `editor` | Play/stop/pause/refresh the Unity Editor |
| `console` | Read, filter, and clear console logs |
| `exec` | Run arbitrary C# code inside Unity |
| `test` | Run EditMode/PlayMode tests |
| `menu` | Execute any Unity menu item by path |
| `reserialize` | Re-serialize assets through Unity's serializer |
| `screenshot` | Capture scene/game view as PNG |
| `profiler` | Read profiler hierarchy, control recording |
| `list` | Show all available tools with parameter schemas |
| `status` | Show Unity Editor connection state |
| `update` | Self-update the CLI binary |

### Editor Control

```bash
# Enter play mode
udit editor play

# Enter play mode and wait until fully loaded
udit editor play --wait

# Stop play mode
udit editor stop

# Toggle pause (only works during play mode)
udit editor pause

# Refresh assets
udit editor refresh

# Refresh and recompile scripts (waits for compilation to finish)
udit editor refresh --compile
```

### Scenes

Observe and switch between scenes without dropping into `exec`. Every subcommand emits structured JSON when `--json` is set, so agents can chain results.

```bash
# List every scene asset in the project (Assets + Packages)
udit scene list

# Describe the currently active scene (path, guid, dirty state, root count)
udit scene active

# Open a scene as the single active scene
udit scene open Assets/Scenes/Main.unity

# Save every open scene that is currently dirty
udit scene save

# Reload the active scene, discarding unsaved edits (requires --force when dirty)
udit scene reload --force
```

**Dirty-scene guard.** `scene open` and `scene reload` refuse to run when the current scene has unsaved changes. Pass `--force` to discard, or call `scene save` first. Both commands are also blocked while Unity is in play mode.

### Console Logs

```bash
# Read error and warning logs (default)
udit console

# Read last 20 log entries of all types
udit console --lines 20 --filter error,warning,log

# Read only errors
udit console --type error

# Include stack traces (user: user code only, full: raw)
udit console --stacktrace user

# Clear console
udit console --clear
```

### Execute C# Code

Run arbitrary C# code inside the Unity Editor at runtime. This is the most powerful command — it gives you full access to UnityEngine, UnityEditor, ECS, and every loaded assembly. No need to write a custom tool for one-off queries or mutations.

Use `return` to get output. Common namespaces are included by default. Add `--usings` only for project-specific types (e.g. `Unity.Entities`). The csc compiler and dotnet runtime are auto-detected; if detection fails, specify manually with `--csc <path>` or `--dotnet <path>`.

```bash
udit exec "return Application.dataPath;"
udit exec "return EditorSceneManager.GetActiveScene().name;"
udit exec "return World.All.Count;" --usings Unity.Entities

# Pipe via stdin to avoid shell escaping issues
echo 'Debug.Log("hello"); return null;' | udit exec
echo 'var go = new GameObject("Marker"); go.tag = "EditorOnly"; return go.name;' | udit exec
```

Because `exec` compiles and runs real C#, it can do anything a custom tool can — inspect ECS entities, modify assets, call internal APIs, run editor utilities. For AI agents, this means **zero-friction access to Unity's entire runtime** without writing a single line of tool code. Piping via stdin avoids shell escaping headaches with complex code.

### Menu Items

```bash
# Execute any Unity menu item by path
udit menu "File/Save Project"
udit menu "Assets/Refresh"
udit menu "Window/General/Console"
```

Note: `File/Quit` is blocked for safety.

### Asset Reserialize

AI agents (and humans) can edit Unity asset files — `.prefab`, `.unity`, `.asset`, `.mat` — as plain text YAML. But Unity's YAML serializer is strict: a missing field, wrong indent, or stale `fileID` will corrupt the asset silently.

`reserialize` fixes this. After a text edit, it tells Unity to load the asset into memory and write it back out through its own serializer. The result is a clean, valid YAML file — as if you had edited it through the Inspector.

```bash
# Reserialize the entire project (no arguments)
udit reserialize

# After editing a prefab's transform values in a text editor
udit reserialize Assets/Prefabs/Player.prefab

# After batch-editing multiple scenes
udit reserialize Assets/Scenes/Main.unity Assets/Scenes/Lobby.unity

# After modifying material properties
udit reserialize Assets/Materials/Character.mat
```

This is what makes text-based asset editing safe. Without it, a single misplaced YAML field can break a prefab with no visible error until runtime. With it, **AI agents can confidently modify any Unity asset through plain text** — add components to prefabs, adjust scene hierarchies, change material properties — and know the result will load correctly.

### Profiler

```bash
# Read profiler hierarchy (last frame, top-level)
udit profiler hierarchy

# Recursive drill-down
udit profiler hierarchy --depth 3

# Set root by name (substring match) — focus on a specific system
udit profiler hierarchy --root SimulationSystem --depth 3

# Drill into a specific item by ID
udit profiler hierarchy --parent 4 --depth 2

# Average over last 30 frames
udit profiler hierarchy --frames 30 --min 0.5

# Average over a specific frame range
udit profiler hierarchy --from 100 --to 200

# Filter and sort
udit profiler hierarchy --min 0.5 --sort self --max 10

# Enable/disable profiler recording
udit profiler enable
udit profiler disable

# Show profiler state
udit profiler status

# Clear captured frames
udit profiler clear
```

### Run Tests

Run EditMode and PlayMode tests via the Unity Test Framework.

```bash
# Run EditMode tests (default)
udit test

# Run PlayMode tests
udit test --mode PlayMode

# Filter by test name (substring match)
udit test --filter MyTestClass
```

Requires the Unity Test Framework package. PlayMode tests trigger a domain reload — the CLI polls for results automatically.

### List Tools

```bash
# Show all available tools (built-in + project custom) with parameter schemas
udit list
```

### Shell completion

```bash
# Bash   (sourced)   : source <(udit completion bash)
# Zsh    (sourced)   : source <(udit completion zsh)
# PowerShell         : udit completion powershell | Out-String | Invoke-Expression
# Fish               : udit completion fish > ~/.config/fish/completions/udit.fish
```

Each completion script is wrapped in sentinel comments:
```
# >>> udit completion >>>
...
# <<< udit completion <<<
```

**Safe re-install** — running `>> $PROFILE` (or `>> ~/.bashrc`) a second time
will duplicate the block and break the shell init. Remove the previous block
first, then append fresh.

Bash / Zsh:
```bash
# Strip any previous udit completion block
sed -i '/^# >>> udit completion >>>/,/^# <<< udit completion <<</d' ~/.bashrc
# Or for zsh: ~/.zshrc
udit completion bash >> ~/.bashrc
```

PowerShell (Windows / cross-platform):
```powershell
$p = Get-Content $PROFILE -Raw
$p = $p -replace '(?s)# >>> udit completion >>>.*?# <<< udit completion <<<\r?\n?', ''
Set-Content $PROFILE -Value $p.TrimEnd() -Encoding utf8
udit completion powershell | Out-File -Append -Encoding utf8 $PROFILE
```

Fish needs no cleanup — each completion lives in its own file, so overwriting
`~/.config/fish/completions/udit.fish` is always safe.

### Custom Tools

```bash
# Call a custom tool directly by name
udit my_custom_tool

# Call with parameters
udit my_custom_tool --params '{"key": "value"}'
```

### Status

```bash
# Show Unity Editor state
udit status
# Output: Unity (port 8590): ready
#   Project: /path/to/project
#   Version: 6000.1.0f1
#   PID:     12345
```

The CLI also checks Unity's state automatically before sending any command. If Unity is busy (compiling, reloading), it waits for Unity to become responsive.

## Global Options

| Flag | Description | Default |
|------|-------------|---------|
| `--port <N>` | Override Unity instance port (skip auto-discovery) | auto |
| `--project <path>` | Select Unity instance by project path | latest |
| `--timeout <ms>` | HTTP request timeout | 120000 |

```bash
# Connect to a specific Unity instance
udit --port 8591 editor play

# Select by project path when multiple Unity instances are open
udit --project MyGame editor stop
```

Use `--help` on any command for detailed usage:

```bash
udit editor --help
udit exec --help
udit profiler --help
```

## Writing Custom Tools

Create a static class with `[UditTool]` attribute in any Editor assembly. The Connector discovers it automatically on domain reload.

```csharp
using UditConnector;
using Newtonsoft.Json.Linq;
using UnityEngine;

[UditTool(Name = "spawn", Description = "Spawn an enemy at a position", Group = "gameplay")]
public static class SpawnEnemy
{
    public class Parameters
    {
        [ToolParameter("X world position", Required = true)]
        public float X { get; set; }

        [ToolParameter("Y world position", Required = true)]
        public float Y { get; set; }

        [ToolParameter("Z world position", Required = true)]
        public float Z { get; set; }

        [ToolParameter("Prefab name in Resources folder", DefaultValue = "Enemy")]
        public string Prefab { get; set; }
    }

    public static object HandleCommand(JObject parameters)
    {
        var p = new ToolParams(parameters);
        float x = p.GetFloat("x", 0);
        float y = p.GetFloat("y", 0);
        float z = p.GetFloat("z", 0);
        string prefabName = p.Get("prefab", "Enemy");

        var prefab = Resources.Load<GameObject>(prefabName);
        var instance = Object.Instantiate(prefab, new Vector3(x, y, z), Quaternion.identity);

        return new SuccessResponse("Enemy spawned", new
        {
            name = instance.name,
            position = new { x, y, z }
        });
    }
}
```

Call it directly with flags or JSON:

```bash
udit spawn --x 1 --y 0 --z 5 --prefab Goblin
udit spawn --params '{"x":1,"y":0,"z":5,"prefab":"Goblin"}'
```

**Key points:**

- **Name**: without `Name`, auto-derived from class name (`SpawnEnemy` → `spawn_enemy`, `UITree` → `ui_tree`). With `Name = "spawn"`, the command becomes `udit spawn`.
- **Parameters class**: optional but recommended. `udit list` uses it to expose parameter names, types, descriptions, and required flags — so AI assistants can discover your tool without reading the source.
- **ToolParams**: use `p.Get()`, `p.GetInt()`, `p.GetFloat()`, `p.GetBool()`, `p.GetRaw()` for consistent param reading.
- **Discovery**: `udit list` shows built-in tools first (`group: "built-in"`), then custom tools (`group: "custom"`) detected from the connected Unity project.

**Attribute reference:**

| Attribute | Property | Description |
|---|---|---|
| `[UditTool]` | `Name` | Command name override (default: class name → snake_case) |
| | `Description` | Tool description shown in `list` |
| | `Group` | Group name for categorization |
| `[ToolParameter]` | `Description` | Parameter description (constructor arg) |
| | `Required` | Whether the parameter is required (default: `false`) |
| | `Name` | Parameter name override |
| | `DefaultValue` | Default value hint |

### Rules

- Class must be `static`
- Must have `public static object HandleCommand(JObject parameters)` or `async Task<object>` variant
- Return `SuccessResponse(message, data)` or `ErrorResponse(message)`
- Add a `Parameters` nested class with `[ToolParameter]` attributes for discoverability
- Class name is auto-converted to snake_case for the command name
- Override with `[UditTool(Name = "my_name")]` if needed
- Runs on Unity main thread, so all Unity APIs are safe to call
- Discovered automatically on Editor start and after every script recompilation
- Duplicate tool names are detected and logged as errors — only the first discovered handler is used

## Multiple Unity Instances

When multiple Unity Editors are open, each registers on a different port (8590, 8591, ...):

```bash
# See all running instances
ls ~/.udit/instances/

# Select by project path
udit --project MyGame editor play

# Select by port
udit --port 8591 editor play

# Default: uses the most recently registered instance
udit editor play
```

## Compared to MCP

| | MCP | udit |
|---|-----|-----------|
| **Install** | Python + uv + FastMCP + config JSON | Single binary |
| **Dependencies** | Python runtime, WebSocket relay | None |
| **Protocol** | JSON-RPC 2.0 over stdio + WebSocket | Direct HTTP POST |
| **Setup** | Generate MCP config, restart AI tool | Add Unity package, done |
| **Reconnection** | Complex reconnect logic for domain reloads | Stateless per request |
| **Compatibility** | MCP-compatible clients only | Anything with a shell |
| **Custom tools** | Same `[Attribute]` + `HandleCommand` pattern | Same |

## Roadmap

See [`docs/ROADMAP.md`](./docs/ROADMAP.md) for the phased plan from `v0.1.0` (current baseline) through `v1.0.0` (API-frozen, production-ready). Highlights:

- **v0.2.0 — Foundation** — bug fixes, global `--json` output, error code registry, `.udit.yaml` config
- **v0.3.0 — Observe** — `scene` / `go` / `asset` / `component` query commands (no more `exec` for reads)
- **v0.4.0 — Mutate** — GameObject / component / prefab creation, modification, deletion
- **v0.5.0 — Automate** — `build player`, `package` (UPM), extended `test`, project preflight
- **v0.6.0 — Stream** — `watch` mode, `log tail --follow` over SSE
- **v1.0.0 — Polish & Freeze** — 50%+ test coverage, cookbook docs, 5-year API commitment

## Acknowledgments

udit is a fork of **[unity-cli](https://github.com/youngwoocho02/unity-cli)** created by **DevBookOfArray** (youngwoocho02). The original tool — its architecture, HTTP bridge, reflection-based tool discovery, heartbeat design, and domain-reload handling — forms the complete foundation of this project. The fork exists to pursue an agent-first roadmap under its own identity, with DevBookOfArray's explicit permission. See [NOTICE.md](./NOTICE.md) for details.

If you find udit useful, please also star the original and subscribe to the author:

[![Original](https://img.shields.io/badge/Original-unity--cli-success?logo=github)](https://github.com/youngwoocho02/unity-cli)
[![YouTube](https://img.shields.io/badge/YouTube-DevBookOfArray-red?logo=youtube&logoColor=white)](https://www.youtube.com/@DevBookOfArray)

## Maintainer

Maintained by **momemo** ([![GitHub](https://img.shields.io/badge/GitHub-momemoV01-181717?logo=github)](https://github.com/momemoV01))

## License

MIT — see [LICENSE](./LICENSE). Copyright notices for both the original and this fork must be preserved in all copies.
