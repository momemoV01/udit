# Security Policy

## Reporting a vulnerability

Please **do not** open a public GitHub issue for security vulnerabilities. Use a private channel instead:

- **Preferred**: open a [Private Security Advisory](https://github.com/momemoV01/udit/security/advisories/new). GitHub keeps it private until disclosure.
- Include: a description, reproduction steps, the impact you see, and (if you have one) a suggested fix.

You should hear back within **7 days** with at least an acknowledgment. Coordinated disclosure timing is worked out on the advisory.

## Supported versions

The latest minor release line is supported with security fixes. Older releases get fixes only if the same fix can be cleanly back-applied.

| Version | Supported          |
|---------|--------------------|
| 0.9.x   | ✅                 |
| < 0.9   | ❌ (please upgrade) |

`udit update` pulls the latest CLI binary; the Unity Connector follows whatever git tag your `Packages/manifest.json` points at.

## Trust model

udit is built around **"trusted local user with the Editor open"**. Specifically:

- The Connector binds to `127.0.0.1` only and rejects requests carrying an `Origin` header — browser-based attacks can't reach it.
- `udit exec`, `udit menu`, and `udit run` execute with **the Editor's full process privileges** (filesystem, network, anything Unity can reach). This is intentional. Don't pipe untrusted input into them, and treat a `.udit.yaml` from an unfamiliar source the way you'd treat an unfamiliar `Makefile`.
- Release binaries are downloaded from `github.com/momemoV01/udit/releases` over HTTPS. Trust is GitHub-level. Binaries are not codesigned (yet); on macOS you'll allow them manually.

What we don't currently defend against:
- Other users on the **same machine** hitting `127.0.0.1:8590` (loopback is not a per-user boundary on shared workstations).
- Supply-chain compromise of Unity packages or Go modules.
- A teammate committing a malicious `.udit.yaml`. That's a code-review concern — diff yaml files like you diff Makefiles.

The full long-form version of this is in the [README's Security & Trust Model section](../README.md#security--trust-model).

## Dependency hygiene

- Dependabot watches `gomod` and `github-actions` weekly (see [`.github/dependabot.yml`](./dependabot.yml)).
- `actions/upload-artifact` and `actions/download-artifact` major bumps are blocked from auto-update because they must move as a pair (see the comment in `dependabot.yml`).
- Stdlib vulnerabilities reported by `govulncheck` against the toolchain land via a Go-version bump in CI/release workflows.
