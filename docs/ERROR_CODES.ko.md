# 에러 코드 (UCI-xxx)

[English](ERROR_CODES.md) | [한국어](ERROR_CODES.ko.md)

`--json` 응답의 안정적인 식별자. 에이전트는 영어 메시지 텍스트 파싱 대신 이 코드로 분기해야 합니다. Go CLI 쪽 (UCI-001..003 — 연결성)과 Unity Connector 쪽 (UCI-010+ — 요청/런타임) 모두에서 매핑됩니다.

## 빠른 참조

| 코드 | 이름 | 출처 | 재시도? | 일반적 원인 |
|---|---|---|---|---|
| `UCI-001` | NoUnityRunning | CLI | ❌ 사용자가 Unity 실행 필요 | instance 파일 없음, 죽은 PID, 잘못된 포트 |
| `UCI-002` | ConnectionRefused | CLI | ⏳ 1-3초 후 | Connector HTTP 서버가 아직 안 떠있음 |
| `UCI-003` | CommandTimeout | CLI | ⏳ 시간 두고 | `--timeout` 초과; Unity busy 또는 hang |
| `UCI-010` | UnknownCommand | Connector | ❌ 명령 이름 수정 | 오타 또는 `[UditTool]` 등록 누락 |
| `UCI-011` | InvalidParams | Connector | ❌ 파라미터 수정 | 필수 누락, 범위 초과, 잘못된 형식 |
| `UCI-020` | UnityCompiling | Connector | ⏳ 2-3초 후 | 스크립트 재컴파일 진행 중 |
| `UCI-021` | UnityUpdating | Connector | ⏳ 2-3초 후 | 에셋 import 진행 중 |
| `UCI-030` | ExecCompileError | Connector | ❌ C# 코드 수정 | `udit exec` 문법/의미 에러 |
| `UCI-031` | ExecRuntimeError | Connector | ❌ C# 로직 수정 | `udit exec` 런타임에서 throw |
| `UCI-040` | AssetNotFound | Connector | ❌ 경로/GUID 수정 | Phase 2 (Observe) 예약 |
| `UCI-041` | SceneNotFound | Connector | ❌ 경로 수정 | `scene open`에 존재하지 않는 경로 |
| `UCI-042` | GameObjectNotFound | Connector | ❌ 재스캔 후 ID 수정 | `go inspect` / `go path` 에 오래되거나 알 수 없는 stable ID |
| `UCI-999` | Unknown | 양쪽 | 🟡 메시지 검사 | 미분류 — 로그 + 업스트림 보고 |

## 상세

### `UCI-001` — NoUnityRunning

**출처**: Go CLI (`cmd/output.go > classifyGoError`)
**발생 시점**:
- `~/.udit/instances/` 가 비어있거나 없음
- 모든 instance 파일의 PID가 죽음 (프로세스 종료)
- `--port N` 요청했는데 해당 포트의 instance 없음
- `--project SUBSTR` 요청했는데 매칭되는 projectPath 없음

**에이전트 행동**: 중단. 사용자에게 Unity 실행 (udit-connector 패키지 포함) 요청. **재시도 금지** — 상황이 자동으로 바뀌지 않음.

**예시**:
```json
{
  "success": false,
  "command": "status",
  "error_code": "UCI-001",
  "message": "no status for port 9999 — Unity may not be running"
}
```

### `UCI-002` — ConnectionRefused

**출처**: Go CLI
**발생 시점**: instance 파일이 있고 PID도 살아있지만, `127.0.0.1:<port>` TCP 연결 실패. 보통 HttpServer가 막 재시작했고 (도메인 리로드) 아직 listen 안 한 상태.

**에이전트 행동**: 1-3초 기다렸다 한 번 재시도. 여전히 실패면 UCI-001 추론으로 fallback (더 큰 문제).

### `UCI-003` — CommandTimeout

**출처**: Go CLI
**발생 시점**: `httpClient.Timeout` 초과 (기본 120000ms; `--timeout` 으로 오버라이드 가능).

**에이전트 행동**: 시간이 더 걸리는 명령 (예: 거대 프로젝트의 `editor refresh --compile`, `test --mode PlayMode`)은 더 큰 `--timeout` 으로 재시도. 빠른 명령은 Unity hang 신호로 처리 — `udit status` 로 확인.

### `UCI-010` — UnknownCommand

**출처**: Connector (`CommandRouter`)
**발생 시점**: 해당 이름으로 매칭되는 `[UditTool]` 핸들러 없음.

**에이전트 행동**: `udit list` 로 등록된 tool 확인. 철자 점검. 커스텀 tool 추가했으면 Editor 어셈블리 컴파일 성공 여부 확인 (Console 에러 없는지).

### `UCI-011` — InvalidParams

**출처**: Connector (여러 tool)
**발생 시점**:
- 필수 파라미터 누락 (예: `code` 없는 `exec`)
- 범위 초과 값 (예: `screenshot --width -1` 또는 `--width 99999`)
- 잘못된 enum 값 (예: `screenshot --view invalid`)
- 잘못된 요청 본문 (HTTP 레이어)

**에이전트 행동**: 메시지 읽기 — 항상 문제 파라미터 이름을 명시함. 수정 후 재시도. **그대로 재시도 금지**.

### `UCI-020` / `UCI-021` — Unity Busy

**출처**: Connector (`CommandRouter` 가드)
**발생 시점**: `EditorApplication.isCompiling` (020) 또는 `isUpdating` (021) true. 라우터는 이 상태 동안 대부분 명령을 거부 — Unity API가 mid-reload에 throw하거나 hang하기 때문.

**에이전트 행동**: 2-3초 기다렸다 재시도. `list` 명령은 예외로 항상 작동. `udit status` 로 현재 상태 보고.

### `UCI-030` — ExecCompileError

**출처**: Connector (`ExecuteCsharp`)
**발생 시점**: csc가 non-zero 반환 (제공된 C# 코드의 컴파일 에러) 또는 30초 타임아웃 초과.

**에이전트 행동**: 에러 읽기 — 사용자 스니펫의 줄 번호 포함됨. C# 수정 후 재시도. **같은 코드로 재시도 금지**.

### `UCI-031` — ExecRuntimeError

**출처**: Connector (`ExecuteCsharp`)
**발생 시점**: 컴파일된 스니펫이 런타임에 throw (NullReferenceException 등).

**에이전트 행동**: 030과 동일 — 메시지 읽고, C# 수정, 재시도. 종종 Unity Console의 `Debug.LogException` 와 짝 (`udit console --type error` 로 보임).

### `UCI-040` / `UCI-041` — Asset/Scene Not Found

**출처**: Connector (`ManageScene`이 041을 emit; 040은 `AssetTools` 용으로 예약)
**발생 시점**: `udit scene open <path>` 에 씬 에셋이 아닌 경로를 넘겼을 때. `UCI-040`은 `asset find/inspect` 가 들어오면 사용.

**에이전트 행동**: 경로 검증. `udit scene list` 가 모든 씬의 path + GUID를 돌려주므로 이걸로 올바른 식별자 탐색.

### `UCI-042` — GameObjectNotFound

**출처**: Connector (`ManageGameObject`)
**발생 시점**: `udit go inspect go:XXXX` 또는 `udit go path go:XXXX` 가 현재 세션의 `StableIdRegistry`에 없는 stable ID를 받았을 때 — 이전 세션의 ID라 (레지스트리는 인메모리, 도메인 리로드 시 초기화), 또는 해당 GameObject가 파괴되었기 때문.

**에이전트 행동**: 먼저 `udit go find` 또는 `udit scene tree`로 레지스트리를 다시 채운다. 스캔 후에도 resolve 안 되면 GO가 사라진 것 — 동일 엔티티에 대해 새로 `go find` 를 호출. 같은 ID로 무작정 재시도하지 말 것.

**예시**:
```json
{
  "success": false,
  "command": "go",
  "error_code": "UCI-042",
  "message": "GameObject not found: go:deadbeef. Run `go find` first if the ID is from a previous session."
}
```

### `UCI-999` — Unknown

**출처**: 양쪽
**발생 시점**: 아직 분류되지 않은 에러 경로. 항상 사람이 읽을 수 있는 메시지와 짝.

**에이전트 행동**: 메시지를 사용자에게 그대로 전달. 재현 가능하면 이슈 등록 — `UCI-999` 발생은 실제 코드로 승격되어야 할 기술 부채.

## 에이전트 의사결정 흐름

```
                         ┌────────────────┐
                         │ error_code 받음 │
                         └───────┬────────┘
                                 │
            ┌────────────────────┼────────────────────┐
            ▼                    ▼                    ▼
       UCI-001/010/030      UCI-002/020/021         UCI-003
       UCI-011/031/040      ────────────────       ───────
       UCI-041/042
       ───────────────
       중단. 사용자에게      1-3초 sleep,          더 큰
       보고. 루프 금지.      한 번 재시도.          --timeout
       UCI-042: go find로   루프 max 3회.          으로 재시도
       재스캔 후에만 재시도.
```

## 새 코드 추가

1. `udit-connector/Editor/Core/Response.cs > ErrorCodes` 에 상수 추가
2. 호출 위치에서 사용: `new ErrorResponse(ErrorCodes.MyNewCode, "...")`
3. 여기 문서화 (설명, 출처, 재시도, 에이전트 행동, 예시) — **영문 ERROR_CODES.md 와 함께 동기화**
4. CLI 쪽 감지가 필요하면 `cmd/output.go > classifyGoError` 확장
5. `CHANGELOG.md` `[Unreleased] > Added` 에 새 코드 항목

코드는 안정적 식별자입니다. **기존 코드 절대 재용도 변경 금지.** 카테고리 분리 필요하면 같은 0xx 대역에서 새 번호 할당.
