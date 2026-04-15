<!-- Thanks for the PR! Keeping this template short — fill in what's relevant, drop what isn't. -->

## What

<!-- One or two sentences. The change, not the implementation. -->

## Why

<!-- The motivation. If it fixes an issue, "Fixes #NNN" is enough. -->

## How verified

<!-- The commands you ran. Tick the ones that apply.
     Connector changes need the udit/Unity loop too. -->

- [ ] `go test ./...`
- [ ] `~/go/bin/golangci-lint run ./...`
- [ ] `gofmt -w .` and `golangci-lint fmt --diff` clean
- [ ] (Connector) `udit editor refresh --compile` + `udit console --type error` clean
- [ ] (Connector) `udit test` passes

## Docs

<!-- Required when user-facing behavior, flags, or commands change. -->

- [ ] README updated (en + ko mirrored — see CONTRIBUTING.md docs policy)
- [ ] CHANGELOG.md entry under `[Unreleased]`
- [ ] N/A — internal-only change

## Anything reviewers should know

<!-- Tradeoffs you made, alternatives you considered, follow-ups you're explicitly leaving for later. -->
