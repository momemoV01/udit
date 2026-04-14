# udit

CLI tool to control Unity Editor from the command line. Agent-first fork of unity-cli.

## Planning

전체 로드맵은 [`docs/ROADMAP.md`](./docs/ROADMAP.md) 참고. 새 기능 작업 시 해당 문서의 Phase 스코프/성공 기준과 맞추고, 설계 결정은 **Decision Log** 섹션에 추가한다.

## Claude Code Skills

이 저장소는 `.claude/skills/` 에 프로젝트 전용 Skill을 포함한다. 반복적인 Unity 작업은 해당 스킬을 우선 호출한다.

- `/unity-verify` — 스크립트 수정 후 compile → console → test 표준 검증 절차
- `/unity-scene-edit` — YAML 에셋 텍스트 편집 후 reserialize 안전 플로우
- `/unity-perf-check` — 프로파일러 활성화 → 시나리오 측정 → 상위 병목 리포트
- `/udit-release` — udit 자체 버전 릴리스 절차 (verification → tag → CI → 바이너리)

권한 정책은 `.claude/settings.json` 참고. Unity 에디터 플레이/정지, exec, reserialize, git push 등 영향 큰 명령은 `ask` 처리.

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

## Windows Store Claude Desktop 샌드박스 주의사항

Windows용 Claude Desktop이 **Microsoft Store(MSIX) 패키지**로 설치된 환경에서는 Claude Code 내부에서 실행한 파일 쓰기가 **앱 컨테이너로 리다이렉트**된다. 이는 조용히 일어나고, 실수하기 쉬우므로 반드시 주의:

### 증상
- Claude Code의 Bash에서 `%LOCALAPPDATA%\udit\udit.exe` 를 빌드했는데
- 외부 cmd/PowerShell/탐색기에서는 **해당 파일이 보이지 않음**
- `Get-Item`의 `Target` 속성에 `C:\Users\<user>\AppData\Local\Packages\Claude_pzs8sxrjxfjjc\LocalCache\Local\...` 표시

### 원인
MSIX 앱(Claude Desktop)은 `%LOCALAPPDATA%`, `%APPDATA%`, `%ProgramFiles%` 등 시스템 폴더 쓰기를 **자동으로 Package 샌드박스로 매핑**한다. 레지스트리 쓰기도 마찬가지. 이는 Windows의 App Container 보안 모델.

### 규칙

1. **udit 바이너리 설치/업데이트는 반드시 Claude 밖에서**:
   ```powershell
   # 시작 메뉴 → PowerShell (Claude 안의 터미널 X)
   cd E:\Workspace\udit
   go build -ldflags="-s -w -X main.Version=vX.Y.Z-local" -o "$env:LOCALAPPDATA\udit\udit.exe" .
   ```
   → 실제 `C:\Users\<user>\AppData\Local\udit\` 에 기록되어 외부 터미널에서도 사용 가능.

2. **Claude 내부 작업은 프로젝트 경로에서만**:
   - `E:\Workspace\udit\` 같이 사용자 명시 경로는 가상화 X
   - `go test`, 소스 편집, commit, push는 내부 Bash에서 OK

3. **PATH 등록**은 Claude 내/외 무관하게 레지스트리에 정상 저장 (확인됨).

4. **의심스러우면 `Target` 확인**:
   ```powershell
   (Get-Item "$env:LOCALAPPDATA\udit\udit.exe").Target
   ```
   값이 `Packages\Claude_*` 를 포함하면 가상화된 파일.

### 우회 불가 작업
외부 접근이 필요한 파일 생성/수정(바이너리 설치, 데스크톱 단축키 등)은 **반드시 사용자에게 외부 터미널 실행을 안내**할 것.

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
