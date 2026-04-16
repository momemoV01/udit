# udit

[English](README.md) | [한국어](README.ko.md)

> Unity editor, from the command line. Built for AI agents, works with anything.
>
> Udit (उदित) — Sanskrit for *risen*. A fork of [unity-cli](https://github.com/youngwoocho02/unity-cli) by [DevBookOfArray](https://github.com/youngwoocho02), extended for agent-first game development workflows.

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/momemoV01/udit?sort=semver)](https://github.com/momemoV01/udit/releases/latest)
[![Go](https://img.shields.io/github/go-mod/go-version/momemoV01/udit)](go.mod)
[![CI](https://github.com/momemoV01/udit/actions/workflows/ci.yml/badge.svg)](https://github.com/momemoV01/udit/actions/workflows/ci.yml)

**No server to run. No config to write. No process to manage. Just type a command.**

## Install

```bash
# Linux / macOS
curl -fsSL https://raw.githubusercontent.com/momemoV01/udit/main/install.sh | sh

# Windows (PowerShell)
irm https://raw.githubusercontent.com/momemoV01/udit/main/install.ps1 | iex

# Go install (any platform)
go install github.com/momemoV01/udit@latest
```

Update: `udit update` &nbsp;|&nbsp; Check only: `udit update --check`

## Unity Setup

**Package Manager → Add package from git URL:**

```
https://github.com/momemoV01/udit.git?path=udit-connector
```

The Connector starts automatically. No configuration needed.

> **Tip:** Set **Edit → Preferences → General → Interaction Mode** to **No Throttling** so commands execute even when Unity is in the background.

## Quick Start

```bash
udit status                              # Check Unity connection
udit editor play --wait                  # Enter play mode
udit exec "return Application.dataPath;" # Run C# code inside Unity
udit console --type error                # Read error logs
udit test run --output results.xml       # Run EditMode tests
udit go find --name "Player*"            # Find GameObjects
udit build player --config production    # Build a player
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
    │    "params": { "action": "play" }}
    │                                      │
    │                                  HttpServer receives
    │                                  CommandRouter dispatches
    │                                  ManageEditor.HandleCommand()
    │                                      │
    ├─ receives JSON response  ←───────────┘
    │  { "success": true,
    │    "message": "Entered play mode." }
    │
    └─ prints result
```

The Unity Connector opens an HTTP server on `localhost:8590`, writes a heartbeat file so the CLI knows where to connect, and routes commands to `[UditTool]` handlers on the main thread. Survives domain reloads. No external dependencies.

## Commands

| Category | Commands | Description |
|----------|----------|-------------|
| **Editor** | `editor play\|stop\|pause\|refresh` | Control play mode and asset refresh |
| **Scene** | `scene list\|active\|open\|save\|reload\|tree` | Query and switch scenes |
| **GameObject** | `go find\|inspect\|path\|create\|destroy\|move\|rename\|setactive` | Query and mutate GameObjects |
| **Component** | `component list\|get\|schema\|add\|remove\|set\|copy` | Read/write component fields |
| **Asset** | `asset find\|inspect\|dependencies\|references\|guid\|path\|create\|move\|delete\|label` | Query and mutate project assets |
| **Prefab** | `prefab instantiate\|unpack\|apply\|find-instances` | Prefab instance operations |
| **Transaction** | `tx begin\|commit\|rollback\|status` | Group mutations into single Undo |
| **Project** | `project info\|validate\|preflight` | Project health and metadata |
| **Package** | `package list\|add\|remove\|info\|search\|resolve` | Unity Package Manager |
| **Build** | `build player\|targets\|addressables\|cancel` | Build standalone players |
| **Console** | `console` | Read/clear console logs |
| **Exec** | `exec "<C# code>"` | Run arbitrary C# in Unity |
| **Test** | `test run\|list` | Run EditMode/PlayMode tests |
| **Profiler** | `profiler hierarchy\|enable\|disable\|status\|clear` | Performance profiling |
| **Automation** | `log tail`, `watch`, `run <task>` | Streaming, file watching, task runner |
| **Config** | `init`, `config show\|validate\|path\|edit` | Project configuration |
| **Utility** | `status`, `update`, `doctor`, `list`, `completion` | System and diagnostics |

**Full reference with examples and flags:** [`docs/COMMANDS.md`](./docs/COMMANDS.md)

Use `--help` on any command: `udit editor --help`, `udit asset --help`, etc.

### Global Options

| Flag | Description | Default |
|------|-------------|---------|
| `--port <N>` | Override Unity instance port | auto |
| `--project <path>` | Select Unity instance by project path | latest |
| `--timeout <ms>` | HTTP request timeout | 120000 |
| `--json` | Emit machine-readable JSON envelope | off |

## Writing Custom Tools

Create a static class with `[UditTool]` in any Editor assembly — the Connector discovers it automatically:

```csharp
[UditTool(Name = "spawn", Description = "Spawn an enemy")]
public static class SpawnEnemy
{
    public static object HandleCommand(JObject parameters)
    {
        var p = new ToolParams(parameters);
        float x = p.GetFloat("x", 0);
        var prefab = Resources.Load<GameObject>(p.Get("prefab", "Enemy"));
        var go = Object.Instantiate(prefab, new Vector3(x, 0, 0), Quaternion.identity);
        return new SuccessResponse("Spawned", new { name = go.name });
    }
}
```

```bash
udit spawn --x 5 --prefab Goblin
```

**Full guide with attribute reference:** [`docs/CUSTOM_TOOLS.md`](./docs/CUSTOM_TOOLS.md)

## Performance

Measured on a 10k GameObject scene (~10,762 assets). All queries return in under 1 second.

| Query | ms (avg) | Notes |
|---|---:|---|
| `scene tree` | ~550 | Full hierarchy |
| `go find --name` | ~760 | 10k matches |
| `go inspect` | ~450 | Single GO dump |
| `asset references` | ~960 | Full project scan |
| `asset dependencies` | ~440 | Direct deps |

Details in [ROADMAP Decision Log](./docs/ROADMAP.md#decision-log).

## Security & Trust Model

udit assumes a **trusted local user with the Editor open**.

- **Transport:** Localhost-only (`127.0.0.1`), rejects browser `Origin` headers
- **Code execution is a feature:** `exec`, `menu`, `run` have full Editor privileges — don't pipe untrusted input
- **Updates:** HTTPS from GitHub Releases with SHA256 checksum verification
- **Not protected:** Shared machines, supply-chain compromise, malicious `.udit.yaml` (treat like a Makefile)

Report vulnerabilities via [GitHub Security Advisory](https://github.com/momemoV01/udit/security/advisories/new).

## Compared to MCP

| | MCP | udit |
|---|-----|-----------|
| **Install** | Python + uv + FastMCP + config JSON | Single binary |
| **Dependencies** | Python runtime, WebSocket relay | None |
| **Protocol** | JSON-RPC 2.0 over stdio + WebSocket | Direct HTTP POST |
| **Setup** | Generate MCP config, restart AI tool | Add Unity package, done |
| **Reconnection** | Complex reconnect logic for domain reloads | Stateless per request |
| **Compatibility** | MCP-compatible clients only | Anything with a shell |

## API Stability

Starting with **v1.0.0**, udit follows [Semantic Versioning](https://semver.org) strictly.

| Surface | Stable from v1.0 | Example |
|---|---|---|
| CLI command & subcommand names | Yes | `udit console`, `udit go find` |
| CLI flag names | Yes | `--json`, `--port`, `--limit` |
| JSON envelope shape | Yes | `{ success, message, data, error_code }` |
| Error codes (UCI-xxx) | Yes — codes are never reused | `UCI-001`, `UCI-042` |
| Existing response field names | Yes | `data.matches`, `data.count` |

Backward-compatible additions in minor versions. Breaking changes only in major versions (deprecated first).

## Unity Compatibility

| Unity Version | Status | Notes |
|---|---|---|
| 6000.4.x (Unity 6.1) | Tested | Benchmarked on 6000.4.2f1 |
| 6000.0.x -- 6000.3.x (Unity 6.0) | Best-effort | Same API surface; not regression-tested |
| 2022.3 LTS | Not tested | Likely compatible; PRs with test results welcome |
| < 2022 | Unsupported | Internal API differences |

## Documentation

| Document | Description |
|----------|-------------|
| [`docs/COMMANDS.md`](./docs/COMMANDS.md) | Full command reference with examples |
| [`docs/CUSTOM_TOOLS.md`](./docs/CUSTOM_TOOLS.md) | Guide to writing project-specific tools |
| [`docs/COOKBOOK.md`](./docs/COOKBOOK.md) | Practical workflow recipes |
| [`docs/ERROR_CODES.md`](./docs/ERROR_CODES.md) | UCI error code registry |
| [`docs/ROADMAP.md`](./docs/ROADMAP.md) | Development roadmap and decision log |

## Acknowledgments

udit is a fork of **[unity-cli](https://github.com/youngwoocho02/unity-cli)** by **DevBookOfArray** (youngwoocho02). The original architecture, HTTP bridge, reflection-based tool discovery, heartbeat design, and domain-reload handling form the complete foundation of this project. See [NOTICE.md](./NOTICE.md).

[![Original](https://img.shields.io/badge/Original-unity--cli-success?logo=github)](https://github.com/youngwoocho02/unity-cli)
[![YouTube](https://img.shields.io/badge/YouTube-DevBookOfArray-red?logo=youtube&logoColor=white)](https://www.youtube.com/@DevBookOfArray)

## Maintainer

Maintained by **momemo** ([![GitHub](https://img.shields.io/badge/GitHub-momemoV01-181717?logo=github)](https://github.com/momemoV01))

## License

MIT — see [LICENSE](./LICENSE). Copyright notices for both the original and this fork must be preserved in all copies.
