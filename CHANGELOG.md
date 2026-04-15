# Changelog

All notable changes to **udit** are documented here. This project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html) and [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

## [0.7.1] - 2026-04-15

Clears the two `build player` options deferred in Phase 4c (v0.5.0).
No new command surface — just `build player` gains `--il2cpp` /
`--no-il2cpp` and `--config <preset>` on top of what v0.5.0 shipped.

Connector bumped to **0.8.1** — adds IL2CPP scripting-backend set/restore
inside `ManageBuild`. Same pipeline otherwise; no protocol changes.

### Added

**`udit build player --il2cpp` / `--no-il2cpp`**

Temporarily flip `PlayerSettings.ScriptingBackend` to IL2CPP (or back to
Mono) for the duration of one build. The previous backend is captured
before the flip and restored in `finally`, so Mono-only projects don't
inherit a permanent IL2CPP setting.

- Scoped to the build's `NamedBuildTarget` (`FromBuildTargetGroup(
  buildOptions.targetGroup)`), so flipping to IL2CPP for a Windows
  build does not touch Android or iOS scripting backends.
- If the backend is already IL2CPP (or the caller passes `--il2cpp`
  with an already-IL2CPP project), no flip + no restore — the asset
  is untouched.
- **Caveat**: if the Editor crashes mid-build the `finally` never
  runs and `ProjectSettings.asset` is left in the flipped state.
  Best-effort; a manual revert from VCS recovers, or re-run with
  the desired backend.

**`udit build player --config <preset>`**

Load build defaults from `.udit.yaml`'s new `build.targets.<preset>`
section; CLI flags always override preset fields.

```yaml
build:
  targets:
    production:
      target: win64
      output: Build/prod/MyGame.exe
      scenes: [Assets/Scenes/Boot.unity, Assets/Scenes/Main.unity]
      il2cpp: true
      development: false
    dev:
      target: win64
      output: Build/dev/MyGame.exe
      development: true
```

```bash
udit build player --config production
udit build player --config production --output Build/custom/x.exe
udit build player --config production --no-il2cpp  # override preset
```

Unknown preset names get an error that lists `Available:` presets —
actionable in one cycle rather than requiring the user to open yaml.

### Implementation

- `udit-connector/Editor/Tools/ManageBuild.cs`:
  * Imports `UnityEditor.Build` for `NamedBuildTarget`.
  * New `il2cpp` param processed after the BuildPlayerOptions build.
  * `SetScriptingBackend` wrapped in try/finally; initial capture
    stores the previous backend only if a flip actually happened.
- `udit-connector/package.json`: 0.8.0 → 0.8.1.
- `cmd/build.go`:
  * `BuildCfg` + `BuildPreset` types (defined in-package; not
    extracted to `internal/` because the yaml-parse-and-merge logic
    lives next to the command that consumes it).
  * `resolveBuildPreset(name)` — looks up a preset with helpful
    error messages: "no .udit.yaml loaded" / "no build.targets"
    / "no such preset. Available: …".
  * `pickString(...)` helper drives the layered merge: CLI flag
    wins over preset field, preset field wins over default.
  * `--il2cpp` / `--no-il2cpp` and `--development` / `--no-development`
    flag pairs so explicit override is distinguishable from absent.
- `cmd/config.go`: `Config` struct gains `Build BuildCfg`.
- `cmd/root.go`: overview help + `printTopicHelp("build")` cover the
  two new flags. Preset schema example inline.
- Test coverage: preset parse, preset resolution (no cfg / no
  section / unknown preset / found), CLI+preset merge, explicit
  `--no-il2cpp` overrides preset `il2cpp: true`, plain `--il2cpp`
  without a preset.

### Upgrade notes

`--il2cpp` requires **Connector ≥ 0.8.1**. With an older connector
the param is silently ignored (connector's ToolParams unpacker
drops unknown keys). Upgrade the UPM package to get the actual
behavior.

## [0.7.0] - 2026-04-15

Phase 5.2 lands. `udit log tail` — a long-lived SSE stream of Unity
console messages delivered as they happen. Complements `udit watch`
(file-change automation) and rounds out Phase 5 (Stream) minus the
`udit run` script runner still to come.

Connector bumped to **0.8.0** — new `/logs/stream` HTTP endpoint,
`Application.logMessageReceived` subscription, ring buffer, multi-
client fanout.

### Added

**`udit log tail` — live SSE stream of Unity console.**

```bash
udit log tail                              # live, all levels, user-filtered stack
udit log tail --type error,warning         # restrict log levels
udit log tail --since 5m                   # backfill last 5min then go live
udit log tail --filter "NullReference"     # client-side regex
udit log tail --json | jq '.level'         # NDJSON for pipelines
```

- `--type CSV` — `error,warning,log,assert,exception` (default: all).
- `--stacktrace MODE` — `none | user | full` (default: `user`).
- `--since DURATION` — `5m`, `30s`, `1h30m`. Live-only when omitted.
- `--filter REGEX` — client-side body regex; drops non-matching events.
- `--json` — NDJSON on stdout (OR with global `--json`).
- `--verbose` — connection notices on stderr.
- `--no-color` — disable ANSI even on a TTY.

**`udit log list` — alias for `udit console`** (snapshot). Same tool,
same flags; kept for vocabulary consistency with `log tail`. The
original `udit console` continues to work unchanged as a permanent
synonym.

### Implementation

- `udit-connector/Editor/LogStream.cs` (new, ~500 lines):
  * `[InitializeOnLoad]` static class; subscribes to
    `Application.logMessageReceived` (main thread only — no threaded
    variant; simpler + race-free).
  * `ConcurrentQueue<BufferedEvent>` ring buffer, 2000 entries,
    drop-oldest with dropped-counter; synthetic `dropped: N` marker on
    next drain so clients know history was lost.
  * `ConcurrentDictionary<Guid, ClientContext>`; drain on
    `EditorApplication.update` with per-client per-tick cap of 500
    events (guard against log-storm freezing Editor).
  * 4-case `--since` handling: live-only / full window / partial +
    `truncated` marker / empty buffer; parse via `since_ms` query.
  * Query-string filter parsing (`types=`, `stacktrace=`, `since_ms=`)
    with HTTP 400 + UCI-006 InvalidStreamFilter on bad values.
  * 30s keep-alive comment frames (`: ping`) survive proxy idle
    timeouts.
  * Stack-trace file/line extractor handles both Editor
    `(at path:N)` and IL2CPP/Mono ` in path:N ` formats.
  * `[ThreadStatic]` re-entrance guard + "never Debug.Log from inside
    the handler" discipline against recursive logging.
- `udit-connector/Editor/Core/LogEvent.cs` (new): serializable struct
  (t, type, message, stack, file, line) with per-field 16KB cap +
  `…[truncated]` suffix.
- `udit-connector/Editor/HttpServer.cs`:
  * SSE branch in `HandleRequest` — **after** the Origin check
    (security: blocks `EventSource()` from a malicious local web
    page siphoning Unity logs).
  * `StopListener` calls `LogStream.OnBeforeReload()` **first**, then
    tears down the listener — deterministic teardown ordering so
    clients get a clean `event: reload` marker, not a raw TCP reset.
    Doesn't rely on `AssemblyReloadEvents` delegate invocation order.
- `internal/client/stream.go` (new): `StreamLogs(ctx, inst, filter,
  connectTimeout)` opens SSE; `Transport.ResponseHeaderTimeout` bounds
  the handshake, body reads are ctx-bound. `DisableKeepAlives` on
  the Transport — each reconnect is a fresh socket. `ParseSSEStream`
  runs the scanner loop (128KB max frame; keep-alive skip; multi-line
  `data:` join). Two sentinel errors: `ErrStreamInterrupted` (UCI-004),
  `ErrConnectorTooOld` (UCI-007).
- `cmd/log.go` (new): CLI dispatch for `log tail` / `log list`;
  exponential reconnect with explicit success rule (≥5s open + 1
  frame OR reload marker → reset to 1s); signal-cancelled context;
  color-coded formatter via `golang.org/x/term.IsTerminal`; NDJSON
  mode.
- `cmd/root.go`: `case "log":` dispatch; overview + topic help.
- `cmd/output.go`: `classifyGoError` maps new error codes UCI-004 /
  UCI-006 / UCI-007.

### Error codes

- **UCI-004 StreamInterrupted** — live connection dropped (EOF,
  reload marker, `net.OpError`). Retryable; `udit log tail` handles
  internally with 1s→2s→4s backoff.
- **UCI-006 InvalidStreamFilter** — HTTP 400 from connector for
  unknown `type`/`stacktrace`/`since_ms` value. Non-retryable.
- **UCI-007 ConnectorTooOld** — response `Content-Type` was not
  `text/event-stream`. Connector < 0.8.0 doesn't speak SSE.
  Short-circuits the reconnect loop.

### Dependencies (Go)

- `+ golang.org/x/term v0.42.0` (direct) — TTY detection for color gating.
- `golang.org/x/sys` upgraded to v0.43.0 (transitive).

### Upgrade notes

`udit log tail` requires **Connector ≥ 0.8.0**. Older connectors return
HTTP 400 for `GET /logs/stream`; the CLI detects this and surfaces
UCI-007 with guidance. Install the new version via the Unity Package
Manager (or git URL if you're tracking `main`).

## [0.6.4] - 2026-04-15

Brings `udit watch` in line with the v0.6.3 `udit init` change: config
resolution now tries the connected Unity instance first. Reported: ran
`udit init --watch` from System32, got the scaffold at the right place,
then `udit watch` failed because it only walked up from cwd (still
System32). Both commands now share the same 4-layer strategy.

### Changed

`udit watch` config resolution (mirrors init exactly):

1. `--config PATH` — explicit override (unchanged).
2. **Connected Unity instance** — if `<inst.ProjectPath>/.udit.yaml`
   exists, load it. Honors global `--port` / `--project` just like
   every other udit command. `ProjectPath="override"` sentinel (emitted
   when `--port` skips the heartbeat read) is filtered out and falls
   through.
3. Walk up from cwd for a `.udit.yaml` (v0.6.0 behavior).
4. Error — actionable message listing both layers that were checked
   and pointing at `udit init --watch` / `--config PATH`:
   ```
   Error: no .udit.yaml found (walking up from C:\WINDOWS\System32;
     C:\Projects\MyGame (connected Unity project — no .udit.yaml there))
     Run `udit init --watch` to generate one, or pass --config PATH.
   ```

### Tests

`cmd/watch_test.go` added:
- `TestLoadWatchConfig_UsesConnectedInstance` — live heartbeat + config
  at the project root → used over cwd walk-up.
- `TestLoadWatchConfig_FallsBackToWalkUp` — heartbeat present but no
  config in the project → walk-up takes over.
- `TestLoadWatchConfig_ErrorListsBothLayers` — nothing found anywhere;
  error names both the walk-up path and the missing-Unity state.
- `TestLoadWatchConfig_ExplicitConfigWins` — `--config` bypasses the
  instance layer.

## [0.6.3] - 2026-04-15

Extends `udit init`'s target resolution with a new first-choice layer:
**the Unity instance `udit status` is already connected to**. Running
from an unrelated shell — another project's directory, a detached VS
Code terminal, System32 — now lands the config at the project you're
actually working on, the same way `scene`, `go`, `component` target the
connected editor. Reported from real use: "can't `init` pick the
project `status` reports?"

### Changed

`udit init` (no `--output`) resolution is now a 4-layer chain:

1. Connected Unity instance — honors global `--port` / `--project`.
   `udit --project MyGame init` picks a specific editor when several
   are open. Same discovery path as every other udit command.
2. Filesystem walk-up (Assets/ + ProjectSettings/). v0.6.2 behavior,
   now the fallback instead of the primary.
3. cwd fallback.
4. `--output PATH` overrides all three (unchanged).

The success message shows the layer that won:
- `Wrote C:\Projects\MyGame\.udit.yaml (connected Unity at port 8590)`
- `Wrote C:\dev\MyGame\.udit.yaml (Unity project root detected (filesystem))`
- `Wrote C:\somewhere\.udit.yaml (no Unity connection / project detected — using cwd)`

### Edge case

`--port N` makes `DiscoverInstance` return `ProjectPath="override"`
(it skips the heartbeat read when a port is forced). The init
resolver explicitly filters that sentinel and falls through to the
filesystem layer — `udit --port 8590 init` lands at the real project
on disk, not a literal "override" directory.

### Tests

- `TestResolveInitTarget_UsesConnectedUnityInstance` — writes a fake
  heartbeat with the test process's PID, verifies init targets the
  project path from it regardless of cwd.
- `TestResolveInitTarget_PortOverrideDoesNotUseStubPath` — ensures
  the "override" sentinel doesn't leak into the target.
- Existing tests now use `isolateInstances(t)` helper so they don't
  accidentally connect to whatever Unity happens to be running on the
  developer's machine.

## [0.6.2] - 2026-04-15

Fixes the `udit init` default target so it lands at the **Unity project
root** instead of blindly writing to cwd. Reported when a user ran
`udit init --watch` from `C:\WINDOWS\System32` and hit "Access is
denied" — the previous behavior wrote to whatever cwd happened to be,
which is wrong for a Unity-centric tool.

### Changed

- `udit init` without `--output` now:
  1. walks up from cwd looking for a directory with BOTH `Assets/` and
     `ProjectSettings/` (same heuristic `udit watch` uses)
  2. falls back to cwd when no Unity project is found
  3. `--output` overrides both (unchanged)
- Success message prints how the target was chosen, e.g.
  `Wrote C:\Projects\MyGame\.udit.yaml (Unity project root detected)`
  so users aren't guessing where the file landed.
- Write failures now surface actionable guidance suggesting the Unity
  project directory or `--output`, instead of the raw OS error.

### Fixed

- `detectProjectRoot(cwd)` used to call `filepath.Dir(cwd)` unconditionally
  (assumed `startHint` was always a file path like the discovered
  `.udit.yaml`). Now stats `start` and only steps to parent when it's a
  file — `init` passes cwd directly.

## [0.6.1] - 2026-04-15

Closes the first-use UX gap in v0.6.0: `udit watch` requires a
`.udit.yaml`, but until now agents had to hand-write one from the help
text. Ships `udit init` — a tiny `git init` / `npm init`-style scaffold
for udit's own config, parallel to the existing surface for generating
Unity assets (`go create`, `component add`, `asset create`, `prefab
instantiate`).

No connector bump — CLI-only.

### Added

**`init` command — scaffold `.udit.yaml`.**

```bash
udit init                      # minimal scaffold with commented-out sections
udit init --watch              # + a watch: section with compile_cs + reserialize_yaml hooks
udit init --output ./my.yaml
udit init --force --watch      # overwrite an existing config
```

The minimal template is a documented starting point: commented globals
(`default_port`, `default_timeout_ms`, `exec.usings`) and a commented
watch stub so users can see the shape before uncommenting. `--watch`
flips the watch section from comment to concrete — two hooks (`refresh
--compile` on `Assets/**/*.cs`, `reserialize $RELFILE` on
`Assets/**/*.{prefab,unity,asset}`) that run against any Unity project
without edits.

- `cmd/init.go` — new subcommand + scaffoldTemplate() + help text.
- `cmd/root.go` — dispatch + overview + topic help.
- `cmd/init_test.go` — minimal scaffold parses, `--watch` scaffold
  validates against `watch.Validate()`, `--force` required for
  overwrite, default `--output` lands in cwd.

## [0.6.0] - 2026-04-15

Opens Phase 5 (Stream). A single deliverable: `udit watch` — a
long-running file-system watcher that runs pre-defined sub-commands
from `.udit.yaml` when matching files change. Zero LLM calls per
event; local CI-style automation inside the editor loop.

No connector version bump — watch is CLI-only. Connector stays at
`0.7.0`.

### Added

**`watch` command — `.udit.yaml`-driven file-change automation.**

```bash
udit watch                     # run hooks from .udit.yaml
udit watch --config foo.yaml   # explicit config path
udit watch --no-exec           # preview what would run
udit watch --json              # NDJSON event log on stdout
udit watch --verbose           # extra diagnostic log
```

Config extension (`watch:` section inside `.udit.yaml`):

```yaml
watch:
  debounce: 300ms
  on_busy: queue          # queue (default) | ignore
  max_parallel: 4
  case_insensitive: true  # default true on Windows
  ignore:
    - "**/*.generated.cs"
  defaults_ignore: true   # Library/Temp/Logs auto-ignored
  hooks:
    - name: compile
      paths: [Assets/**/*.cs, Packages/**/*.cs]
      run: refresh --compile
    - name: reserialize
      paths: [Assets/**/*.prefab, Assets/**/*.unity]
      run: reserialize $RELFILE
      run_on_start: false
      debounce: 500ms     # hook-level override
      on_busy: queue
```

**Variable expansion in `run` strings** (per-token policy, no silent
data loss):

- `$FILE` / `$RELFILE` → per-file invocation (N-file batch fires the
  hook N times, serialized).
- `$FILES` / `$RELFILES` → single invocation, paths injected via env
  `UDIT_CHANGED_FILES` / `UDIT_CHANGED_RELFILES` (newline-separated).
  Left literal in argv to avoid the Windows 8191-char argv limit.
- `$EVENT` — dominant event type: `create` / `write` / `remove` /
  `rename`.
- `$HOOK` — hook name (useful for logging).
- Using both `$FILE`-class AND `$FILES`-class in one `run` is a
  config-load error (pick per-file or batch dispatch).

**Safety mechanisms**:

- **Circuit breaker**: 10 fires in 10 seconds disables the hook
  (self-trigger loop protection) with an explicit log suggesting the
  ignore patterns to add.
- **max_parallel**: global semaphore caps concurrent hook executions.
- **`.meta` sibling collapse**: Unity rewrites `.meta` files slightly
  after the real asset; the debouncer merges them into one logical
  event. Orphan `.meta` (sibling missing on disk) surfaces as a
  remove on the real asset path.
- **Signals**: `Ctrl+C` once drains in-flight hooks + exits cleanly;
  within 2 seconds a second `Ctrl+C` force-quits.

### Implementation

- `internal/watch/` — new package:
  - `config.go` — `WatchCfg` / `Hook` types (embedded in `cmd.Config`).
  - `ignore.go` — built-in Unity defaults (Library/, Temp/, Logs/, …)
    plus user patterns via `doublestar` glob matching.
  - `matcher.go` — stateless hook-to-path matcher.
  - `debounce.go` — per-file timer map + `.meta` collapse.
  - `watcher.go` — fsnotify wrap with recursive walk-and-add (fsnotify
    does not recurse on its own) and `WithBufferSize` tuning for
    Windows `ReadDirectoryChangesW`.
  - `runner.go` — per-hook worker goroutine with queue/ignore
    policies + sliding-window circuit breaker.
  - `expand.go` — variable substitution + POSIX-like arg tokenizer.
  - `clock.go` — `Clock` interface for deterministic debounce tests.
- `cmd/watch.go` — CLI entry; shells out to the same `udit` binary
  via `os.Executable()` for each hook invocation (no in-process
  recursive dispatch; keeps `flag.CommandLine` / `os.Exit` / config
  state clean between hooks).
- `cmd/root.go` — `case "watch":` added **above** the
  `DiscoverInstance` call so watch can start without Unity running.
- `cmd/config.go` — `Config.Watch watch.WatchCfg` field (yaml.v3
  silently ignores the `watch:` section in configs without it).

New dependencies:

- `github.com/fsnotify/fsnotify v1.9.0` (MIT)
- `github.com/bmatcuk/doublestar/v4 v4.10.0` (MIT, stdlib-only)

### Deferred to v0.6.x

- `udit watch --path P --on-change C` ad-hoc mode (no config file).
- `watch reload` hot-reload of `.udit.yaml` (MVP logs a notice when
  the config file changes but does not reload).
- `on_busy: restart` policy (Windows Process.Kill + pipe drain +
  orphan subprocess handling deferred until user demand justifies
  complexity).

### Upgrade notes

- `.udit.yaml` remains fully backwards-compatible; configs without a
  `watch:` section are unaffected.
- `udit watch` is a new subcommand — no existing workflow behavior
  changes.

## [0.5.0] - 2026-04-15

Closes Phase 4 (Automate). Two new tool surfaces — `package` (UPM)
and `build` (player + addressables) — plus a long-overdue
.meta GUID separation from upstream unity-cli that finally lets the
two connectors coexist in the same Unity project. With this release,
the Automate ROADMAP is fully shipped: agents can drive
project-info → validate → test → package → build end-to-end without
ever touching `exec`.

Connector bumped to `0.7.0` — two new C# tools (`ManagePackage`,
`ManageBuild`) plus the GUID separation. The two new tools are
the visible surface; existing tools have no behavior change.

### Added

**`package` namespace (6 subcommands) — new `ManagePackage` tool.**

```bash
udit package list                              # declared deps from manifest.json
udit package list --resolved                   # resolved graph via Client.List
udit package add com.unity.cinemachine
udit package add com.unity.cinemachine@2.9.7
udit package add https://github.com/dbrizov/NaughtyAttributes.git
udit package remove com.unity.cinemachine
udit package info com.unity.cinemachine
udit package search cinemachine
udit package resolve
```

- `list` (default) parses `Packages/manifest.json` directly — sub-
  second response with `{ name, version_declared, kind }` per entry
  (`kind` = `registry` / `git` / `file`). `--resolved` switches to
  `PackageManager.Client.List` (transitive deps + actual install
  source), 1-3s on a cold registry.
- `add <id>` forwards the id to `Client.Add`. Unity parses the form
  (registry name, pinned version, git URL) — id passes through the
  CLI unchanged. Triggers domain reload on success. Response carries
  resolved `{ name, version, source, package_id }`.
- `remove <name>` is the symmetric `Client.Remove`.
- `info <name>` returns single-package metadata via `Client.Search`:
  current version, latest, latest_release, description, registry,
  recent versions (last 10) — enough to decide what to add without
  an extra round-trip.
- `search <keyword>` substring-matches name + display_name across
  the full registry catalog (`Client.SearchAll`), capped at 50.
- `resolve` calls `Client.Resolve` (with `AssetDatabase.Refresh`
  fallback) — useful after editing manifest.json externally.

All async operations (everything except declared `list`) are polled
on `EditorApplication.update` via a shared `AwaitRequest<TReq>`
helper that mirrors the `TaskCompletionSource` pattern from
`RunTests.cs`. First-slice limitation: domain reloads triggered by
`add`/`remove` can truncate the response if Unity tears down the
HTTP server mid-recompile. Re-running `package list` confirms
post-state. Pending-file + `[InitializeOnLoad]` reload-recovery
(see `TestRunnerState.cs` for the pattern) is the natural follow-up
if this becomes a real friction.

CLI side: `cmd/package.go` dispatches to the `manage_package` tool,
6 actions, with a `firstPositional` helper that mirrors how
`parseSubFlags` consumes flag values so a `--key value` pair isn't
mistaken for the package id. `cmd/package_test.go` covers all 6
actions, all 3 `add` forms, missing-positional rejection for each
action that requires one, the unknown-action path, and the
positional helper edge cases (15 cases, all green).

Help text in `printHelp` (Packages section) + dedicated `package`
topic in `printTopicHelp`. Shell completion (bash / zsh /
powershell / fish) gains the new top-level command + 6
subcommands. README.md / README.ko.md gain a `### Package` section
after `### Project`, kept in lockstep per the bilingual doc policy.

Connector version unchanged (`0.6.2`) — bump deferred to `0.7.0`
when Phase 4c (`build`) lands and v0.5.0 cuts the two together,
matching the established Phase 2/3 day-1-patch cadence.

**`build` namespace (4 subcommands) — new `ManageBuild` tool.**

```bash
udit build targets                                       # discover supported BuildTargets
udit build player --target win64 --output builds/win64/  # standalone player
udit build player --target android --output app.apk \
    --scenes Assets/Scenes/Main.unity,Assets/Scenes/Boot.unity \
    --development
udit build addressables [--profile MobileRelease]        # AddressableAssetSettings.BuildPlayerContent
udit build cancel                                        # BuildPipeline.CancelBuild
```

- `targets` walks every `BuildTarget` enum value and reports
  `{ name, group, supported }` per entry, plus the active target
  and `supported_count`. `supported` reflects
  `BuildPipeline.IsBuildTargetSupported` against the local Editor
  install — agents filter on this before attempting `build player`.
- `player` wraps `BuildPipeline.BuildPlayer`. `--target` accepts
  friendly aliases (win64/win32/mac/linux/android/ios/webgl) plus
  any full enum name (StandaloneWindows64, etc.). `--output`
  resolves relative paths against the CLI's cwd (matches
  `test --output` / `screenshot --output_path`); parent dir is
  auto-created. `--scenes` is comma-separated; when omitted, falls
  back to enabled scenes in Build Settings. `--development` enables
  `BuildOptions.Development`. CLI uses an infinite timeout for
  `build player` so the agent's global `--timeout` doesn't fire
  mid-build.
- `addressables` calls `AddressableAssetSettings.BuildPlayerContent`
  via reflection — the connector takes no hard dependency on
  `com.unity.addressables`. Missing-package case returns a clear
  `UCI-011` pointing at `udit package add com.unity.addressables`.
  `--profile` temporarily switches `activeProfileId`, then restores
  it (best-effort).
- `cancel` calls `BuildPipeline.CancelBuild`. Silent no-op when no
  build is in progress; response always reports success so re-issue
  is safe.

Response shape on `player`: full `BuildReport` summary —
`{ result, platform, output_path, total_size, total_errors,
total_warnings, duration_sec, build_started_at, build_ended_at,
steps_count, scenes_count }`. Failed/Cancelled builds return as
`ErrorResponse` with the same payload so callers don't need to
parse a different shape.

Out of scope for this slice (deferred to a v0.5.x patch):
- `--il2cpp` — needs PlayerSettings.SetScriptingBackend with
  set/restore semantics; failure modes (build crash mid-restore)
  warrant a careful design pass.
- `--config <name>` reading build presets from `.udit.yaml`'s
  `build.targets.<name>` section — adds yaml schema work and
  nested-map merging. Worth its own slice once the CLI surface
  has settled in real use.

CLI side: `cmd/build.go` dispatches to `manage_build`, four actions,
plus a `splitTrim` helper for comma-separated `--scenes` parsing.
`cmd/root.go` registers `case "build":` with a dedicated `buildSend`
that uses `client.Send(..., 0)` (infinite timeout) — same trick
`test` uses for PlayMode runs. `cmd/build_test.go` covers all four
actions, every `player` flag (target/output/scenes/development),
absolute-vs-relative output resolution, scene comma-split with
whitespace + empty-entry handling, missing-required-flag rejections,
unknown-action and empty-args paths, plus the `splitTrim` helper
edge cases (16 cases, all green).

Help text in `printHelp` (Builds section) + dedicated `build`
topic in `printTopicHelp`. Shell completion (bash/zsh/powershell/
fish) gains the new top-level command + 4 subcommands at both
call sites. README.md / README.ko.md gain a `### Build` section
after `### Package`, kept in lockstep per the bilingual doc policy.

### Fixed

**udit-connector .meta GUIDs permanently separated from upstream
unity-cli-connector.** udit forked from unity-cli without renaming
.meta GUIDs, so installing both packages in the same Unity project
caused a GUID conflict. Unity auto-resolved by reassigning all 27
udit-connector GUIDs and writing them back to the file: source —
which kept reappearing as a dirty working tree on every developer
machine that had both connectors installed.

Adopting Unity's new GUIDs as canonical so the two packages can
coexist in the same project without further conflict. Connector
bumped `0.6.1` → `0.6.2`.

Affected files: 27 .meta files under `udit-connector/Editor/` plus
`udit-connector/package.json.meta`. Asset references that survive
this change are unaffected because [UditTool] classes are static
(no GameObject MonoBehaviour references) and the asmdef GUIDs
weren't depended on by external assemblies. CLI users are
unaffected entirely — Go binary unchanged, no CLI tag cut.

## [0.4.3] - 2026-04-15

Interim patch covering the first two slices of Phase 4 (Automate) —
`project` read-only commands and the `test` surface extensions — plus
a path-semantics fix shared by `test run --output` and
`screenshot --output_path`. The remaining Phase 4 blocks (`build`,
`package`) land together in v0.5.0; splitting here keeps the JUnit-XML
→ CI-integration path usable now instead of waiting for the full
automate roadmap.

Connector bumped to `0.6.1` — one new `[UditTool]` (`ListTests`) and
parameter-description tweaks on existing tools; no behavior change
in `RunTests` / `EditorScreenshot` themselves.

### Added

**`project` namespace (3 subcommands) — new `ManageProject` tool.**

```bash
udit project info                    # Unity version, build target, packages, asset stats
udit project validate [--include-packages] [--limit N]
udit project preflight [--include-packages] [--limit N]
```

- `info` — fast one-shot project snapshot: Unity version, active
  build target, render pipeline, product/company/bundle version,
  scripting backend + color space, scenes in Build Settings (with
  enabled flag + index), packages (declared versions from
  `manifest.json`), asset counts (total / cs / scenes / prefabs /
  materials / textures). Manifest-only package read — deliberate
  tradeoff to keep the response sub-second on large projects.
- `validate` — scans for issues an agent should know before acting:
  prefab assets with missing script references (via
  `GameObjectUtility.GetMonoBehavioursWithMissingScriptCount`), Build
  Settings with no enabled scenes. Returns `{ ok, errors, warnings,
  scan_ms, issues[] }`. `--include-packages` widens scope to
  `Packages/`; `--limit` caps issues per severity (default 100).
- `preflight` — validate + pre-build hygiene: compile state
  (`EditorApplication.isCompiling`), empty `productName`,
  `companyName` left at `"DefaultCompany"`. Designed to front-run
  `udit build player` once that lands.

**`test list` and `test run --output`.**

```bash
udit test list [--mode EditMode|PlayMode]   # enumerate without running
udit test run --output junit.xml            # also write JUnit XML
```

- `test list` — read-only walk of the test tree via
  `TestRunnerApi.RetrieveTestList`. Returns `{ mode, total, tests[] }`
  with `{ full_name, name, class_name, type_info, run_state }` per
  leaf. Use `full_name` as the `--filter` value for a follow-up `test
  run`.
- `test run --output <path>` — emits a `testsuites/testsuite/testcase`
  JUnit XML alongside the JSON response. Format is the shape GitHub
  Actions and GitLab CI JUnit parsers expect; failed tests carry
  message + stack inside `<failure>`, Inconclusive/Skipped map to
  `<skipped/>`, tests grouped by class name. Domain-reload-safe —
  the output path threads through `TestRunnerState.MarkPending` so
  a PlayMode run that reattaches post-reload still writes the XML.

### Fixed

**`test run --output` and `screenshot --output_path` now resolve
relative paths against the CLI's cwd, not Unity's project root.**
A relative `--output foo.xml` used to silently land in the Unity
project directory (because Unity is the process doing the write,
with its own working directory). That breaks the POSIX expectation
of `udit <cmd> --output foo.xml` — the file now lands next to where
the caller typed the command. Absolute paths pass through unchanged.
A CLI-side `absolutizePath` helper handles both cases uniformly.
Agents hard-coded to the old behavior should switch to absolute
paths, which have always worked.

### Changed

- **Connector bumped to `0.6.1`** (`udit-connector/package.json`).
  Adds `ListTests` tool; `RunTests.Output` and
  `EditorScreenshot.OutputPath` parameter descriptions updated to
  describe the new CLI-side resolution (C# fallback behavior for
  direct HTTP callers is unchanged).
- **Help text and shell completion** (bash/zsh/powershell/fish) gain
  `project info/validate/preflight` and `test run/list`
  subcommands. `--output`/`--output_path` help clarifies CLI-cwd
  resolution.
- **README.md / README.ko.md** gain a "Project" subsection after
  "Transactions", describe the new `test list` / `--output` surface,
  and note the CLI-cwd path resolution. Kept in lockstep per the
  bilingual doc policy.
- **Internal cleanup**: `cmd/paths.go` holds `absolutizePath` +
  `absolutizePathParam`, shared between `test` (explicit handler)
  and `screenshot` (default passthrough). Pre-existing
  `cmd/asset.go` conditional simplified (De Morgan — `staticcheck
  QF1001`); `.golangci.yml` errcheck exclude adds
  `syscall.CloseHandle` (Windows-only, not actionable under
  `defer`).

## [0.4.2] - 2026-04-15

Closes Phase 3 (Mutate). Third same-day patch in the v0.4.x line,
adding transactions that let agents group multi-step scene edits into
a single Unity Undo entry. With this release, the ROADMAP for Mutate
is fully shipped — agents can observe, mutate, and batch-undo scene
changes end-to-end without ever dropping into `exec`.

Connector bumped to `0.6.0` — one new C# tool (`ManageTransaction`)
added to the existing connector. Small in surface area, large in the
reach it gives the other mutation tools.

### Added

**`tx` namespace (4 subcommands) — new `ManageTransaction` tool.**

```bash
udit tx begin [--name "Spawn boss setup"]
udit go create --name Boss
udit component add go:abcd1234 --type Rigidbody
udit component set go:abcd1234 Rigidbody m_Mass 5.5
udit tx commit                       # single Ctrl+Z now reverses all 3

udit tx begin --name "Try a layout"
udit go create --name Candidate
udit go move go:abcd1234 --parent go:5678abcd
udit tx rollback                     # every change since begin is unwound

udit tx status                       # { active, group, name, duration_ms }
```

- `begin` captures the current Unity Undo group via
  `Undo.IncrementCurrentGroup` + `Undo.GetCurrentGroup`. Optional
  `--name` lands in the Edit → Undo menu after commit.
- `commit` calls `Undo.CollapseUndoOperations(savedGroup)` — every
  Undo sub-group created since begin gets merged into one. A single
  Ctrl+Z in the Editor reverses the entire batch. `--name` at commit
  overrides the begin-time label, handy when the final description
  only crystallises after the work is done.
- `rollback` calls `Undo.RevertAllDownToGroup(savedGroup)` and
  replays the Undo stack back to the pre-begin state in place.
- `status` reports whether a transaction is active and, if so, its
  group / name / elapsed time.

State cost is minimal — three static fields on the connector side
(`group`, `name`, `started`). All real change state lives on Unity's
own Undo stack.

### Constraints (documented in help + README)

- **One transaction per Unity instance.** The Undo stack is global,
  so there's exactly one nesting at a time. `begin` during an active
  tx returns `UCI-011` with the existing transaction's name and age.
- **Domain reload wipes the handle.** Script recompiles tear down the
  static state. Mid-transaction reloads leave partial mutations on
  the Undo stack but drop the tx handle; `tx status` reports no
  active, agent re-begins if they want to keep grouping.
- **AssetDatabase operations don't participate.** `asset create/
  move/delete/label` write straight to disk and can't be collapsed
  into a scene-Undo group. They still execute inside a transaction,
  just not reversibly via commit/rollback. This matches the
  underlying Unity API rather than trying to paper over it.

### Changed

- **Connector bumped to `0.6.0`** (`udit-connector/package.json`).
  New `ManageTransaction` tool only — existing tools are unchanged.
- **Shell completion** (bash/zsh/powershell/fish) learns the new
  top-level `tx` command and its four subcommands.
- **Help text** gains dedicated `udit tx --help` describing the
  begin/commit/rollback/status surface, the three constraints, and
  both typical use cases (single-undo batch, mid-op rollback).
- **README.md / README.ko.md** gain a "Transactions" subsection
  after "Prefabs", kept in lockstep per the bilingual doc policy.

### Design note

**Why Unity-native instead of tracking command history.** An
alternative design would be to have udit record every mutation
executed since `begin` and replay them in reverse on `rollback`.
That breaks three ways:
1. Stateless HTTP: the connector would need to hold a growing
   command log across requests.
2. Asymmetry with Unity's own Undo: a user pressing Ctrl+Z in the
   Editor would reverse operations individually, but `udit tx
   rollback` would only reverse the ones udit tracked.
3. Irreversible operations: `asset create/move/delete` can't be
   exactly undone by issuing the inverse call, and managing partial
   rollback gets complicated fast.

Delegating to Unity's own Undo stack keeps udit's rollback semantics
identical to what Ctrl+Z already gives users, at the cost of
AssetDatabase operations not participating. That tradeoff is called
out in the docs rather than hidden.

## [0.4.1] - 2026-04-15

Same-day patch closing the Phase 3 middle block. v0.4.0 shipped
GameObject + Component mutation; this release fills in the three
remaining commonly-needed mutation paths — ObjectReference writes
(so agents can actually assign sprites/materials/clips via
`component set`), prefab operations, and asset-level create/move/
delete/label. Only transactions are left from Phase 3.

Connector bumped to `0.5.0` — one new C# tool (`ManagePrefab`) plus
two substantial action blocks added to existing tools
(`ManageComponent.ApplyParsedValue` gains ObjectReference;
`ManageAsset` gains Create/Move/Delete/Label).

### Added

**`component set` ObjectReference writes.** Closes the "read works,
write doesn't" asymmetry from v0.4.0.

```bash
udit component set go:X SpriteRenderer m_Sprite Assets/Sprites/Player.png
udit component set go:X Material m_MainTex Assets/Textures/wall.jpg
udit component set go:X Camera m_TargetTexture null
```

Sub-asset auto-pick: `.png` imported as Texture2D + Sprite sub-asset
resolves to the Sprite for `SpriteRenderer.m_Sprite` and Texture2D
for `RawImage.texture` — same path, different assignment, no
sub-asset knowledge needed on the caller's side. Type-compatibility
is checked up front via `SerializedProperty.type`'s `PPtr<$TypeName>`
form; mismatches return `UCI-011` with the expected type and what
was actually found at that path. Scene object references
(`go:XXXXXXXX`) are still read-only in this version and return
`UCI-011` with a pointer to `exec` for now. `"null"` / `"none"` /
`""` all clear the reference.

**`prefab` namespace (4 subcommands) — new `ManagePrefab` tool.**

```bash
udit prefab instantiate Assets/Prefabs/Enemy.prefab --parent go:abcd1234 --pos 5,0,0
udit prefab unpack go:5678abcd --mode completely
udit prefab apply go:5678abcd
udit prefab find-instances Assets/Prefabs/Enemy.prefab
```

- `instantiate` uses `PrefabUtility.InstantiatePrefab` so the scene
  instance keeps its prefab link (unlike `Object.Instantiate`, which
  gives a disconnected copy). `--pos` writes localPosition to match
  `go create`'s convention.
- `unpack` with `--mode root` (default) unpacks the outermost prefab
  root only; `--mode completely` recurses into nested prefabs. When
  the caller points at a nested GO under an instance, unpack and
  apply both auto-resolve to the outermost root — matches what
  Unity's own context menu does.
- `apply` commits the scene instance's overrides back to the prefab
  asset via `PrefabUtility.ApplyPrefabInstance(..., AutomatedAction)`.
- `find-instances` walks every loaded scene and returns outermost
  roots whose `GetCorrespondingObjectFromSource` matches the given
  asset. Read-only, no Undo.

Every mutation subcommand supports `--dry-run`. Mutations are
blocked in play mode and register with Unity Undo (per-op groups, so
Ctrl+Z in the Editor reverses one logical step at a time).

**Stable-ID shifts on unpack** — unpacking changes a GameObject's
`GlobalObjectId` because the prefab link is part of identity, which
in turn changes the id the registry emits. The `unpack` response
returns the new id; the old id starts returning `UCI-042`. This is
Unity's identity model, not a udit choice — surfaced explicitly in
help + README so agents learn it up front.

**`asset` mutation namespace (4 subcommands).** Extends
`ManageAsset`.

```bash
udit asset create --type MyGame.GameConfig --path Assets/Config/
udit asset create --type Folder --path Assets/NewFolder
udit asset move Assets/Old.prefab Assets/New/Moved.prefab
udit asset delete Assets/Unused.prefab               # trash (recoverable)
udit asset delete Assets/Unused.prefab --permanent   # DeleteAsset
udit asset label add    Assets/Prefabs/Boss.prefab boss_content critical
udit asset label remove Assets/Prefabs/Boss.prefab critical
udit asset label list   Assets/Prefabs/Boss.prefab
udit asset label set    Assets/Prefabs/Boss.prefab final
udit asset label clear  Assets/Prefabs/Boss.prefab
```

- `create` handles ScriptableObject-derived types and the sentinel
  `Folder`. `--path` ending in `/` or resolving to an existing folder
  auto-appends `<TypeName>.asset`; an explicit filename overrides.
  Unqualified type names prefer UnityEngine; pass the full namespace
  for project types that would otherwise collide.
- `move` runs `ValidateMoveAsset` first so agents get Unity's own
  diagnostic string (e.g. "Destination path is not within project")
  instead of a generic "returned false". GUID is preserved —
  existing references in the project stay valid.
- `delete` defaults to `MoveAssetToTrash` (OS-trash recoverable).
  `--permanent` uses `DeleteAsset` AND scans the whole project first
  to report `referenced_by: N` on dry-run so the caller sees the
  blast radius before committing.
- `label` sub-ops `add` / `remove` / `list` / `set` / `clear`. The
  CLI sends labels as a comma-joined string; the C# side splits them
  back. `list` is special-cased as read-only.

**Important caveat, documented.** AssetDatabase operations
(`CreateAsset`, `MoveAsset`, `DeleteAsset`, `SetLabels`) do **not**
participate in Unity's scene Undo. Ctrl+Z in the Editor will not
reverse them — this is the underlying Unity API's design, not a
udit choice. The safety nets are `--dry-run` for preview and
`delete` defaulting to the OS trash. README and `udit asset --help`
call this out prominently.

### Changed

- **Connector bumped to `0.5.0`** (`udit-connector/package.json`).
  New `ManagePrefab` tool, two substantial action blocks added to
  `ManageComponent` / `ManageAsset`, and two helpers
  (`ResolveUnityObjectType`, `StripPPtrWrapper`) for the
  ObjectReference write path.
- **Shell completion** (bash/zsh/powershell/fish) learns the new
  `prefab` top-level command with 4 subcommands, plus 4 new `asset`
  subcommands (`create`, `move`, `delete`, `label`).
- **Help text** gains dedicated `udit prefab --help` and expanded
  `udit asset --help`. The `udit component --help` value-parser
  cheat sheet gets an ObjectReference row.
- **README.md / README.ko.md** gain "Component mutation →
  ObjectReference", "Asset mutation", and "Prefabs" subsections,
  kept in lockstep per the bilingual doc policy.
- **Error messaging.** `prefab instantiate` now distinguishes
  "no asset at path" (UCI-040) from "asset exists but isn't a
  GameObject" (UCI-011 with a hint to run `asset inspect`), instead
  of collapsing both into a single "prefab not found" message.

### Design notes

- **Same vocabulary on both sides of `component`.** `component set`
  field names match what `component get` returns, including
  Transform's virtual fields (`position`, `local_position`, etc.)
  and the type-specific parser shapes (Vector/Color/Enum/Ref). An
  agent that can describe a component can also edit it using the
  same identifiers.
- **Prefab mutations live in a separate tool.** `ManagePrefab` could
  have been action arms on `manage_game_object`, but the 4 actions
  all operate on the asset↔instance relationship rather than on a
  GameObject's own state. Separating them keeps both tools'
  surfaces legible and matches how Unity's own menu groups prefab
  operations.
- **Asset safety = preview + trash + blast-radius scan.** Since
  Unity Undo cannot cover AssetDatabase operations, safety is
  pushed to the CLI surface: `--dry-run` on every mutation, default
  `delete` goes to OS trash, and `--permanent` surfaces
  `referenced_by` before committing. Agents that want hard deletes
  have to opt in twice (flag + confirm).

## [0.4.0] - 2026-04-15

First release of Phase 3 (**Mutate**). After this release agents can
build and edit scenes without dropping into `exec` — the full read +
write loop is covered for GameObjects and their components. The next
minor release (v0.4.x) will layer on prefab operations, asset-level
mutations, and multi-command transactions; this release intentionally
ships the bottom half of that stack so the basic authoring loop
(`create GO → addComponent → setField`) is out the door while the
design of the rest gets real feedback.

Connector bumped to `0.4.0` — two substantial new action blocks in C#
plus the Unity 6 API cleanup described below.

### Added

**`go` mutation namespace (5 subcommands).** Extends the existing
`manage_game_object` tool with write operations. Every action is
routed through Unity Undo so Ctrl+Z in the Editor reverses an agent's
change the same way it reverses a human's Inspector edit, and every
action accepts `--dry-run` to preview the impact without touching the
scene.

- `go create --name N [--parent go:P] [--pos x,y,z]` — spawns a
  GameObject and returns its fresh stable ID. Without `--parent` the
  GO attaches at scene root.
- `go destroy <go:ID>` — destroys a GameObject and every descendant.
  Response reports `children_affected` so the caller knows the cascade
  size up front.
- `go move <go:ID> [--parent go:P]` — reparents a GameObject. Omit
  `--parent` to move back to the scene root. Cycle-creating reparents
  (parent under self or descendant) are rejected with `UCI-011`
  *before* the transform changes so Unity cannot crash on the edge.
- `go rename <go:ID> <newname>` — renames in place.
- `go setactive <go:ID> --active true|false` — toggles `activeSelf`.
  Already-in-state calls return success with `no_change: true`,
  deliberately skipping the Undo group so Ctrl+Z doesn't have to
  pop over a no-op.

**`component` mutation namespace (4 subcommands).** Extends
`manage_component`. Field names match what `component get` emits, so
the read/write vocabulary is unified — an agent that can describe a
component can also edit it using the same identifiers.

- `component add <go:ID> --type <T>` — `Undo.AddComponent(go, type)`.
  Respects `DisallowMultipleComponent` and `RequireComponent`. Rejects
  Transform up front ("Every GameObject already has a Transform")
  with a clearer message than `AddComponent` would give.
- `component remove <go:ID> <T> [--index N]` — removes one component.
  Transform is blocked — the error message redirects to `go destroy`.
- `component set <go:ID> <T> <field> <value> [--index N]` — writes one
  field. The value is parsed based on the target's
  `SerializedPropertyType`. Transform's virtual fields (`position`,
  `local_position`, `rotation_euler`, `local_rotation_euler`,
  `local_scale`) set world-space values directly via Transform API so
  the caller does not need to know about `m_LocalPosition`.
- `component copy <go:SRC> <T> <go:DST> [--index N]` —
  `EditorUtility.CopySerialized`. If the destination lacks the type,
  `Undo.AddComponent` runs first; the observable end state is a
  single matching component with the source's values either way.

**Value parser for `component set`.** Parses a single string into the
target field's Unity type:

| SerializedPropertyType | Input |
| --- | --- |
| Integer / LayerMask / ArraySize / Character | `"42"` |
| Boolean | `"true"` / `"false"` / `"1"` / `"0"` / `"yes"` / `"no"` / `"on"` / `"off"` |
| Float | `"3.14"` |
| String | any text |
| Vector2 / 3 / 4 / Quaternion | comma-separated floats |
| Color | `"r,g,b[,a]"` in 0–1 range, or `"#RRGGBB[AA]"` |
| Enum | display name (`"Solid Color"`) or value index |

ObjectReference, AnimationCurve, Gradient, and ManagedReference are
read-only in v0.4.0 and return `UCI-011` with a "read-only in this
version" message — these need asset lookup / keyframe parsing / type
resolution plumbing that fits better in a follow-up slice.

**`--dry-run` on every mutation (cross-cutting).** Both `go` and
`component` mutations accept `--dry-run`. The response shape matches
what a real run would return (`would_destroy`, `children_affected`,
`from`/`to`, etc.) but Unity is not touched. This makes "plan then
execute" a clean one-flag change instead of two command paths.

**Per-mutation Unity Undo groups.** Every mutation starts with
`Undo.IncrementCurrentGroup()` + `Undo.SetCurrentGroupName(...)`
before its first side effect. Unity normally increments the current
group once per editor tick; without the explicit increment, multiple
udit commands fired within the same tick can collapse into one group
and a single Ctrl+Z unwinds a whole agent session at once (or, worse,
cancels a `create + destroy` pair to a no-op). This was discovered
during the first live-test of Phase 3.1 and fixed in the same slice.
Editor's Edit → Undo menu now shows descriptive labels like
`"udit component set Rigidbody.m_Mass"` for each step.

### Changed

- **Connector bumped to `0.4.0`** (`udit-connector/package.json`).
  `ManageGameObject` and `ManageComponent` each grew a full mutation
  block; `SerializedInspect` is unchanged but its output now feeds
  both the read (`component get`) and write (`component set` field
  echo) paths symmetrically.
- **Unity 6 API cleanup.** Several Unity 6 deprecations surfaced
  during the Phase 3.2 live-test and got fixed together:
  - `EditorUtility.CopySerialized` returns `void` in Unity 6 (was
    `bool`). Wrapped in `try`/`catch` so a failure still surfaces as a
    structured `UCI-011` instead of a 500.
  - `Object.FindObjectsByType<T>(FindObjectsInactive, FindObjectsSortMode)`
    is deprecated. Switched `ManageGameObject.Find` to the single-arg
    overload — we sort by hierarchy path locally anyway, so the sort
    mode never mattered.
  - `ShaderUtil.GetPropertyCount/Name/Type` are deprecated in favor of
    the `Shader` instance methods, and the enum moved from
    `ShaderUtil.ShaderPropertyType` to
    `UnityEngine.Rendering.ShaderPropertyType` (with `TexEnv` renamed
    to `Texture`). `ManageAsset.DescribeMaterial` updated accordingly.
- **Shell completion** (bash/zsh/powershell/fish) learns the five new
  `go` subcommands and four new `component` subcommands.
- **Help text** in `udit --help` gains a GameObject-mutation block and
  a Component-mutation block; `udit go --help` and
  `udit component --help` document every new action, the value-parser
  cheat sheet, and every error code an agent can expect.
- **README.md / README.ko.md** gain a "GameObject mutation" subsection
  and a "Component mutation" subsection with the parser cheat sheet,
  kept in lockstep per the bilingual doc policy.

### Design notes

- **Dry-run + Undo together cover the agent's "are you sure?" space.**
  Before a destructive call, the agent can check `--dry-run` to see
  what would change. After a call, Ctrl+Z in the Editor reverses it
  one logical step at a time. Neither feature alone would be as
  useful as both together.
- **Transform virtual fields travel both directions.** `component get
  go:X Transform position` returns world-space `{x,y,z}`; `component
  set go:X Transform position 0,10,0` writes world-space by the same
  name. Agents do not have to learn `m_LocalPosition` for the common
  case. Local-space variants are available under their own names
  (`local_position` etc.).
- **Phase 3 split (3a ships now, 3b follows).** `go` +  `component`
  mutation alone is enough to unblock `create GO → addComponent →
  setField` — the authoring loop most agents need. `prefab`, `asset`
  mutation, and transactions (3b) land in the next patch so their
  design can benefit from real feedback on 3a.

## [0.3.1] - 2026-04-15

Same-day patch closing Phase 2b. Where v0.3.0 covered scene + go, this
release adds **component** (field-level zoom-in on individual properties)
and **asset** (project asset graph: find / inspect / dependencies /
references / guid / path). The agent's read path is now end-to-end
covered without `exec` for every Observe scenario in the ROADMAP.

Connector bumped to `0.3.0` to reflect the two new C# tools and the
`SerializedInspect.ObjectToJson` public addition. CLI and Connector
versions remain independent.

### Added

**`component` namespace (3 subcommands).** Reuses the
`SerializedInspect` converter from v0.3.0, so the field names returned
by `component get` are exactly the ones that show up under each
`go inspect` component entry — agents learn one vocabulary and
re-apply it.

- `component list <go:ID>` — `{ index, type, full_type, enabled }` for
  every component on the GameObject. Lighter than `go inspect` when you
  only need attached types.
- `component get <go:ID> <Type> [field]` — Without a field, dumps every
  visible property. With a dotted field path (e.g. `position`,
  `position.z`, `m_Cameras.elements.0`), navigates the JObject returned
  by SerializedInspect and returns the leaf value. The CLI always
  passes the `field` string through verbatim, so the same vocabulary
  works for nested objects, struct fields, and array indices.
- `component get <go:ID> <Type> --index N` — Pick the Nth attached
  component when multiple of the same type exist (e.g. two BoxColliders).
- `component schema <Type>` — Serialized-property schema for a type:
  `{ name, display_name, property_type, is_array, has_children }`.
  v1 probes an *existing* live instance in the loaded scenes rather than
  spawning one (AddComponent has side effects: RequireComponent chains,
  internal flags). A reflection-only fallback for the "no instance
  anywhere" case is a later slice.

Type-name resolution is case-insensitive. Unqualified names prefer
`UnityEngine.*` when multiple assemblies ship a Component with the same
simple name; pass the full namespace (`MyGame.Camera`) to disambiguate.

**`asset` namespace (6 subcommands).** Project asset graph queries.
All paths are project-relative (`Assets/...` or `Packages/...`); GUIDs
are Unity's 32-char hex strings.

- `asset find` — AssetDatabase query with combinable filters:
  `--type X` maps to Unity's `t:` syntax, `--label X` maps to `l:`,
  `--name <glob>` is a post-filter (since AssetDatabase's free-text
  term is substring not wildcard), `--folder F1,F2,...` scopes the
  search, `--limit N --offset M` paginate. Results sorted by path.
- `asset inspect <path>` — Common header `{ path, guid, name, type,
  full_type, labels }` plus a type-specific `details` block:
  - **Texture2D** — width, height, format, filter/wrap mode, mip count,
    is_readable.
  - **Material** — shader name, render queue, shader keywords, plus a
    full property list (each value typed via ShaderUtil:
    Color/Float/Vector/Texture/Int).
  - **AudioClip** — length seconds, frequency, channels, samples,
    load type, preload flag.
  - **GameObject (Prefab root)** — tag, layer, root_components,
    child_count.
  - **ScriptableObject** — full serialized dump via
    `SerializedInspect.ObjectToJson`.
  - **TextAsset** — length, 500-char preview, truncated flag.
  - Other types return `details: null` with the common header so
    agents can still key off the type.
- `asset dependencies <path> [--recursive]` — Direct deps by default
  (matches Unity's Inspector behavior). `--recursive` walks the full
  transitive tree.
- `asset references <path>` — Reverse dependency scan. Unity has no
  index for this, so the implementation walks every asset in the
  project and checks whether `<path>` appears in its dependencies.
  Response includes `scan_ms` and `scanned_assets` so agents see the
  cost (~1.8s on a 12k-asset Unity 6 URP project in our verification).
  `--limit N --offset M` paginate; default limit 100, max 1000.
- `asset guid <path>` — `AssetDatabase.AssetPathToGUID` lookup.
- `asset path <guid>` — `AssetDatabase.GUIDToAssetPath` lookup.

**`SerializedInspect.ObjectToJson(UnityEngine.Object)`** — Public API
addition so non-Component assets (ScriptableObject, Material, anything
backed by a SerializedObject) can round-trip through the same
`{ type, properties }` shape that `ComponentToObject` returns. Internal
`WalkProperties` helper now shared between both paths; the
Component-specific entry point still special-cases Transform and
`Behaviour.enabled`.

**Error code `UCI-043 ComponentNotFound`.** Single code for three
structurally similar cases on `component get` / `component schema`:
- Type not attached to the GameObject.
- `--index N` exceeds the number of attached components of that type.
- `schema` for a type with no live instance in any loaded scene (or a
  type that does not exist in any loaded assembly).

Each variant's error message names the actual remediation data —
attached types, real instance count, or whether the type itself was
not found — so the agent can self-correct without scraping.

### Changed

- **`UCI-040 AssetNotFound` is now actively emitted** by the new
  `asset` tools. It was reserved in v0.3.0 for this exact use. Single
  code covers `inspect`/`dependencies`/`references`/`guid` with a
  bad path and `path` with an unknown GUID; messages all instruct
  the agent to verify via `asset find`.
- **Connector bumped to `0.3.0`** (`udit-connector/package.json`).
  Two new C# tools (`ManageComponent`, `ManageAsset`) plus
  `SerializedInspect.ObjectToJson` are a substantive C# delta over
  v0.2.0.
- **Shell completion** (bash/zsh/powershell/fish) learns `component`
  with three subcommands and `asset` with six.
- **Help text** in `udit --help` gains **Components** and **Assets**
  sections; `udit component --help` and `udit asset --help` cover
  every subcommand, flag, type-name resolution rule, and failure mode.
- **README.md / README.ko.md** gain Components and Assets sections,
  kept in lockstep per the bilingual policy.
- **docs/ERROR_CODES.md / .ko.md** — UCI-043 added with example
  payloads, UCI-040 entry rewritten now that it is emitted in
  practice, agent decision flow diagram updated.

### Design notes

- **`component get` field paths traverse a JObject** instead of using
  a separate path resolver on the C# side. The full SerializedInspect
  result is converted to JObject and walked by the dotted segments.
  This keeps the field-name vocabulary identical to `go inspect`,
  handles nested structs and array indices uniformly, and means
  Transform's virtual fields (`position`, `local_position`) work
  without a special case in the resolver.
- **`asset references` honestly exposes its cost.** Returning
  `scan_ms` and `scanned_assets` in the response — instead of a flag
  buried in docs — pushes agents to set `--limit` on large projects
  rather than discovering the cost through timeouts.
- **`asset inspect` uses per-type detail handlers.** A single
  SerializedObject walk would bury Material's ShaderUtil metadata,
  Texture2D's format/mip info, and AudioClip's sample rate. Six
  handlers (Texture2D, Material, AudioClip, GameObject-as-prefab,
  ScriptableObject, TextAsset) keep type-specific information
  prominent without expanding the surface for callers who only need
  the common header.

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
