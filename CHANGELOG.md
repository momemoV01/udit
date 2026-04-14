# Changelog

All notable changes to **udit** are documented here. This project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html) and [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

## [0.3.0] - 2026-04-15

First release of Phase 2 (**Observe**). Agents can now query scenes and
GameObjects without dropping into `exec`. Connector bumped to `0.2.0`
to reflect the new C# tools and shared utilities. See
[docs/ROADMAP.md](./docs/ROADMAP.md) > Phase 2a for context and the
rationale for splitting Observe into 2a (scene + go, shipped here) and
2b (asset + component, next minor).

Phase 2 was originally planned as a single release spanning scene,
go, asset, and component. During implementation the scene + go block
alone clearly passed the agent-value threshold (`exec` dependence
drops sharply), so we shipped it now rather than batching everything
behind a longer milestone. asset / component arrive in v0.3.x.

### Added

**Stable GameObject IDs.** New `StableIdRegistry` in the Connector
hashes Unity's `GlobalObjectId` down to compact `go:XXXXXXXX` strings.
Eight hex chars by default (32 bits, ~1/86 collision chance at 10,000
GameObjects), extending to 10/12/14/16 chars on collision, with a
40-char SHA1 fallback plus warning for the pathological case. IDs are
deterministic across Editor restarts: the same GameObject always hashes
to the same prefix, so agents can persist `go:` references between
sessions and re-resolve them later. Unknown or destroyed targets
resolve cleanly to `(false, null)` without throwing.
([docs/ROADMAP.md#phase-2a](./docs/ROADMAP.md#phase-2a-v030--observe-scene--gameobject))

**`scene` namespace (6 subcommands).** Scene-level operations without
`exec`. Every subcommand emits structured JSON under `--json`:
- `scene list` — every `.unity` asset in Assets + Packages, annotated
  with build-settings membership and index, sorted by path.
- `scene active` — path / guid / dirty / loaded / root_count /
  build_index / is_untitled for the currently active scene.
- `scene open <path> [--force]` — switch active scene. Blocked during
  play mode; refuses when the current scene has unsaved changes unless
  `--force` discards them.
- `scene save` — saves every open scene that is currently dirty and
  reports which paths were actually written.
- `scene reload [--force]` — re-opens the active scene path. Same
  play-mode and dirty-scene guards as `open`.
- `scene tree [--depth N] [--active-only]` — JSON dump of the active
  scene hierarchy. Every node carries `{ id: go:XXXXXXXX, name, active,
  components, children }`. Depth -1 is unlimited, 0 is roots only.

**`go` namespace (3 subcommands).** GameObject queries keyed by
stable IDs:
- `go find [--name PAT] [--tag T] [--component C] [--active-only]
  [--limit N --offset M]` — search loaded scenes. Filters are ANDed;
  `--name` is a case-insensitive glob (`*` wildcard). Returns compact
  entries `{ id, name, active, tag, layer, path }`. Results are sorted
  by hierarchy path so paginated calls are deterministic across pages.
- `go inspect go:XXXXXXXX` — full dump of one GameObject: scene, path,
  `parent_id`, `children_ids`, and every component with its serialized
  properties typed (see SerializedInspect below).
- `go path go:XXXXXXXX` — hierarchy path string (`Root/Child/Leaf`).

**`SerializedInspect` utility.** Converts a `Component` to a
JSON-shaped object via `SerializedObject` (what the Unity Inspector
shows). Transform special-cases world + local coordinates plus
`sibling_index` / `child_count`. Every `SerializedPropertyType` maps
to a typed JSON shape:
- Vector2/3/4, Quaternion → `{x, y, z[, w]}`
- Color → `{r, g, b, a}`
- Bounds / BoundsInt, Rect / RectInt → explicit shape
- Enum → `{value, name}` with safe fallback when the enum is broken
- ObjectReference / ExposedReference → `{type, name, path, guid}`
- ManagedReference → `{type, id}`
- Generic structs → one level of nested fields
- Arrays clipped at 20 elements with `{count, elements, truncated}`
- Missing-script slots render as `"<Missing Script>"` so stale prefabs
  are detectable without scraping the console.

**Pagination, introduced.** `go find` accepts `--limit N --offset M`
(default limit 100, max 1000). Responses include `total`, `offset`,
`limit`, `returned`, `has_more` so agents can iterate predictably.
First real use of Phase 2's cross-cutting pagination requirement —
later `asset find` will follow the same shape.

**Error code `UCI-042 GameObjectNotFound`.** Emitted by `go inspect`
and `go path` when the stable ID is not in the current session's
registry (prior session, domain reload cleared in-memory state) or
the GameObject has been destroyed. Error message tells the agent to
re-scan via `go find` or `scene tree` before retrying, so callers do
not loop forever on a permanently-dead reference. Registered in
[docs/ERROR_CODES.md](./docs/ERROR_CODES.md) alongside a refreshed
agent decision flow.

### Changed

- **Connector bumped to `0.2.0`** (`udit-connector/package.json`).
  Reflects the three new C# tools (`ManageScene`, `ManageGameObject`,
  `SerializedInspect`), the `StableIdRegistry` utility, and the new
  shared `Common/` directory. CLI and Connector still version
  independently; they happen to synchronize here but will not in
  general.
- **Shell completion** (`bash`, `zsh`, `powershell`, `fish`) learns
  `scene` + six subcommands and `go` + three subcommands. The sentinel
  markers from v0.2.1 are preserved, so re-running `udit completion X
  >> $PROFILE` still replaces the block safely.
- **Help text** in `udit --help` now includes dedicated **Scene** and
  **GameObjects** sections with examples; `udit scene --help` and
  `udit go --help` cover every subcommand, flag, and failure mode.
- **README.md / README.ko.md** gained Scene and GameObjects sections
  describing the stable-ID concept and every new subcommand, kept in
  lockstep per the bilingual doc policy.

### Design notes

- **`Manage<Namespace>` + `action` parameter, not one UditTool per
  subcommand.** The ROADMAP sketch suggested `SceneTools.cs` with
  separate `scene_list` / `scene_open` tools. Sticking with the
  existing `ManageEditor` / `ManageProfiler` pattern turned out to be
  meaningfully cheaper: one tool definition, one Parameters class, one
  help entry, one switch. Easier to read and maintain for a solo
  project.
- **Dirty-scene guard refuses by default.** `EditorSceneManager.
  OpenScene` silently discards unsaved edits when called from
  automation. Requiring `--force` to acknowledge the discard prevents
  accidental data loss — an agent that wanted the old behavior has to
  opt in explicitly, and the guard reports the current dirty path in
  the error payload so the caller can choose to `scene save` first.
- **Tree / find results are deterministically ordered** so pagination
  is stable. `scene tree` walks transforms in their hierarchy order;
  `go find` sorts by hierarchy path. Same search two seconds apart
  returns the same entries in the same order unless the scene actually
  changed.

## [0.2.1] - 2026-04-14

Patch release — no API changes, but two foot-guns removed. Safe drop-in
upgrade from v0.2.0.

### Fixed
- **Shell completion scripts now safe to re-install.** Running
  `udit completion powershell >> $PROFILE` (or the bash/zsh equivalent)
  a second time used to duplicate the entire Register-ArgumentCompleter
  block, corrupting the shell init file and breaking every new session
  until the duplicate was manually removed. Completion scripts are now
  wrapped in sentinel comments (`# >>> udit completion >>>` ...
  `# <<< udit completion <<<`) and README.md documents the
  sed/PowerShell one-liner that removes the old block before appending
  a fresh one. Discovered on Windows PowerShell during v0.2.0 testing.

### Changed
- **CI: drop ALL Node 20 actions in Release workflow** so tag pushes keep
  building after GitHub removes Node 20 from runners on 2026-09-16.
  Verified each action's `using:` field instead of trusting the
  deprecation warning text (which only mentioned three of the five
  actions). Final `release.yml` versions, all confirmed `using: node24`:
  - `actions/checkout` v4 → v5
  - `actions/setup-go` v5 → v6
  - `actions/upload-artifact` v4 → v6 (v5 was still node20)
  - `actions/download-artifact` v4 → v7 (v5 and v6 were both node20)
  - `softprops/action-gh-release` v2 → v3 (v2 was node20; v3 release
    notes confirm "runtime move only — no input/output changes")
  `ci.yml` was already on v5/v6 plus `golangci-lint-action@v9` (node24);
  no change there. This release is the first real-runner verification
  that the bumped Release workflow actually builds all five platforms.

## [0.2.0] - 2026-04-14

Foundation release — first functional iteration after the v0.1.0
rebranding baseline. See [docs/ROADMAP.md](./docs/ROADMAP.md) > Phase 1
for the full plan.

### Fixed
- **ExecuteCsharp** now kills the `csc` process when compilation exceeds 30s,
  preventing orphan processes from accumulating across long sessions.
  ([Phase 1.1](./docs/ROADMAP.md#11-크리티컬-버그-픽스-from-unity-cli-분석))
- **EditorScreenshot** caps width/height at 8192 to prevent OOM crashes from
  accidental huge values, and rejects non-positive dimensions outright.
- **CommandRouter** rejects most commands while Unity is compiling or
  asset-importing, returning an actionable retry message instead of hanging
  or crashing mid-reload. `list` (read-only) remains allowed.

### Added
- **Shell completion** (Phase 1.5). New `udit completion <shell>` command
  emits a static completion script for `bash`, `zsh`, `powershell`, or `fish`.
  Tab-completes top-level commands, sub-actions for `editor` / `profiler` /
  `completion`, and global flags (`--port` / `--project` / `--timeout` /
  `--json` / `--help`). Custom `[UditTool]` handlers aren't auto-discovered
  because completion runs without a live Unity to query — the static built-in
  list covers daily typing.

  Install examples:
  ```
  bash:       source <(udit completion bash)
  zsh:        source <(udit completion zsh)
  powershell: udit completion powershell | Out-String | Invoke-Expression
  fish:       udit completion fish > ~/.config/fish/completions/udit.fish
  ```
- **`.udit.yaml` config file** (Phase 1.4). Walks from cwd upward (stopping
  at `$HOME` exclusive, then filesystem root) and applies project-wide
  defaults. CLI flags always win over config; config wins over built-in
  defaults. Supported keys today:
  ```yaml
  default_port: 8590           # used unless --port is set
  default_timeout_ms: 120000   # used unless --timeout is set
  exec:
    usings:                    # prepended to every `udit exec --usings`,
      - Unity.Entities         # de-duplicated against the CLI list
      - MyGame.Core
  ```
  Invalid YAML emits a warning to stderr and falls back to defaults — never
  blocks the command. 6 unit tests cover discovery, walk, home-stop,
  parse-failure, and exec-usings merge semantics.
- **Global `--json` flag** (Phase 1.2). When set, every command emits a
  uniform machine-readable envelope to stdout (success) or stderr (failure):
  ```json
  {
    "success": true,
    "command": "status",
    "message": "Unity (port 8590): ready",
    "data": {...},
    "error_code": "UCI-...",   // omitted on success
    "unity": {                 // CLI-side meta — port, project, state, version
      "port": 8590,
      "project": "E:/Games/MyGame",
      "state": "ready",
      "version": "6000.4.2f1"
    }
  }
  ```
  CLI-side failures (no Unity running, network errors, timeouts) are
  classified into `UCI-001`/`UCI-002`/`UCI-003` via `classifyGoError`.
  Connector-side errors propagate their own code (Phase 1.3) through
  `client.CommandResponse.ErrorCode`. Legacy text mode is the default
  and unchanged. New tests cover `--json` parsing in `splitArgs`.
- **Error code registry** (Phase 1.3). `ErrorResponse` now carries an optional
  `error_code` field (serialized as `error_code`, omitted when null) so agents
  can branch on a stable identifier instead of fragile message-text matching.
  Codes registered: `UCI-001` NoUnityRunning, `UCI-002` ConnectionRefused,
  `UCI-003` CommandTimeout, `UCI-010` UnknownCommand, `UCI-011` InvalidParams,
  `UCI-020` UnityCompiling, `UCI-021` UnityUpdating, `UCI-030` ExecCompileError,
  `UCI-031` ExecRuntimeError, `UCI-040` AssetNotFound (reserved for Phase 2),
  `UCI-041` SceneNotFound (reserved for Phase 2), `UCI-999` Unknown fallback.
  CommandRouter, HttpServer, ExecuteCsharp, EditorScreenshot now emit codes
  on their error paths.

### Changed
- **buildParams (Go CLI)** distinguishes "switch" flags (`--wait`) from value
  flags (`--key value`). Previously `--filter true` was wrongly coerced to
  bool true because the value happened to be the literal "true". Now string
  values stay strings regardless of content; switches still produce bool true.
  All existing tests pass; new regression tests cover the literal `"true"` /
  `"false"` string cases and switch-flag behavior.
- **EditorScreenshot** uses `FindAnyObjectByType<Camera>()` on Unity 2023.1+,
  replacing the now-deprecated `FindFirstObjectByType<Camera>()` (CS0618).
  This is a pure "any camera" fallback when `Camera.main` is null, so the
  no-ordering semantics are correct.

### Internal
- Korean documentation policy: README.md / docs/ROADMAP.md / docs/ERROR_CODES.md
  now have `.ko.md` siblings synced in lockstep. Policy recorded in CLAUDE.md.
- CLAUDE.md adds a "Windows Store Claude Desktop sandbox" section after
  losing two hours to MSIX file-system virtualization redirecting
  `%LOCALAPPDATA%\udit\` writes into the Claude package container.

## [0.1.0] - 2026-04-14

### Forked from
[unity-cli](https://github.com/youngwoocho02/unity-cli) v0.3.9 by DevBookOfArray, with explicit permission.
See [NOTICE.md](./NOTICE.md) for full attribution.

### Changed (rebranding only — no functional changes vs. upstream v0.3.9)
- Go module path: `github.com/youngwoocho02/unity-cli` → `github.com/momemoV01/udit`
- Binary name: `unity-cli` → `udit`
- Unity package id: `com.youngwoocho02.unity-cli-connector` → `com.momemov01.udit-connector`
- Unity package folder: `unity-connector/` → `udit-connector/`
- C# namespace: `UnityCliConnector` → `UditConnector`
- C# attribute: `[UnityCliTool]` / `UnityCliToolAttribute` → `[UditTool]` / `UditToolAttribute`
- Instance/heartbeat directory: `~/.unity-cli/` → `~/.udit/`
- Default HTTP port: `8090` → `8590` (coexists with unity-cli)
- Default git branch: `master` → `main`

### Removed
- `README.ko.md` (Korean README — English `README.md` is the single source going forward)

### Notes
This release is a clean rebranding baseline. No behavior changes versus upstream. Subsequent releases (`0.2.0` onward) will introduce functional additions per the roadmap.
