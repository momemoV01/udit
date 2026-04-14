# NOTICE

`udit` is a fork of [unity-cli](https://github.com/youngwoocho02/unity-cli) by **DevBookOfArray** (youngwoocho02), redistributed and extended under the terms of the MIT License.

## Original Project

- **Name**: unity-cli
- **Author**: DevBookOfArray
- **Repository**: https://github.com/youngwoocho02/unity-cli
- **License**: MIT
- **YouTube**: https://www.youtube.com/@DevBookOfArray

The architecture, protocol design, and initial implementation of the CLI and Unity Connector all originate from that project. Every substantive idea this fork inherits — the stateless HTTP bridge to Unity, the `[UnityCliTool]` reflection-based discovery pattern, the heartbeat-driven instance registry, the domain-reload-resilient connector — was first built there.

## Fork Information

- **Fork name**: udit
- **Maintained by**: momemo (`momemoV01`)
- **Forked from**: unity-cli (latest `master` as of April 2026)
- **Fork date**: 2026-04-14
- **Original author permission**: Granted verbally by DevBookOfArray prior to fork.

## What Changed from the Original

This fork carries a distinct identity to enable an agent-first development roadmap without diverging from upstream. The changes at the point of initial fork are purely **rebranding** — no functional changes yet:

- Go module path: `github.com/youngwoocho02/unity-cli` → `github.com/momemoV01/udit`
- Binary name: `unity-cli` → `udit`
- Unity package name: `com.youngwoocho02.unity-cli-connector` → `com.momemov01.udit-connector`
- Unity package folder: `unity-connector/` → `udit-connector/`
- C# namespace: `UnityCliConnector` → `UditConnector`
- C# attribute: `[UnityCliTool]` → `[UditTool]`
- Instance/heartbeat directory: `~/.unity-cli/` → `~/.udit/`
- Default HTTP port: `8090` → `8590` (to coexist with unity-cli)
- Version reset: `v0.3.x` → `v0.1.0`

Subsequent functional changes are tracked in [CHANGELOG.md](./CHANGELOG.md).

## Acknowledgments

Thank you to **DevBookOfArray** for creating the original tool, for making the design elegant enough that a fork could be this small, and for explicitly permitting this fork. If you find `udit` useful, please also consider starring [unity-cli](https://github.com/youngwoocho02/unity-cli) and subscribing to [@DevBookOfArray](https://www.youtube.com/@DevBookOfArray).

## Attribution Requirement (MIT)

Per the MIT License, both the original and the fork copyright notices in [`LICENSE`](./LICENSE) must be preserved in all copies or substantial portions of this software.
