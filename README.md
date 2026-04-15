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

# Dump the active scene hierarchy as a JSON tree with stable IDs
udit scene tree --depth 3
udit scene tree --active-only --json
```

**Dirty-scene guard.** `scene open` and `scene reload` refuse to run when the current scene has unsaved changes. Pass `--force` to discard, or call `scene save` first. Both commands are also blocked while Unity is in play mode.

**Stable IDs.** Every GameObject in `scene tree` gets a `go:XXXXXXXX` id that is a hash of Unity's `GlobalObjectId`. The id is deterministic across Editor restarts, so an agent can save results from a prior session and resolve them later via the `go` commands below. Example `roots` entry:

```json
{
  "id": "go:9598abb1",
  "name": "Main Camera",
  "active": true,
  "components": ["Transform", "Camera", "AudioListener"],
  "children": []
}
```

### GameObjects

Query GameObjects across the loaded scenes. Every result is keyed by the same `go:XXXXXXXX` id format as `scene tree`, and resolves back to a live GameObject until the Editor restarts (or the GO is destroyed).

```bash
# Find every GameObject in the loaded scenes (paginated)
udit go find

# Filter by name wildcard, tag, and component type (AND)
udit go find --name "Enemy*" --tag Enemy --component Rigidbody

# Paginate for large scenes — first page, 20 per page
udit go find --limit 20 --offset 0

# Full dump of one GameObject: scene, path, parent_id, children_ids,
# and every component's serialized properties
udit go inspect go:9598abb1

# Just the hierarchy path string ("Root/Child/Leaf")
udit go path go:9598abb1
```

`go inspect` returns every component with typed values: Transform special-cases world+local coordinates, enums render as `{value, name}`, ObjectReferences render as `{type, name, path, guid}`, arrays over 20 elements are clipped to `{count, elements, truncated: true}`. Missing-script slots show `"<Missing Script>"` so stale prefabs are detectable.

Unknown or stale ids return `UCI-042 GameObjectNotFound` — run `go find` or `scene tree` to re-seed the stable-ID registry, then retry with the fresh id.

#### GameObject mutation (v0.4.0+)

`go` also exposes the five basic scene-editing operations. Each is **routed through Unity Undo**, so Ctrl+Z in the Editor reverses an agent's changes exactly the way it would reverse a human's Inspector edit. The active scene is marked dirty so the standard save prompt fires on close.

```bash
# Spawn a GameObject. Returns the new go: ID; --pos is local position floats.
udit go create --name Boss
udit go create --name Minion --parent go:abcd1234 --pos 0,5,0

# Destroy a GameObject and every descendant. children_affected reports the cascade.
udit go destroy go:5678abcd

# Reparent. Omit --parent to move to scene root.
# Cycles (parent under self/descendant) are rejected up front.
udit go move go:5678abcd --parent go:abcd1234
udit go move go:5678abcd

# Rename in place.
udit go rename go:5678abcd "Renamed_Boss"

# Toggle activeSelf. Already-in-state calls return success with no_change=true.
udit go setactive go:5678abcd --active false
```

**`--dry-run` on every mutation.** Pass `--dry-run` to any of the five subcommands to preview the change without touching the scene. The response includes the same fields it would after a real run (`would_destroy`, `children_affected`, `from`/`to` parents, etc.) so an agent can branch on the impact before committing.

```bash
udit go destroy go:5678abcd --dry-run
# {
#   "would_destroy": "Root/Boss",
#   "children_affected": 12,
#   "components": ["Transform", "Rigidbody", "PlayerController"],
#   "dry_run": true
# }
```

Mutations are blocked while Unity is in play mode. Each mutation gets its own Unity Undo group with a descriptive name (visible under Edit → Undo in the Editor), so `Undo.PerformUndo` reverses one logical operation at a time rather than collapsing the whole agent session.

### Components

Zoom in on a single component (or a single field) without re-dumping the whole GameObject. Field names mirror what `go inspect` emits, so the same vocabulary works end-to-end.

```bash
# Enumerate the components attached to a GameObject
udit component list go:9598abb1

# Dump one component (every visible field)
udit component get go:9598abb1 Transform

# Zoom in on one field; dotted paths traverse nested objects
udit component get go:9598abb1 Transform position
udit component get go:9598abb1 Transform position.z

# Pick among multiple instances of the same type
udit component get go:abcd1234 BoxCollider --index 1

# Inspect the serialized-property schema of a type
# (requires a live instance in the loaded scenes)
udit component schema Camera
udit component schema UnityEngine.Transform
udit component schema MyGame.PlayerController
```

Type names are **case-insensitive**. Unqualified short names (`Camera`) resolve against `UnityEngine.*` first; pass the full namespace (`MyGame.Camera`) to disambiguate project types that shadow built-ins.

Failure modes:
- GameObject id unknown / stale → `UCI-042` (run `go find` to re-seed).
- Type not on the GameObject, bad `--index`, or `schema` with no live instance → `UCI-043`; the message enumerates attached types or instance count so agents can self-correct.
- Field path does not exist → `UCI-011` with the list of valid top-level fields.

#### Component mutation (v0.4.0+)

Same Undo integration and `--dry-run` surface as `go` mutations. Field names match what `component get` emits, so the read/write vocabulary is unified.

```bash
# Add a component. Respects DisallowMultipleComponent + RequireComponent.
udit component add go:9598abb1 --type Rigidbody

# Remove one. Transform is blocked (use `go destroy` instead).
udit component remove go:9598abb1 Rigidbody

# Write a field. The value is parsed based on the field's SerializedPropertyType.
udit component set go:9598abb1 Transform position 0,10,0
udit component set go:9598abb1 Rigidbody m_Mass 2.5
udit component set go:9598abb1 Camera m_BackGroundColor "#FF8800"
udit component set go:9598abb1 Camera m_BackGroundColor "1,0,0,1"
udit component set go:9598abb1 Camera m_ClearFlags "Solid Color"

# Pick the Nth instance when multiple of the same type are attached.
udit component set go:abcd1234 BoxCollider m_Size 1,1,1 --index 1

# Copy one component between GameObjects (adds on dest if missing).
udit component copy go:aaaa1111 Rigidbody go:bbbb2222
```

**Value parsing cheat sheet:**
| SerializedPropertyType | Input format |
| --- | --- |
| Integer / LayerMask / Character | `"42"` |
| Boolean | `"true"`, `"false"`, `"1"`, `"0"`, `"yes"`, `"no"`, `"on"`, `"off"` |
| Float | `"3.14"` |
| String | any text |
| Vector2 / 3 / 4 / Quaternion | comma-separated floats (`"x,y"`, `"x,y,z"`, `"x,y,z,w"`) |
| Color | `"r,g,b[,a]"` in 0–1 range, or `"#RRGGBB[AA]"` |
| Enum | display name (`"Solid Color"`) or value index |
| ObjectReference | Asset path (`"Assets/Sprites/Player.png"`) or `"null"` / `"none"` to clear |

Transform exposes **virtual fields** for world + local coordinates on `component set` as well: `position`, `local_position`, `rotation_euler`, `local_rotation_euler`, `local_scale` — all take `"x,y,z"`. This matches what `component get` returns, so round-tripping works.

**ObjectReference** accepts any project asset path. If the path has sub-assets (e.g. a `.png` imported with both `Texture2D` and `Sprite`), `component set` auto-picks the first sub-asset assignable to the target field's type. For fields with no compatible asset at the given path, you get `UCI-011` with the expected type and what was actually found at that path. Scene object references (`go:XXXXXXXX`) are not writable through `component set` in this version — fall back to `udit exec` for those.

AnimationCurve / Gradient / ExposedReference / ManagedReference are still **read-only** in this version; attempting to set them returns `UCI-011` with guidance.

### Assets

Query the AssetDatabase — prefabs, textures, materials, scripts, and anything else Unity indexes. Paths are project-relative (`Assets/...` or `Packages/...`), GUIDs are Unity's 32-char hex strings.

```bash
# Filter by type (maps to Unity's 't:' syntax), label, name glob, or folder scope
udit asset find --type Prefab
udit asset find --type Texture2D --folder Assets/Art --limit 20
udit asset find --label boss --name "*Enemy*"

# Full metadata plus a type-specific 'details' block
# (Texture2D: dimensions + format, Material: shader + properties,
#  Prefab: root components, AudioClip: length + channels, etc.)
udit asset inspect Assets/Materials/Player.mat

# What does this asset depend on? Direct by default, --recursive for transitive.
udit asset dependencies Assets/Scenes/Main.unity
udit asset dependencies Assets/Scenes/Main.unity --recursive

# Who references this asset? Unity has no reverse index, so this does a full
# project scan. Response includes scan_ms + scanned_assets so agents see the
# cost. Always set --limit on large projects.
udit asset references Assets/Prefabs/Enemy.prefab --limit 50

# GUID / path round-trip
udit asset guid Assets/Scenes/SampleScene.unity
udit asset path 8c9cfa26abfee488c85f1582747f6a02
```

`inspect` handles Prefab, Texture2D, Material, AudioClip, ScriptableObject, and TextAsset with type-specific detail blocks. Other types still return the common header (`path`, `guid`, `name`, `type`, `labels`) with `details: null` so agents can at least key off the type.

Unknown paths or GUIDs return `UCI-040 AssetNotFound` — verify the identifier with `asset find` before retrying.

#### Asset mutation (v0.4.x+)

Create, move, delete, and label assets. All mutations accept `--dry-run` and `delete` defaults to the OS trash for recoverability.

```bash
# Create a ScriptableObject-derived asset. --path ending in '/' auto-appends
# <TypeName>.asset; pass an explicit filename to override.
udit asset create --type MyGame.GameConfig --path Assets/Config/
udit asset create --type MyGame.GameConfig --path Assets/Config/Custom.asset

# Folder creation uses the sentinel type "Folder".
udit asset create --type Folder --path Assets/NewFolder

# Move keeps the GUID, so existing references in the project stay valid.
udit asset move Assets/Old.prefab Assets/New/Moved.prefab

# Delete to the OS trash (recoverable, default) or permanently.
udit asset delete Assets/Unused.prefab
udit asset delete Assets/Unused.prefab --permanent

# --permanent scans the whole project and reports how many other assets
# reference this one (referenced_by) so the caller sees the blast radius.
udit asset delete Assets/Shared.mat --permanent --dry-run

# Labels: add / remove take one or more names, set replaces the whole set,
# clear removes all, list is read-only.
udit asset label add    Assets/Prefabs/Boss.prefab boss_content critical
udit asset label remove Assets/Prefabs/Boss.prefab critical
udit asset label list   Assets/Prefabs/Boss.prefab
udit asset label set    Assets/Prefabs/Boss.prefab final_content
udit asset label clear  Assets/Prefabs/Boss.prefab
```

**Undo caveat.** AssetDatabase operations (Create/Move/Delete/SetLabels) do **not** participate in Unity's scene Undo. Ctrl+Z in the Editor will not reverse them. The safety nets are `--dry-run` (preview without side effects) and `delete` defaulting to `MoveAssetToTrash` (recoverable from the OS trash). When in doubt, dry-run first.

Failure modes:
- Missing path or GUID → `UCI-040`.
- `create --type X` where X is not a ScriptableObject-derived type (or the sentinel `Folder`) → `UCI-011` with a note on supported types. Pass a fully-qualified name (`MyGame.GameConfig`) to disambiguate against UnityEngine types.
- Destination already exists for `create` / `move` → `UCI-011` (move first or delete).
- Label op is not one of `add / remove / list / set / clear` → `UCI-011`.

### Prefabs

Prefab operations on top of `scene` + `go` + `asset`. `instantiate` uses `PrefabUtility.InstantiatePrefab` so the scene instance keeps its link back to the asset (unlike `Object.Instantiate`), and everything goes through Unity Undo.

```bash
# Spawn a scene instance of a prefab asset. --pos sets localPosition.
udit prefab instantiate Assets/Prefabs/Enemy.prefab
udit prefab instantiate Assets/Prefabs/Enemy.prefab --parent go:abcd1234 --pos 5,0,0

# Convert a scene instance back to a plain GameObject (breaks the prefab link).
udit prefab unpack go:5678abcd                       # outermost root only
udit prefab unpack go:5678abcd --mode completely     # including nested prefabs

# Commit the scene instance's overrides back to the prefab asset.
# Works on any GO inside an instance — auto-resolves to the outermost root.
udit prefab apply go:5678abcd

# Find every scene instance of a given prefab.
udit prefab find-instances Assets/Prefabs/Enemy.prefab
```

**Stable IDs shift on unpack.** When a prefab instance is unpacked, Unity's `GlobalObjectId` for the GO changes (it's no longer tied to the asset), so the stable id the registry emits changes too. The `unpack` response returns the new id — use that for subsequent operations. The old id returns `UCI-042`.

Failure modes:
- No asset at the given path → `UCI-040`.
- Path exists but isn't a GameObject (e.g. a script file) → `UCI-011` with a hint to run `asset inspect` for the actual type.
- GameObject asset exists but isn't a prefab (e.g. a raw model) → `UCI-011`.
- `unpack`/`apply` on a GO that isn't a prefab instance → `UCI-011`.

### Transactions

Without a transaction, every mutation (`go create`, `component set`, `prefab instantiate`, ...) creates its own Unity Undo group — reversing a multi-step agent change requires N Ctrl+Z's. Inside a transaction, `commit` collapses every mutation since `begin` into a single Undo entry; a single Ctrl+Z in the Editor reverses the whole batch.

```bash
# Single-Undo batch
udit tx begin --name "Spawn boss setup"
udit go create --name Boss
udit component add go:abcd1234 --type Rigidbody
udit component set go:abcd1234 Rigidbody m_Mass 5.5
udit tx commit                       # Ctrl+Z now reverses all 3 at once

# Mid-operation revert
udit tx begin --name "Try a layout"
udit go create --name Candidate
udit go move go:abcd1234 --parent go:5678abcd
udit tx rollback                     # every change since begin is unwound

# Check where you are
udit tx status
```

Implementation detail: `begin` captures the current Unity Undo group, `commit` calls `Undo.CollapseUndoOperations(savedGroup)`, `rollback` calls `Undo.RevertAllDownToGroup(savedGroup)`. The transaction's name shows up in Edit → Undo after commit, so the human who joins the project later sees "Undo Spawn boss setup" rather than "Undo udit go create 'Boss'" for the last micro-op.

Constraints worth knowing:
- **One transaction per Unity instance.** The Undo stack is global, so there's exactly one nesting at a time. `begin` during an active tx returns `UCI-011` with the existing transaction's name and age.
- **Domain reload wipes the handle.** Script recompiles tear down the connector's static state, so a mid-transaction reload leaves any partial mutations on the Undo stack but drops the transaction itself. `tx status` will report no active tx afterward — start a new one if you intended to keep grouping.
- **AssetDatabase mutations do not participate.** `asset create/move/delete/label` write straight to disk and can't be collapsed into a scene-Undo group. Inside a transaction they still execute, but they are not reversible by commit/rollback the way scene mutations are.

### Project

Pre-build health check and project inspection. First block of the Automate phase — agents use these to answer "what is this project?" and "is it healthy enough to build?" before kicking off heavier operations.

```bash
# Unity version, build target, packages, scenes in build, asset counts
udit project info

# Scan prefabs for missing script references, flag empty Build Settings
udit project validate                   # Assets/ only (fast)
udit project validate --include-packages  # also scan Packages/

# validate + player-settings + compile state check
udit project preflight
```

`project info` returns a fast summary without the async `PackageManager.Client.List` — packages come straight from `Packages/manifest.json` (declared versions). Stats come from `AssetDatabase.FindAssets` counts (no asset loads), so even on a 12k-asset project the response arrives in a few hundred ms.

`project validate` walks every prefab and uses `GameObjectUtility.GetMonoBehavioursWithMissingScriptCount` to count broken references. Response includes `scan_ms` so agents can decide whether to cache between runs. `--limit` caps issues per severity at 100 by default.

`project preflight` is `validate` + pre-build hygiene: warns on empty `productName`, `"DefaultCompany"` default, active compilation state. Use before `udit build player` (coming in a later Phase 4 slice) to catch empty names, missing scenes, or compile hiccups up front.

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

Run EditMode and PlayMode tests via the Unity Test Framework, plus enumerate them without running.

```bash
# Run EditMode tests (default)
udit test run               # or the back-compat alias: `udit test`

# Run PlayMode tests
udit test run --mode PlayMode

# Filter by full test name (substring matches the TestRunnerApi test path)
udit test run --filter MyNamespace.MyTests

# Also write a JUnit XML report (CI-friendly). Relative paths resolve
# against the CLI cwd (not Unity's project root); written after the run.
udit test run --mode PlayMode --output test-results/playmode.xml

# Enumerate tests without running any — useful for discovery before filtering
udit test list
udit test list --mode PlayMode
```

`test list` returns `{ mode, total, tests[] }` with `full_name / name / class_name / type_info / run_state` per leaf. Use `full_name` as the value for `--filter` on a subsequent `test run`.

JUnit XML output is compatible with GitHub Actions / GitLab CI JUnit parsers out of the box. Failed tests carry the Unity failure message + stack trace inside `<failure>`; skipped/inconclusive map to `<skipped/>`.

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
