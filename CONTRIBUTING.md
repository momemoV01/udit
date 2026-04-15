# Contributing to udit

Thanks for thinking about contributing. udit is small enough that anything from a typo fix to a new `[UditTool]` is welcome.

## Project shape

Two halves you'll want to understand:

- **`cmd/` + `internal/` (Go)** — the CLI binary. Talks to Unity over HTTP, no state of its own. Most user-facing behavior lives here.
- **`udit-connector/` (C# Unity package)** — the Editor-side HTTP server + tools. Loaded as a UPM package via git URL.

Pick the side you're comfortable with. PRs that touch both are fine; the verification checklist below covers each.

## Setting up

```bash
# Go side
go version          # 1.26+
go test ./...
~/go/bin/golangci-lint run ./...

# C# side — open the udit-connector folder as a Unity package via
# Window > Package Manager > Add package from disk
# Then point your test project at it. The connector is Editor-only.
```

For full local dev, install [Unity 6000.4+](https://unity.com/releases) and add the connector to a scratch project. The README's **Unity Setup** section walks through it from scratch.

## Submitting changes

1. Fork → branch from `main` (`feat/...`, `fix/...`, `docs/...`).
2. Make the change. Keep commits focused — small commits review fast.
3. Run the verification suite (next section).
4. Open a PR. The PR template has a short checklist.

The maintainer reviews within a few days. CI must be green to merge.

## Verification (run before pushing)

```bash
go clean -testcache
gofmt -w .
go vet ./...
go test ./...
~/go/bin/golangci-lint run ./...
~/go/bin/golangci-lint fmt --diff
```

Connector changes also need:

```bash
udit editor refresh --compile          # recompile the Editor scripts
udit console --type error --lines 20   # surface compile errors
udit test                              # run NUnit fixtures
```

The integration tests are tagged `//go:build integration` and skipped by default. Run them when Unity is open and you want to exercise the real HTTP path:

```bash
go test -tags integration ./...
```

## Code style

- **Go**: gofmt + golangci-lint settle most of it. Idiomatic Go, no surprises. Errors wrap with `%w` when the cause matters; otherwise `fmt.Errorf("…")` is fine.
- **C#**: 4-space indent, K&R-ish braces. Match the surrounding file's style — most of `udit-connector` is one consistent house style. Don't introduce new abstractions just because.
- **Tests**: prefer table-driven for parsers and dispatch tables. The `Vector3ParsingTests.cs` and `cmd/log_test.go` patterns are good references.

## Documentation policy (English + Korean)

User-facing docs stay paired:

| English (canonical) | Korean (translation) |
|---|---|
| `README.md` | `README.ko.md` |
| `docs/ROADMAP.md` | `docs/ROADMAP.ko.md` |
| `docs/ERROR_CODES.md` | `docs/ERROR_CODES.ko.md` |

If you change the English file, mirror the change in the Korean one in the same PR. Code blocks, command names, GUIDs stay verbatim — translate the prose around them.

`CHANGELOG.md`, `LICENSE`, `NOTICE.md`, and `CLAUDE.md` are English-only.

## Adding a new tool

A custom tool is a `[UditTool]` static class in `udit-connector/Editor/Tools/`. The CLI auto-discovers it at startup; no Go change is needed for read-style tools. The `udit help custom-tools` topic walks through the minimum viable example.

If your tool needs polling or long-running behavior on the CLI side (e.g. `udit test` waits for PlayMode to finish), add a thin Go file in `cmd/` for that loop. The `cmd/test.go` and `cmd/editor.go` files are the prior art.

## Versioning

CLI and Connector version independently:

- **CLI** (Go binary, `git tag vX.Y.Z`): bump only when the binary itself changed.
- **Connector** (`udit-connector/package.json`'s `version`): bump only when the C# changed.

If both changed, both bump. Both follow [SemVer](https://semver.org/spec/v2.0.0.html). Pre-`v1.0.0` we treat minor bumps as our "feature added" granularity and patch bumps as "fix or doc-only".

## Reporting bugs / requesting features

- Bug → GitHub Issues (template provided). Include `udit version`, Unity version, OS, and the smallest reproduction you can manage.
- Feature → start with a Discussion or a sketchy issue. Big features get scoped in [`docs/ROADMAP.md`](./docs/ROADMAP.md) before implementation.

## Security

Don't open a public issue for security vulnerabilities — see [`SECURITY.md`](.github/SECURITY.md).

## License

By contributing, you agree your contribution is licensed under MIT (see [LICENSE](./LICENSE)). Original-fork attribution stays intact regardless — see [NOTICE.md](./NOTICE.md).
