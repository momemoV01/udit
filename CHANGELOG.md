# Changelog

All notable changes to **udit** are documented here. This project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html) and [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

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
