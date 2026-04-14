# Changelog

All notable changes to **udit** are documented here. This project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html) and [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

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
