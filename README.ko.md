# udit

[English](README.md) | [한국어](README.ko.md)

> 명령줄에서 Unity Editor를 조작하세요. AI 에이전트를 위해 만들었지만, 어떤 도구든 작동합니다.
>
> Udit (उदित) — 산스크리트어로 *떠오르다*. [DevBookOfArray](https://github.com/youngwoocho02)의 [unity-cli](https://github.com/youngwoocho02/unity-cli)를 fork하여 에이전트 중심 게임 개발 워크플로에 맞게 확장했습니다.

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**띄울 서버도, 작성할 설정도, 관리할 프로세스도 없습니다. 그냥 명령어를 칩니다.**

## 왜 만들었나

터미널에서 Unity를 조작하고 싶었습니다. 기존 MCP 기반 통합은 Python 런타임, WebSocket 릴레이, JSON-RPC 프로토콜 레이어, 설정 파일, 시작/종료 관리해야 하는 서버 프로세스, tool 등록 절차, 그리고 수만 줄의 과잉 엔지니어링된 코드를 요구했습니다. 단순히 Unity에 명령 하나 보내려고요.

게다가 Unity를 쓰고 싶은 모든 AI 에이전트가 자기 MCP 설정과 통합 작업을 따로 해야 했습니다. CLI는 그런 거 신경 안 씁니다 — shell 명령을 실행할 수 있는 어떤 에이전트든 즉시 사용 가능합니다.

뭔가 잘못됐다고 느꼈습니다. URL을 `curl`할 수 있다면, 왜 그 모든 게 필요한가요?

그래서 정반대를 만들었습니다: HTTP로 Unity와 직접 통신하는 단일 바이너리. 띄울 서버 없음 — Unity 패키지가 자동으로 listen합니다. 작성할 설정 없음 — Unity 인스턴스를 스스로 발견합니다. tool 등록 없음 — 이름으로 호출만 하면 됩니다. 캐싱도, 프로토콜 레이어도, 의식도 없습니다.

전체 CLI는 약 800줄의 Go (그리고 약 300줄의 도움말 텍스트)입니다. Unity 쪽 connector는 약 2,300줄의 C#. shell에서 Unity를 조작하게 해주는 얇은 레이어 — 그 이상도 이하도 아닙니다. 바이너리 설치하고, Unity 패키지 추가하면 작동합니다.

## 설치

### Linux / macOS

```bash
curl -fsSL https://raw.githubusercontent.com/momemoV01/udit/main/install.sh | sh
```

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/momemoV01/udit/main/install.ps1 | iex
```

### 다른 방법

```bash
# Go install (Go가 설치된 모든 플랫폼)
go install github.com/momemoV01/udit@latest

# 수동 다운로드 (플랫폼 선택)
# Linux amd64 / Linux arm64 / macOS amd64 / macOS arm64 / Windows amd64
curl -fsSL https://github.com/momemoV01/udit/releases/latest/download/udit-linux-amd64 -o udit
chmod +x udit && sudo mv udit /usr/local/bin/
```

지원 플랫폼: Linux (amd64, arm64), macOS (Intel, Apple Silicon), Windows (amd64).

### 업데이트

```bash
# 최신 버전으로 업데이트
udit update

# 설치 없이 업데이트 확인만
udit update --check
```

## Unity 설정

**Package Manager → Add package from git URL** 로 Unity Connector 패키지 추가:

```
https://github.com/momemoV01/udit.git?path=udit-connector
```

또는 `Packages/manifest.json` 에 직접 추가:
```json
"com.momemov01.udit-connector": "https://github.com/momemoV01/udit.git?path=udit-connector"
```

특정 버전을 고정하려면 URL에 태그 추가 (예: `#v0.2.21`).

추가 후 Unity가 열리면 Connector가 자동으로 시작됩니다. 추가 설정 불필요.

### 권장: Editor Throttling 비활성화

Unity는 기본적으로 창이 비활성 상태일 때 editor 업데이트를 throttle합니다. 이는 Unity 창을 다시 클릭할 때까지 CLI 명령이 실행 안 될 수 있다는 뜻입니다.

해결책: **Edit → Preferences → General → Interaction Mode** 를 **No Throttling** 으로 설정.

이렇게 하면 Unity가 백그라운드에 있어도 CLI 명령이 즉시 처리됩니다.

## 빠른 시작

```bash
# Unity 연결 확인
udit status

# play 모드 진입 후 대기
udit editor play --wait

# Unity 안에서 C# 코드 실행
udit exec "return Application.dataPath;"

# 콘솔 로그 읽기
udit console --type error,warning,log
```

## 작동 원리

```
Terminal                              Unity Editor
────────                              ────────────
$ udit editor play --wait
    │
    ├─ ~/.udit/instances/*.json 스캔
    │  → 포트 8590에서 Unity 발견
    │
    ├─ POST http://127.0.0.1:8590/command
    │  { "command": "manage_editor",
    │    "params": { "action": "play",
    │                "wait_for_completion": true }}
    │                                      │
    │                                  HttpServer 수신
    │                                      │
    │                                  CommandRouter 디스패치
    │                                      │
    │                                  ManageEditor.HandleCommand()
    │                                  → EditorApplication.isPlaying = true
    │                                  → PlayModeStateChange 대기
    │                                      │
    ├─ JSON 응답 수신  ←───────────┘
    │  { "success": true,
    │    "message": "Entered play mode (confirmed)." }
    │
    └─ 출력: Entered play mode (confirmed).
```

Unity Connector는:
1. Editor 시작 시 `localhost:8590` 에서 HTTP 서버를 엽니다
2. 프로젝트별 instance 파일을 `~/.udit/instances/` 에 작성하여 CLI가 어디로 연결할지 알게 합니다
3. 0.5초마다 현재 상태로 instance 파일을 갱신합니다 (heartbeat)
4. 매 요청마다 reflection으로 모든 `[UditTool]` 클래스를 발견합니다
5. 들어오는 명령을 메인 스레드의 매칭 핸들러로 라우팅합니다
6. 도메인 리로드 (script recompilation) 후에도 살아남습니다

컴파일하거나 리로드하기 전에, Connector는 상태(`compiling`, `reloading`)를 instance 파일에 기록합니다. 메인 스레드가 멈추면 timestamp가 더 이상 갱신되지 않습니다. CLI는 이를 감지하고 fresh timestamp를 기다린 후에만 명령을 보냅니다.

## 빌트인 명령

| 명령 | 설명 |
|---------|-------------|
| `editor` | Unity Editor play/stop/pause/refresh |
| `console` | 콘솔 로그 읽기, 필터링, 지우기 |
| `exec` | Unity 내부에서 임의의 C# 코드 실행 |
| `test` | EditMode/PlayMode 테스트 실행 |
| `menu` | Unity 메뉴 항목을 path로 실행 |
| `reserialize` | Unity의 시리얼라이저로 에셋 재직렬화 |
| `screenshot` | scene/game 뷰를 PNG로 캡처 |
| `profiler` | profiler 계층 읽기, 녹화 제어 |
| `list` | 모든 가용 tool과 파라미터 스키마 표시 |
| `status` | Unity Editor 연결 상태 표시 |
| `update` | CLI 바이너리 자체 업데이트 |
| `completion` | 셸 자동완성 스크립트 출력 |

### Editor 제어

```bash
# play 모드 진입
udit editor play

# play 모드 진입 후 완전히 로드될 때까지 대기
udit editor play --wait

# play 모드 종료
udit editor stop

# pause 토글 (play 모드에서만 동작)
udit editor pause

# 에셋 새로고침
udit editor refresh

# 새로고침 + 스크립트 재컴파일 (컴파일 완료 대기)
udit editor refresh --compile
```

### 콘솔 로그

```bash
# 에러와 경고 로그 읽기 (기본)
udit console

# 모든 타입의 마지막 20개 로그 항목
udit console --lines 20 --filter error,warning,log

# 에러만 읽기
udit console --type error

# 스택 트레이스 포함 (user: 사용자 코드만, full: 원본)
udit console --stacktrace user

# 콘솔 지우기
udit console --clear
```

### C# 코드 실행

Unity Editor 내부에서 런타임에 임의의 C# 코드를 실행. 가장 강력한 명령으로, UnityEngine, UnityEditor, ECS, 그리고 모든 로드된 어셈블리에 완전 접근 가능. 일회성 쿼리나 변경에 커스텀 tool 작성 불필요.

`return` 으로 출력을 받습니다. 일반적인 네임스페이스는 기본 포함. 프로젝트 고유 타입 (예: `Unity.Entities`)에만 `--usings` 추가. csc 컴파일러와 dotnet 런타임은 자동 감지; 실패 시 `--csc <path>` 또는 `--dotnet <path>`로 수동 지정.

```bash
udit exec "return Application.dataPath;"
udit exec "return EditorSceneManager.GetActiveScene().name;"
udit exec "return World.All.Count;" --usings Unity.Entities

# stdin으로 파이프 — 셸 이스케이핑 문제 회피
echo 'Debug.Log("hello"); return null;' | udit exec
echo 'var go = new GameObject("Marker"); go.tag = "EditorOnly"; return go.name;' | udit exec
```

`exec`은 진짜 C#을 컴파일·실행하므로 커스텀 tool로 할 수 있는 모든 걸 할 수 있습니다 — ECS 엔티티 검사, 에셋 수정, 내부 API 호출, editor 유틸리티 실행. AI 에이전트에겐 **tool 코드 한 줄도 안 쓰고 Unity 런타임 전체에 무마찰 접근**을 의미합니다. 복잡한 코드는 stdin 파이프로 셸 이스케이핑 두통 회피.

### 메뉴 항목

```bash
# Unity 메뉴 항목을 path로 실행
udit menu "File/Save Project"
udit menu "Assets/Refresh"
udit menu "Window/General/Console"
```

참고: `File/Quit` 은 안전상 차단됨.

### 에셋 재직렬화 (Reserialize)

AI 에이전트(와 사람)가 Unity 에셋 파일 — `.prefab`, `.unity`, `.asset`, `.mat` — 을 일반 텍스트 YAML로 편집할 수 있습니다. 하지만 Unity의 YAML 시리얼라이저는 엄격합니다: 필드 누락, 들여쓰기 잘못, stale `fileID` 가 에셋을 조용히 손상시킵니다.

`reserialize`가 이를 해결합니다. 텍스트 편집 후, Unity에게 에셋을 메모리에 로드한 다음 자체 시리얼라이저로 다시 쓰라고 지시합니다. 결과는 깨끗하고 유효한 YAML 파일 — Inspector로 편집한 것처럼.

```bash
# 전체 프로젝트 재직렬화 (인자 없음)
udit reserialize

# 텍스트 에디터로 prefab의 transform 값 편집 후
udit reserialize Assets/Prefabs/Player.prefab

# 여러 씬 일괄 편집 후
udit reserialize Assets/Scenes/Main.unity Assets/Scenes/Lobby.unity

# 머티리얼 속성 수정 후
udit reserialize Assets/Materials/Character.mat
```

이게 텍스트 기반 에셋 편집을 안전하게 만드는 핵심입니다. 이거 없이는 단 한 줄 잘못된 YAML 필드가 prefab을 깨뜨리고 런타임까지 가시 에러도 없습니다. 이거 있으면 **AI 에이전트는 일반 텍스트로 어떤 Unity 에셋도 자신 있게 수정할 수 있습니다** — prefab에 컴포넌트 추가, scene 계층 조정, 머티리얼 속성 변경 — 결과가 정상 로드될 거라 확신할 수 있습니다.

### Profiler

```bash
# profiler 계층 읽기 (마지막 프레임, 최상위)
udit profiler hierarchy

# 재귀 드릴다운
udit profiler hierarchy --depth 3

# 이름으로 root 설정 (substring match) — 특정 시스템에 집중
udit profiler hierarchy --root SimulationSystem --depth 3

# ID로 특정 항목 드릴다운
udit profiler hierarchy --parent 4 --depth 2

# 마지막 30 프레임 평균
udit profiler hierarchy --frames 30 --min 0.5

# 특정 프레임 범위 평균
udit profiler hierarchy --from 100 --to 200

# 필터 + 정렬
udit profiler hierarchy --min 0.5 --sort self --max 10

# profiler 녹화 활성화/비활성화
udit profiler enable
udit profiler disable

# profiler 상태 표시
udit profiler status

# 캡처된 프레임 지우기
udit profiler clear
```

### 테스트 실행

Unity Test Framework로 EditMode와 PlayMode 테스트 실행.

```bash
# EditMode 테스트 실행 (기본)
udit test

# PlayMode 테스트 실행
udit test --mode PlayMode

# 테스트 이름으로 필터 (substring match)
udit test --filter MyTestClass
```

Unity Test Framework 패키지 필요. PlayMode 테스트는 도메인 리로드를 트리거 — CLI가 결과를 자동 폴링합니다.

### Tool 목록

```bash
# 모든 가용 tool (빌트인 + 프로젝트 커스텀)을 파라미터 스키마와 함께 표시
udit list
```

### 셸 자동완성

```bash
# Bash   (세션)       : source <(udit completion bash)
# Zsh    (세션)       : source <(udit completion zsh)
# PowerShell          : udit completion powershell | Out-String | Invoke-Expression
# Fish                : udit completion fish > ~/.config/fish/completions/udit.fish
```

각 completion 스크립트는 sentinel 주석으로 감싸져 있습니다:
```
# >>> udit completion >>>
...
# <<< udit completion <<<
```

**안전한 재설치** — `>> $PROFILE` (또는 `>> ~/.bashrc`)를 두 번 실행하면 블록이 중복 삽입되어 셸 초기화가 **깨집니다**. 반드시 이전 블록을 먼저 제거한 뒤 새로 추가하세요.

Bash / Zsh:
```bash
# 기존 udit completion 블록 제거
sed -i '/^# >>> udit completion >>>/,/^# <<< udit completion <<</d' ~/.bashrc
# zsh는 ~/.zshrc
udit completion bash >> ~/.bashrc
```

PowerShell (Windows / 크로스 플랫폼):
```powershell
$p = Get-Content $PROFILE -Raw
$p = $p -replace '(?s)# >>> udit completion >>>.*?# <<< udit completion <<<\r?\n?', ''
Set-Content $PROFILE -Value $p.TrimEnd() -Encoding utf8
udit completion powershell | Out-File -Append -Encoding utf8 $PROFILE
```

Fish는 별도 정리 불필요 — 각 completion이 자기 파일에 있어서 `~/.config/fish/completions/udit.fish` 덮어쓰기가 항상 안전합니다.

### 커스텀 Tool

```bash
# 커스텀 tool을 이름으로 직접 호출
udit my_custom_tool

# 파라미터와 함께 호출
udit my_custom_tool --params '{"key": "value"}'
```

### 상태

```bash
# Unity Editor 상태 표시
udit status
# 출력: Unity (port 8590): ready
#   Project: /path/to/project
#   Version: 6000.1.0f1
#   PID:     12345
```

CLI는 명령을 보내기 전에 Unity 상태도 자동 확인합니다. Unity가 바쁘면 (컴파일/리로드 중), Unity가 응답 가능해질 때까지 기다립니다.

## 글로벌 옵션

| 플래그 | 설명 | 기본값 |
|------|-------------|---------|
| `--port <N>` | 특정 Unity 인스턴스 포트 지정 (자동 발견 건너뜀) | auto |
| `--project <path>` | 프로젝트 경로로 Unity 인스턴스 선택 | latest |
| `--timeout <ms>` | HTTP 요청 타임아웃 | 120000 |
| `--json` | 모든 응답을 정형 JSON envelope으로 (`error_code` 포함) | off |

```bash
# 특정 Unity 인스턴스에 연결
udit --port 8591 editor play

# 여러 Unity 인스턴스가 열려 있을 때 프로젝트 경로로 선택
udit --project MyGame editor stop

# JSON 출력 (에이전트 친화)
udit --json status
```

`--json` 응답의 `error_code` 필드는 안정적인 식별자 (`UCI-001`, `UCI-020` 등) — 영어 메시지 텍스트 파싱 대신 코드로 분기. 전체 코드 레지스트리는 [`docs/ERROR_CODES.ko.md`](./docs/ERROR_CODES.ko.md) 참고.

어떤 명령이든 `--help` 로 자세한 사용법:

```bash
udit editor --help
udit exec --help
udit profiler --help
```

## 프로젝트 설정 파일 (`.udit.yaml`)

프로젝트 루트(또는 상위)에 `.udit.yaml` 을 두면 매번 `--port`/`--timeout`/`--usings` 반복 입력 제거.

```yaml
# 프로젝트 루트 .udit.yaml
default_port: 8590           # --port 미지정 시 사용
default_timeout_ms: 120000   # --timeout 미지정 시 사용
exec:
  usings:                    # 모든 udit exec 호출에 자동 추가
    - Unity.Entities
    - MyGame.Core
```

검색 규칙: cwd → 상위로 walk-up, `$HOME` 직전에 멈춤. 첫 발견된 `.udit.yaml` 사용.
우선순위: **CLI 플래그 > config 파일 > 빌트인 기본값**.


## 커스텀 Tool 작성

Editor 어셈블리에 `[UditTool]` 어트리뷰트가 붙은 정적 클래스 생성. Connector가 도메인 리로드 시 자동 발견.

```csharp
using UditConnector;
using Newtonsoft.Json.Linq;
using UnityEngine;

[UditTool(Name = "spawn", Description = "Spawn an enemy at a position", Group = "gameplay")]
public static class SpawnEnemy
{
    public class Parameters
    {
        [ToolParameter("X world position", Required = true)]
        public float X { get; set; }

        [ToolParameter("Y world position", Required = true)]
        public float Y { get; set; }

        [ToolParameter("Z world position", Required = true)]
        public float Z { get; set; }

        [ToolParameter("Prefab name in Resources folder", DefaultValue = "Enemy")]
        public string Prefab { get; set; }
    }

    public static object HandleCommand(JObject parameters)
    {
        var p = new ToolParams(parameters);
        float x = p.GetFloat("x", 0);
        float y = p.GetFloat("y", 0);
        float z = p.GetFloat("z", 0);
        string prefabName = p.Get("prefab", "Enemy");

        var prefab = Resources.Load<GameObject>(prefabName);
        var instance = Object.Instantiate(prefab, new Vector3(x, y, z), Quaternion.identity);

        return new SuccessResponse("Enemy spawned", new
        {
            name = instance.name,
            position = new { x, y, z }
        });
    }
}
```

플래그 또는 JSON으로 직접 호출:

```bash
udit spawn --x 1 --y 0 --z 5 --prefab Goblin
udit spawn --params '{"x":1,"y":0,"z":5,"prefab":"Goblin"}'
```

**핵심 포인트:**

- **Name**: `Name` 없으면 클래스명에서 자동 도출 (`SpawnEnemy` → `spawn_enemy`, `UITree` → `ui_tree`). `Name = "spawn"` 이면 명령이 `udit spawn`.
- **Parameters 클래스**: 선택이지만 권장. `udit list` 가 이걸로 파라미터 이름/타입/설명/필수 여부를 노출 — AI 어시스턴트가 소스를 안 읽고도 tool을 발견할 수 있습니다.
- **ToolParams**: `p.Get()`, `p.GetInt()`, `p.GetFloat()`, `p.GetBool()`, `p.GetRaw()` 로 일관된 파라미터 읽기.
- **발견**: `udit list` 가 빌트인 tool 먼저 (`group: "built-in"`), 그 다음 연결된 Unity 프로젝트에서 발견된 커스텀 tool (`group: "custom"`) 표시.

**어트리뷰트 레퍼런스:**

| 어트리뷰트 | 속성 | 설명 |
|---|---|---|
| `[UditTool]` | `Name` | 명령 이름 오버라이드 (기본: 클래스명 → snake_case) |
| | `Description` | `list` 에 표시될 tool 설명 |
| | `Group` | 분류용 그룹 이름 |
| `[ToolParameter]` | `Description` | 파라미터 설명 (생성자 인자) |
| | `Required` | 필수 파라미터 여부 (기본: `false`) |
| | `Name` | 파라미터 이름 오버라이드 |
| | `DefaultValue` | 기본값 힌트 |

### 규칙

- 클래스는 `static` 이어야 함
- `public static object HandleCommand(JObject parameters)` 또는 `async Task<object>` 변형 필수
- `SuccessResponse(message, data)` 또는 `ErrorResponse(message)` 반환
- discoverability를 위해 `Parameters` nested 클래스에 `[ToolParameter]` 어트리뷰트 추가
- 클래스명이 명령 이름으로 자동 snake_case 변환
- 필요 시 `[UditTool(Name = "my_name")]` 으로 오버라이드
- Unity 메인 스레드에서 실행 — 모든 Unity API 안전하게 호출 가능
- Editor 시작 시와 매 스크립트 재컴파일 후 자동 발견
- 중복 tool 이름은 감지되어 에러로 로깅 — 첫 번째 발견된 핸들러만 사용

## 여러 Unity 인스턴스

여러 Unity Editor가 열려 있으면, 각각 다른 포트(8590, 8591, ...)에 등록됩니다:

```bash
# 모든 실행 중 인스턴스 확인
ls ~/.udit/instances/

# 프로젝트 경로로 선택
udit --project MyGame editor play

# 포트로 선택
udit --port 8591 editor play

# 기본: 가장 최근 등록된 인스턴스 사용
udit editor play
```

## MCP와 비교

| | MCP | udit |
|---|-----|-----------|
| **설치** | Python + uv + FastMCP + 설정 JSON | 단일 바이너리 |
| **의존성** | Python 런타임, WebSocket 릴레이 | 없음 |
| **프로토콜** | JSON-RPC 2.0 over stdio + WebSocket | 직접 HTTP POST |
| **세팅** | MCP 설정 생성, AI 도구 재시작 | Unity 패키지 추가, 끝 |
| **재연결** | 도메인 리로드 위한 복잡한 재연결 로직 | 요청마다 stateless |
| **호환성** | MCP 호환 클라이언트만 | shell 있는 모든 것 |
| **커스텀 tool** | 동일 `[Attribute]` + `HandleCommand` 패턴 | 동일 |

## 로드맵

`v0.1.0` (현재 기반선)부터 `v1.0.0` (API 동결, production-ready)까지의 단계별 계획은 [`docs/ROADMAP.ko.md`](./docs/ROADMAP.ko.md) 참고. 하이라이트:

- **v0.2.0 — Foundation** ✅ — 버그 픽스, 글로벌 `--json` 출력, 에러 코드 레지스트리, `.udit.yaml` config, 셸 완성
- **v0.3.0 — Observe** — `scene` / `go` / `asset` / `component` 쿼리 명령 (읽기에 더 이상 `exec` 불필요)
- **v0.4.0 — Mutate** — GameObject / 컴포넌트 / prefab 생성·수정·삭제
- **v0.5.0 — Automate** — `build player`, `package` (UPM), 확장된 `test`, project preflight
- **v0.6.0 — Stream** — `watch` 모드, SSE로 `log tail --follow`
- **v1.0.0 — Polish & Freeze** — 50%+ 테스트 커버리지, cookbook 문서, 5년 API commitment

## 감사의 말

udit은 **DevBookOfArray** (youngwoocho02)가 만든 **[unity-cli](https://github.com/youngwoocho02/unity-cli)** 의 fork입니다. 원본의 — 아키텍처, HTTP 브리지, reflection 기반 tool 발견, heartbeat 설계, 도메인 리로드 처리 — 가 이 프로젝트의 완전한 기반을 이룹니다. fork는 DevBookOfArray의 명시적 허락 하에, 자체 정체성으로 에이전트 중심 로드맵을 추구하기 위해 존재합니다. 자세한 attribution은 [NOTICE.md](./NOTICE.md) 참고.

udit이 유용하다면, 원본도 별표 부탁드리고 작성자도 구독해주세요:

[![Original](https://img.shields.io/badge/Original-unity--cli-success?logo=github)](https://github.com/youngwoocho02/unity-cli)
[![YouTube](https://img.shields.io/badge/YouTube-DevBookOfArray-red?logo=youtube&logoColor=white)](https://www.youtube.com/@DevBookOfArray)

## 메인테이너

**momemo** 가 유지보수합니다 ([![GitHub](https://img.shields.io/badge/GitHub-momemoV01-181717?logo=github)](https://github.com/momemoV01))

## 라이선스

MIT — [LICENSE](./LICENSE) 참고. 원본과 fork 양쪽의 저작권 표기는 모든 복사본에 보존되어야 합니다.
