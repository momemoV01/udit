# udit

CLI tool to control Unity Editor from the command line. Agent-first fork of unity-cli.

## Structure

```
cmd/                  # Go CLI — thin passthrough layer
  root.go             # Entry point, flag/arg parsing, default passthrough
  editor.go           # editor command (waitForReady polling)
  test.go             # test command (PlayMode result polling)
  status.go           # status, waitForAlive, heartbeat reading
  update.go           # self-update from GitHub releases
  version_check.go    # periodic update notice (12h interval)
internal/client/      # Unity HTTP client, instance discovery
udit-connector/       # C# Unity Editor package (UPM)
  Editor/
    Core/             # Shared utilities (Response, ParamCoercion, ToolParams, StringCaseUtility)
    Tools/            # Tool implementations (auto-registered via [UditTool] attribute)
    TestRunner/       # Test runner (RunTests, TestRunnerState)
```

## Development

### Adding a Command

1. Add a C# tool in `udit-connector/Editor/Tools/` with `[UditTool(Name = "command_name")]`
2. CLI command name matches the tool name — default passthrough handles dispatch
3. Positional args arrive as `args` array, flags as named params
4. Go-side code is only needed for polling/waiting logic (editor, test)

## Verification

Run all of the following before pushing:

```bash
go clean -testcache
gofmt -w .
~/go/bin/golangci-lint run ./...
~/go/bin/golangci-lint fmt --diff
go test ./...
```

### Integration Tests (requires Unity)

Integration tests are tagged with `//go:build integration` and excluded from the default test run.
Run them manually when Unity Editor is open:

```bash
go test -tags integration ./...
```

CI skips these since Unity is not available.

## Checklist

### 변경 시

CLI option, command, parameter를 수정하면 관련된 모든 곳을 함께 반영한다:
- C# tool (Parameters class, HandleCommand)
- Go help text (root.go의 overview + command별 detailed help)
- README.md

### 버전 관리

CLI(Go)와 Connector(C#)는 독립 버전. 변경된 쪽만 올린다.

- **Connector** (udit-connector/package.json): C# 코드 변경 시 버전 갱신
- **CLI** (git tag vX.Y.Z): Go 코드 변경 시 태그 생성 + push → CI가 Release 빌드

둘 다 바뀌면 둘 다 올린다. 한쪽만 바뀌면 한쪽만.

### 작업 마무리 시

- Verification 항목 전부 실행
- 변경한 기능은 Unity가 열려 있으면 `udit`로 직접 실행해서 동작 확인
- 로컬 임시 파일(테스트용 스크립트, 디버깅 출력 등) 정리
- 관련 없는 변경은 별도 커밋으로 분리
- 변경 이력은 CHANGELOG.md에 반영

## Git

Commit all unstaged changes before finishing. Unrelated changes should be committed separately.

기본 브랜치는 `main` (원본 unity-cli는 `master` 기준이었으나 udit은 `main` 사용).

## 실행 규칙

`go run .`은 테스트 목적일 때만 사용. CLI 기능 실행은 반드시 설치된 바이너리 `udit`로.

기본 HTTP 포트는 **8590** (원본 unity-cli의 `8090`과 공존 가능하도록 분리).

## 릴리스 플로우

"커밋하고 올려" 지시 시 아래를 한 번에 수행:

1. Verification 전부 실행
2. 변경된 쪽 버전 갱신 (Connector package.json / CLI tag)
3. 커밋 + push
4. CLI 변경 있으면 새 tag push
5. CI(CI + Release) 완료 대기 (`gh run watch --exit-status`, background)
6. `go clean -cache -testcache`로 빌드/테스트 캐시 전부 정리
7. 둘 다 성공하면 `udit update`로 설치된 CLI 업데이트

## CI

- `push/PR → main`: build, vet, test, lint, format
- `tag push (v*)`: cross-compile (linux/darwin/windows × amd64/arm64) + GitHub Release

## Upstream Relationship

udit is a fork of [unity-cli](https://github.com/youngwoocho02/unity-cli) by DevBookOfArray.
See NOTICE.md for attribution and MIT requirements.

업스트림 유지보수:
- 원본에 중요 버그픽스가 나오면 cherry-pick 검토
- 원본과 충돌 없는 범용 개선은 upstream에 PR 제안

```bash
# 업스트림 체크 (selective — 자동 병합 아님)
git remote add upstream https://github.com/youngwoocho02/unity-cli 2>/dev/null || true
git fetch upstream
git log upstream/master --oneline --since="2 weeks ago"
```
