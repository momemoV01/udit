# Error Codes (UCI-xxx)

[English](ERROR_CODES.md) | [한국어](ERROR_CODES.ko.md)

Stable identifiers in `--json` responses. Agents should branch on these instead of parsing English message text. Codes are mapped from both the Go CLI side (UCI-001..003 — connectivity) and the Unity Connector side (UCI-010+ — request/runtime).

## Quick Reference

| Code | Name | Origin | Retry? | Typical cause |
|---|---|---|---|---|
| `UCI-001` | NoUnityRunning | CLI | ❌ User must launch Unity | No instance file, dead PID, wrong port |
| `UCI-002` | ConnectionRefused | CLI | ⏳ After 1-3s | Connector HTTP server not yet up |
| `UCI-003` | CommandTimeout | CLI | ⏳ After delay | `--timeout` exceeded; Unity busy or hung |
| `UCI-004` | StreamInterrupted | CLI | ⏳ Reconnect with backoff | Live SSE connection dropped (EOF, reload, net.OpError) |
| `UCI-006` | InvalidStreamFilter | CLI/Connector | ❌ Fix flag value | `log tail` query param has unknown `type`/`stacktrace`/`since_ms` value |
| `UCI-007` | ConnectorTooOld | CLI | ❌ Upgrade connector | Response Content-Type was not `text/event-stream`; connector < 0.8.0 |
| `UCI-010` | UnknownCommand | Connector | ❌ Fix command name | Typo or missing `[UditTool]` registration |
| `UCI-011` | InvalidParams | Connector | ❌ Fix params | Required param missing, out of bounds, wrong shape |
| `UCI-020` | UnityCompiling | Connector | ⏳ After 2-3s | Script recompilation in progress |
| `UCI-021` | UnityUpdating | Connector | ⏳ After 2-3s | Asset import in progress |
| `UCI-030` | ExecCompileError | Connector | ❌ Fix C# code | `udit exec` syntax/semantic error |
| `UCI-031` | ExecRuntimeError | Connector | ❌ Fix C# logic | `udit exec` threw at runtime |
| `UCI-040` | AssetNotFound | Connector | ❌ Fix path/GUID | `asset inspect`/`dependencies`/`references`/`guid`/`path` with a path or GUID that the AssetDatabase cannot resolve |
| `UCI-041` | SceneNotFound | Connector | ❌ Fix path | `scene open` with non-existent path |
| `UCI-042` | GameObjectNotFound | Connector | ❌ Re-scan, then fix ID | `go inspect` / `go path` with stale or unknown stable ID |
| `UCI-043` | ComponentNotFound | Connector | ❌ Fix type name | `component get` / `component schema` where the GameObject has no component of that type, or no such type exists in loaded assemblies |
| `UCI-999` | Unknown | Either | 🟡 Inspect message | Unclassified — log & report upstream |

## Detail

### `UCI-001` — NoUnityRunning

**Origin**: Go CLI (`cmd/output.go > classifyGoError`)
**Triggers when**:
- `~/.udit/instances/` empty or missing
- All instance files have dead PIDs (process gone)
- `--port N` requested but no instance on that port
- `--project SUBSTR` requested but no projectPath matches

**Agent action**: Stop. Ask the user to launch Unity (with the udit-connector package). Do not retry — the situation won't change automatically.

**Example**:
```json
{
  "success": false,
  "command": "status",
  "error_code": "UCI-001",
  "message": "no status for port 9999 — Unity may not be running"
}
```

### `UCI-002` — ConnectionRefused

**Origin**: Go CLI
**Triggers when**: Instance file exists and PID is alive, but TCP connect to `127.0.0.1:<port>` fails. Usually means the HttpServer just restarted (domain reload) and isn't listening yet.

**Agent action**: Wait 1-3 seconds and retry once. If still failing, fall back to UCI-001 reasoning (something more wrong).

### `UCI-003` — CommandTimeout

**Origin**: Go CLI
**Triggers when**: `httpClient.Timeout` exceeds (default 120000ms; overridable via `--timeout`).

**Agent action**: For commands that take longer (e.g. `editor refresh --compile` on huge projects, `test --mode PlayMode`), retry with a higher `--timeout`. For quick commands, treat as a sign Unity is hung — `udit status` to check.

### `UCI-004` — StreamInterrupted

**Origin**: Go CLI (`cmd/output.go > classifyGoError`; emitted by `udit log tail`)
**Triggers when**: A live Server-Sent Events connection to `/logs/stream` drops — EOF on the body reader, a `net.OpError`, or receipt of the synthetic `event: reload` marker the Connector emits before a domain reload.

**Agent action**: Retryable. `udit log tail` handles this internally with exponential backoff (1s → 2s → 4s cap). Agents that run `log tail --json` and parse NDJSON will see `{"kind":"reconnect","in_ms":…,"reason":…}` between streams of `{"kind":"log",…}` events.

### `UCI-006` — InvalidStreamFilter

**Origin**: Connector (`LogStream.TryParseFilter`) returning HTTP 400; also Go CLI for malformed `--since`.
**Triggers when**: `udit log tail` is invoked with an unknown `--type` value (accepted: `error,warning,log,assert,exception`), `--stacktrace` mode (accepted: `none,user,full`), or malformed `--since` duration.

**Agent action**: Fix the flag value. Do not retry verbatim.

### `UCI-007` — ConnectorTooOld

**Origin**: Go CLI (`internal/client/stream.go > StreamLogs`).
**Triggers when**: `GET /logs/stream` returned HTTP 200 but the `Content-Type` header is not `text/event-stream`. The Connector predates the SSE endpoint (< 0.8.0) and is interpreting the GET as an unknown-path POST.

**Agent action**: Upgrade the Connector to ≥ 0.8.0 in the Unity project. Non-retryable.

### `UCI-010` — UnknownCommand

**Origin**: Connector (`CommandRouter`)
**Triggers when**: No `[UditTool]` handler matches the command name.

**Agent action**: Run `udit list` to see registered tools. Check spelling. If a custom tool was added, ensure the Editor assembly compiled (no Console errors).

### `UCI-011` — InvalidParams

**Origin**: Connector (multiple tools)
**Triggers when**:
- Required parameter missing (e.g. `exec` without `code`)
- Out-of-bounds value (e.g. `screenshot --width -1` or `--width 99999`)
- Wrong enum value (e.g. `screenshot --view invalid`)
- Malformed request body (HTTP layer)

**Agent action**: Read the message — it always names the offending parameter. Fix and retry. Do not retry verbatim.

### `UCI-020` / `UCI-021` — Unity Busy

**Origin**: Connector (`CommandRouter` guard)
**Triggers when**: `EditorApplication.isCompiling` (020) or `isUpdating` (021) is true. The router refuses most commands during these states because Unity APIs throw or hang mid-reload.

**Agent action**: Wait 2-3 seconds and retry. The `list` command remains exempt and always works. `udit status` reports the current state.

### `UCI-030` — ExecCompileError

**Origin**: Connector (`ExecuteCsharp`)
**Triggers when**: csc returns non-zero (compile error in supplied C# code) or hangs past the 30s timeout.

**Agent action**: Read the error — it includes line numbers from the user's snippet. Fix the C# and retry. Do not retry the same code.

### `UCI-031` — ExecRuntimeError

**Origin**: Connector (`ExecuteCsharp`)
**Triggers when**: Compiled snippet throws at runtime (NullReferenceException, etc).

**Agent action**: Same as 030 — read the message, fix the C#, retry. Often paired with a `Debug.LogException` in Unity Console (visible via `udit console --type error`).

### `UCI-040` / `UCI-041` — Asset/Scene Not Found

**Origin**: Connector (`ManageAsset` emits 040, `ManageScene` emits 041)
**Triggers when**: A path or GUID cannot be resolved by the AssetDatabase.
- `UCI-040` — `asset inspect`, `asset dependencies`, `asset references`, `asset guid` with an unknown path; `asset path` with an unknown GUID.
- `UCI-041` — `scene open <path>` with a path that does not exist.

**Agent action**: Verify the identifier. Run `udit asset find` (for UCI-040) or `udit scene list` (for UCI-041) to discover valid paths/GUIDs. Paths are project-relative and start with `Assets/` or `Packages/`. GUIDs are 32 hex chars (no dashes).

### `UCI-042` — GameObjectNotFound

**Origin**: Connector (`ManageGameObject`)
**Triggers when**: `udit go inspect go:XXXX` or `udit go path go:XXXX` is called with a stable ID that the current session's `StableIdRegistry` does not know — either because the ID is from a previous session (the registry is in-memory and resets on domain reload), or because the GameObject was destroyed.

**Agent action**: Run `udit go find` or `udit scene tree` first to re-seed the registry. If the ID still does not resolve after a scan, the GameObject is gone — the agent should fall back to a fresh `go find` for the same entity. Do not retry the same ID blindly.

**Example**:
```json
{
  "success": false,
  "command": "go",
  "error_code": "UCI-042",
  "message": "GameObject not found: go:deadbeef. Run `go find` first if the ID is from a previous session."
}
```

### `UCI-043` — ComponentNotFound

**Origin**: Connector (`ManageComponent`)
**Triggers when**: Three distinct cases, all of which map to the same code because the remediation is the same (check the type name):
- `component get go:XXXX MyType` — the GameObject has no component of type `MyType`.
- `component schema MyType` — no type named `MyType` exists in any loaded assembly, or the type is not a `Component` subclass, or no live instance of `MyType` exists in the loaded scenes (schema v1 requires a probe instance).
- `component get go:XXXX MyType --index 3` — fewer than 4 components of that type on the GameObject.

**Agent action**: Run `udit component list go:XXXX` to see which types are actually on the GameObject (for the first two cases) or `udit go find --component MyType` to see if any scene has an instance (for schema). Fix the type name / index / scene setup and retry. Type names are matched case-insensitively and unqualified names resolve against `UnityEngine.*` first, so `Transform` and `UnityEngine.Transform` behave identically.

**Example**:
```json
{
  "success": false,
  "command": "component",
  "error_code": "UCI-043",
  "message": "Component type 'Rigidbody' not found on go:9598abb1. Attached: Transform, Camera, AudioListener."
}
```

### `UCI-999` — Unknown

**Origin**: Either side
**Triggers when**: An error path that hasn't been classified yet. Always paired with a human-readable message.

**Agent action**: Surface the message to the user. If reproducible, file an issue — `UCI-999` occurrences are tech debt to be promoted to a real code.

## Agent Decision Flow

```
                         ┌───────────────┐
                         │ Got error_code │
                         └───────┬───────┘
                                 │
            ┌────────────────────┼────────────────────┐
            ▼                    ▼                    ▼
       UCI-001/010/030      UCI-002/020/021         UCI-003
       UCI-011/031/040      ────────────────       ───────
       UCI-041/042/043                                
       ───────────────                                
       Stop. Report to       Sleep 1-3s,           Retry once
       user. Don't loop.     retry once.           with longer
       UCI-042: re-scan     Loop max 3x.          --timeout.
       via go find first.
       UCI-043: component
       list first.
```

## Adding New Codes

1. Add the constant to `udit-connector/Editor/Core/Response.cs > ErrorCodes`
2. Use it at the call site: `new ErrorResponse(ErrorCodes.MyNewCode, "...")`
3. Document here (description, origin, retry, agent action, example)
4. If the CLI-side detection is needed, extend `cmd/output.go > classifyGoError`
5. Bump `CHANGELOG.md` `[Unreleased] > Added` with the new code

Codes are stable identifiers. **Never repurpose an existing code.** If a category needs to split, allocate a new number in the same 0xx band.
