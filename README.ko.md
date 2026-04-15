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

### 씬 관리

`exec`를 거치지 않고 씬을 조회·전환할 수 있다. `--json`을 붙이면 모든 서브커맨드가 정형 JSON으로 응답하므로 에이전트가 결과를 체이닝하기 좋다.

```bash
# 프로젝트의 모든 씬 에셋 목록 (Assets + Packages)
udit scene list

# 현재 활성 씬 정보 (path, guid, dirty 여부, root GameObject 수)
udit scene active

# 단일 활성 씬으로 열기
udit scene open Assets/Scenes/Main.unity

# 열려 있는 씬 중 dirty한 것만 저장
udit scene save

# 활성 씬을 다시 로드해 변경사항 폐기 (dirty면 --force 필요)
udit scene reload --force

# 활성 씬 하이어라키를 stable ID가 포함된 JSON 트리로 덤프
udit scene tree --depth 3
udit scene tree --active-only --json
```

**Dirty 가드.** 활성 씬에 저장되지 않은 변경사항이 있을 때 `scene open`과 `scene reload`는 실행을 거부한다. 버리려면 `--force`를 붙이고, 보존하려면 먼저 `scene save`를 호출한다. 두 명령 모두 플레이 모드 중에는 차단된다.

**Stable ID.** `scene tree`의 모든 GameObject는 Unity `GlobalObjectId`를 해시한 `go:XXXXXXXX` id를 가진다. Editor를 재시작해도 동일 GO는 동일 id를 얻는 결정적 포맷이므로, 에이전트가 이전 세션의 결과를 저장했다가 아래 `go` 명령으로 재조회할 수 있다. 예시 `roots` 항목:

```json
{
  "id": "go:9598abb1",
  "name": "Main Camera",
  "active": true,
  "components": ["Transform", "Camera", "AudioListener"],
  "children": []
}
```

### GameObject 쿼리

로드된 씬의 GameObject를 조회한다. 모든 결과는 `scene tree`와 동일한 `go:XXXXXXXX` id 포맷으로 키잉되어 있고, 에디터 재시작 전(또는 GO 파괴 전)까지 라이브 GameObject로 resolve된다.

```bash
# 로드된 씬의 모든 GameObject (페이지네이션됨)
udit go find

# 이름/태그/컴포넌트 타입 조합 필터 (AND)
udit go find --name "Enemy*" --tag Enemy --component Rigidbody

# 큰 씬 페이지네이션 — 페이지당 20개
udit go find --limit 20 --offset 0

# 한 GameObject의 전체 덤프: scene, path, parent_id, children_ids,
# 그리고 모든 컴포넌트의 serialized 프로퍼티
udit go inspect go:9598abb1

# 계층 경로 문자열만 ("Root/Child/Leaf")
udit go path go:9598abb1
```

`go inspect`는 컴포넌트별 타입화된 값을 반환: Transform은 world/local 좌표 모두 특별 처리, enum은 `{value, name}`, ObjectReference는 `{type, name, path, guid}`, 20개 초과 배열은 `{count, elements, truncated: true}`로 클립. 누락된 스크립트는 `"<Missing Script>"`로 표시되어 오래된 prefab 탐지 가능.

알 수 없거나 만료된 id는 `UCI-042 GameObjectNotFound`를 반환 — `go find` 또는 `scene tree`로 stable-ID 레지스트리를 재시딩한 뒤 새 id로 재시도한다.

#### GameObject 변경 (v0.4.0+)

`go` 네임스페이스는 다섯 가지 기본 씬 편집 연산도 노출한다. 각 연산은 **Unity Undo를 통해 실행**되므로, Editor에서 Ctrl+Z로 사람이 인스펙터에서 편집한 것과 동일하게 되돌릴 수 있다. 활성 씬은 dirty로 마크되어 닫을 때 표준 저장 프롬프트가 뜬다.

```bash
# GameObject 생성. 새 go: ID 반환; --pos 는 local position float.
udit go create --name Boss
udit go create --name Minion --parent go:abcd1234 --pos 0,5,0

# GameObject + 모든 자손 파괴. children_affected 가 cascade 크기 보고.
udit go destroy go:5678abcd

# 부모 변경. --parent 생략 시 씬 루트로.
# 사이클 (자기 자신 또는 자손 아래로 이동) 은 사전에 거부됨.
udit go move go:5678abcd --parent go:abcd1234
udit go move go:5678abcd

# 이름 변경.
udit go rename go:5678abcd "Renamed_Boss"

# activeSelf 토글. 이미 그 상태면 no_change=true 로 success 반환.
udit go setactive go:5678abcd --active false
```

**모든 mutation은 `--dry-run` 지원.** 다섯 서브커맨드 중 어느 것에든 `--dry-run`을 붙이면 씬을 건드리지 않고 변경 영향을 미리 보여준다. 응답에는 실제 실행 시와 같은 필드(`would_destroy`, `children_affected`, `from`/`to` 등)가 포함되어 에이전트가 commit 전에 영향 범위로 분기할 수 있다.

```bash
udit go destroy go:5678abcd --dry-run
# {
#   "would_destroy": "Root/Boss",
#   "children_affected": 12,
#   "components": ["Transform", "Rigidbody", "PlayerController"],
#   "dry_run": true
# }
```

플레이 모드 중에는 mutation 차단. 각 mutation은 자기만의 Undo 그룹(Editor의 Edit → Undo 메뉴에 표시되는 설명적인 이름) 을 가지므로, `Undo.PerformUndo`가 에이전트 세션 전체를 한 번에 collapse 하지 않고 한 번에 한 논리 연산만 되돌린다.

### 컴포넌트 쿼리

GameObject 전체를 다시 덤프하지 않고 특정 컴포넌트(또는 필드)만 zoom-in. 필드 이름은 `go inspect`가 내보내는 것과 동일 — 전체 체인에서 동일 어휘 사용.

```bash
# GameObject에 붙은 컴포넌트 목록
udit component list go:9598abb1

# 특정 컴포넌트 한 개의 모든 필드 덤프
udit component get go:9598abb1 Transform

# 단일 필드 zoom-in; 점 표기로 중첩 객체 탐색
udit component get go:9598abb1 Transform position
udit component get go:9598abb1 Transform position.z

# 같은 타입이 여러 개면 --index 로 선택
udit component get go:abcd1234 BoxCollider --index 1

# 타입의 serialized-property 스키마
# (로드된 씬에 live 인스턴스가 있어야 함)
udit component schema Camera
udit component schema UnityEngine.Transform
udit component schema MyGame.PlayerController
```

타입 이름은 **대소문자 무시**. 짧은 이름(`Camera`)은 `UnityEngine.*` 우선 매칭되므로, 프로젝트에서 built-in을 섀도잉하는 타입이면 전체 네임스페이스(`MyGame.Camera`)로 명시.

실패 케이스:
- GameObject id 미지/만료 → `UCI-042` (→ `go find`로 재시딩).
- 해당 타입이 GO에 없음, `--index` 범위 밖, `schema`에 live 인스턴스 없음 → `UCI-043`. 메시지가 실제 붙은 타입 목록 또는 인스턴스 수를 알려줌.
- 필드 경로 없음 → `UCI-011` + 유효한 top-level 필드 목록.

#### Component 변경 (v0.4.0+)

`go` 변경과 동일한 Undo 통합 + `--dry-run` 표면. 필드 이름이 `component get`과 일치하므로 read/write vocabulary가 통일되어 있음.

```bash
# 컴포넌트 추가. DisallowMultipleComponent + RequireComponent 존중.
udit component add go:9598abb1 --type Rigidbody

# 제거. Transform은 차단 (대신 `go destroy` 사용).
udit component remove go:9598abb1 Rigidbody

# 필드 쓰기. 값은 필드의 SerializedPropertyType에 따라 파싱.
udit component set go:9598abb1 Transform position 0,10,0
udit component set go:9598abb1 Rigidbody m_Mass 2.5
udit component set go:9598abb1 Camera m_BackGroundColor "#FF8800"
udit component set go:9598abb1 Camera m_BackGroundColor "1,0,0,1"
udit component set go:9598abb1 Camera m_ClearFlags "Solid Color"

# 같은 타입 여러 개 붙어 있으면 --index 로 선택.
udit component set go:abcd1234 BoxCollider m_Size 1,1,1 --index 1

# GameObject 간 컴포넌트 복사 (없으면 dst에 add 먼저).
udit component copy go:aaaa1111 Rigidbody go:bbbb2222
```

**값 파싱 요약:**
| SerializedPropertyType | 입력 포맷 |
| --- | --- |
| Integer / LayerMask / Character | `"42"` |
| Boolean | `"true"`, `"false"`, `"1"`, `"0"`, `"yes"`, `"no"`, `"on"`, `"off"` |
| Float | `"3.14"` |
| String | 임의 텍스트 |
| Vector2 / 3 / 4 / Quaternion | 쉼표로 구분한 float (`"x,y"`, `"x,y,z"`, `"x,y,z,w"`) |
| Color | 0–1 범위 float `"r,g,b[,a]"` 또는 `"#RRGGBB[AA]"` |
| Enum | display name (`"Solid Color"`) 또는 value index |
| ObjectReference | 에셋 경로 (`"Assets/Sprites/Player.png"`) 또는 clear는 `"null"` / `"none"` |

Transform은 `component set`에서도 **virtual field**를 노출: `position`, `local_position`, `rotation_euler`, `local_rotation_euler`, `local_scale` — 모두 `"x,y,z"`. `component get`이 반환하는 것과 동일하므로 round-trip 가능.

**ObjectReference**는 어떤 프로젝트 에셋 경로든 받음. 서브에셋이 있는 경로(예: `.png`가 `Texture2D` + `Sprite`로 임포트된 경우)는 `component set`이 **타겟 필드 타입에 assign 가능한 첫 서브에셋**을 자동 선택. 해당 경로에 호환되는 에셋이 없으면 `UCI-011` + 기대 타입 + 실제 발견된 타입 표시. 씬 오브젝트 참조(`go:XXXXXXXX`)는 이 버전에서 `component set`으로 쓰기 불가 — `udit exec`로 우회.

AnimationCurve / Gradient / ExposedReference / ManagedReference은 여전히 **읽기 전용**; set 시 `UCI-011` + 안내 메시지 반환.

### 에셋 쿼리

AssetDatabase 조회 — Prefab, Texture, Material, Script, Unity가 인덱싱하는 모든 것. 경로는 프로젝트 상대(`Assets/...` 또는 `Packages/...`), GUID는 Unity의 32자 hex.

```bash
# 타입 필터 (Unity의 't:' 문법으로 매핑), 레이블, 이름 glob, 폴더 스코프
udit asset find --type Prefab
udit asset find --type Texture2D --folder Assets/Art --limit 20
udit asset find --label boss --name "*Enemy*"

# 메타데이터 + 타입별 'details' 블록
# (Texture2D: 크기+포맷, Material: 쉐이더+프로퍼티,
#  Prefab: 루트 컴포넌트, AudioClip: 길이+채널 등)
udit asset inspect Assets/Materials/Player.mat

# 이 에셋이 의존하는 것들. 기본은 direct only, --recursive 로 transitive.
udit asset dependencies Assets/Scenes/Main.unity
udit asset dependencies Assets/Scenes/Main.unity --recursive

# 이 에셋을 참조하는 것들. Unity는 역인덱스 없어서 프로젝트 전체 스캔.
# 응답에 scan_ms + scanned_assets 포함 — 에이전트가 비용 인지 가능.
# 큰 프로젝트에서는 반드시 --limit 설정.
udit asset references Assets/Prefabs/Enemy.prefab --limit 50

# GUID / path 왕복
udit asset guid Assets/Scenes/SampleScene.unity
udit asset path 8c9cfa26abfee488c85f1582747f6a02
```

`inspect`는 Prefab, Texture2D, Material, AudioClip, ScriptableObject, TextAsset에 타입별 details 블록 제공. 다른 타입도 공통 헤더(`path`, `guid`, `name`, `type`, `labels`)는 반환하고 `details: null` 이므로 에이전트가 최소한 타입으로 분기 가능.

알 수 없는 path/GUID는 `UCI-040 AssetNotFound` — `asset find`로 먼저 식별자를 검증.

#### 에셋 변경 (v0.4.x+)

에셋 생성, 이동, 삭제, 라벨 관리. 모든 mutation이 `--dry-run` 지원, `delete`는 기본적으로 OS 휴지통으로 이동 (복구 가능).

```bash
# ScriptableObject 파생 에셋 생성. --path가 '/'로 끝나면 <TypeName>.asset 자동 추가;
# 명시적 파일명을 주면 그대로 사용.
udit asset create --type MyGame.GameConfig --path Assets/Config/
udit asset create --type MyGame.GameConfig --path Assets/Config/Custom.asset

# 폴더 생성은 sentinel 타입 "Folder" 사용.
udit asset create --type Folder --path Assets/NewFolder

# move는 GUID 유지 — 프로젝트의 기존 참조가 모두 살아있음.
udit asset move Assets/Old.prefab Assets/New/Moved.prefab

# 삭제: 기본은 OS 휴지통 (복구 가능), --permanent로 완전 삭제.
udit asset delete Assets/Unused.prefab
udit asset delete Assets/Unused.prefab --permanent

# --permanent는 프로젝트 전체를 스캔해서 이 에셋을 참조하는 다른 에셋 수를
# referenced_by로 보고 — caller가 영향 범위를 미리 확인 가능.
udit asset delete Assets/Shared.mat --permanent --dry-run

# 라벨: add/remove는 하나 이상, set은 전체 교체, clear는 전부 삭제, list는 읽기.
udit asset label add    Assets/Prefabs/Boss.prefab boss_content critical
udit asset label remove Assets/Prefabs/Boss.prefab critical
udit asset label list   Assets/Prefabs/Boss.prefab
udit asset label set    Assets/Prefabs/Boss.prefab final_content
udit asset label clear  Assets/Prefabs/Boss.prefab
```

**Undo 주의.** AssetDatabase 연산 (Create/Move/Delete/SetLabels)은 Unity의 씬 Undo에 **참여하지 않음**. Editor의 Ctrl+Z로 되돌리기 불가. 안전장치는 `--dry-run` (side-effect 없이 preview)과 `delete`의 기본 `MoveAssetToTrash` (OS 휴지통에서 복구 가능). 불확실하면 dry-run 먼저.

실패 케이스:
- 없는 path/GUID → `UCI-040`.
- `create --type X`에서 X가 ScriptableObject 파생이 아니거나 sentinel `Folder`도 아닌 경우 → `UCI-011` + 지원 타입 안내. UnityEngine 타입과 disambiguate하려면 full name (`MyGame.GameConfig`) 사용.
- `create` / `move` destination 이미 존재 → `UCI-011` (먼저 move 또는 delete).
- 라벨 op이 `add / remove / list / set / clear`가 아니면 → `UCI-011`.

### Prefab

`scene` + `go` + `asset` 위의 Prefab 연산. `instantiate`는 `PrefabUtility.InstantiatePrefab`을 사용하므로 씬 인스턴스가 에셋과의 link를 유지 (`Object.Instantiate`와 다름). 모든 연산 Unity Undo 통과.

```bash
# prefab asset의 씬 인스턴스 생성. --pos는 localPosition.
udit prefab instantiate Assets/Prefabs/Enemy.prefab
udit prefab instantiate Assets/Prefabs/Enemy.prefab --parent go:abcd1234 --pos 5,0,0

# 씬 인스턴스를 일반 GameObject로 변환 (prefab link 끊김).
udit prefab unpack go:5678abcd                       # 외곽 루트만
udit prefab unpack go:5678abcd --mode completely     # 중첩 prefab까지 전부

# 씬 인스턴스의 override를 prefab 에셋에 commit.
# 인스턴스 내부 어느 GO든 받음 — 외곽 루트로 자동 resolve.
udit prefab apply go:5678abcd

# 주어진 prefab의 모든 씬 인스턴스 찾기.
udit prefab find-instances Assets/Prefabs/Enemy.prefab
```

**Unpack 시 stable ID 변경됨.** prefab 인스턴스가 unpack되면 Unity의 `GlobalObjectId`가 바뀌고 (더 이상 에셋과 연결 안 됨), 결과적으로 stable ID도 바뀐다. `unpack` 응답에 **새 ID**가 포함되므로 이후 연산에 그것을 사용. 옛 ID는 `UCI-042` 반환.

실패 케이스:
- 경로에 에셋 없음 → `UCI-040`.
- 경로 존재하지만 GameObject 아님 (예: 스크립트 파일) → `UCI-011` + `asset inspect`로 실제 타입 확인 힌트.
- GameObject 에셋이지만 prefab이 아님 (예: 날것의 모델) → `UCI-011`.
- `unpack`/`apply`에 prefab 인스턴스가 아닌 GO → `UCI-011`.

### 트랜잭션

트랜잭션 없이는 모든 mutation (`go create`, `component set`, `prefab instantiate`, ...)이 자기만의 Unity Undo 그룹을 만들어서, 다단계 에이전트 변경을 되돌리려면 Ctrl+Z를 N번 눌러야 한다. 트랜잭션 안에서 `commit`하면 `begin` 이후의 모든 mutation이 하나의 Undo 엔트리로 합쳐지고, Editor에서 Ctrl+Z 한 번으로 전체가 reverse된다.

```bash
# Single-Undo batch
udit tx begin --name "Spawn boss setup"
udit go create --name Boss
udit component add go:abcd1234 --type Rigidbody
udit component set go:abcd1234 Rigidbody m_Mass 5.5
udit tx commit                       # Ctrl+Z 한 번으로 3개 변경 모두 되돌림

# 작업 중간 revert
udit tx begin --name "Try a layout"
udit go create --name Candidate
udit go move go:abcd1234 --parent go:5678abcd
udit tx rollback                     # begin 이후 모든 변경 되감기

# 현재 상태 확인
udit tx status
```

구현: `begin`은 현재 Unity Undo 그룹 인덱스를 캡처, `commit`은 `Undo.CollapseUndoOperations(savedGroup)`, `rollback`은 `Undo.RevertAllDownToGroup(savedGroup)`. commit 후 트랜잭션 이름이 Edit → Undo에 표시되어, 프로젝트에 나중에 합류하는 사람이 "Undo udit go create 'Boss'" 대신 "Undo Spawn boss setup"을 봄.

알아둘 제약:
- **Unity 인스턴스당 한 개의 트랜잭션만.** Undo 스택은 전역이라 동시에 한 트랜잭션만 가능. 활성 tx가 있는 상태에서 `begin`하면 기존 트랜잭션의 이름 + 경과 시간과 함께 `UCI-011` 반환.
- **도메인 리로드가 핸들을 지움.** 스크립트 재컴파일은 connector static 상태를 내리므로, 트랜잭션 중간에 리로드가 일어나면 부분 mutation은 Undo 스택에 남되 트랜잭션 핸들 자체는 사라진다. `tx status`가 "no active"로 바뀌므로, 계속 묶고 싶었으면 새로 `begin`.
- **AssetDatabase 변경은 참여 안 함.** `asset create/move/delete/label`은 디스크에 바로 쓰고 씬 Undo 그룹에 묶일 수 없음. 트랜잭션 안에서 실행은 되지만 commit/rollback으로 되돌릴 수는 없음 (씬 mutation과 달리).

### Project (프로젝트 조회 + 헬스체크)

빌드 전 헬스체크 + 프로젝트 인스펙션. Automate 단계의 첫 블록 — "이 프로젝트가 뭐지?" / "빌드 걸기 전에 문제없나?"를 빠르게 답해준다.

```bash
# Unity 버전, 빌드 타겟, 패키지, 빌드씬, 에셋 카운트
udit project info

# Prefab 전체에서 missing script scan + Build Settings 체크
udit project validate                   # Assets/ only (빠름)
udit project validate --include-packages  # Packages/도 포함

# validate + player-settings + 컴파일 상태 체크
udit project preflight
```

`project info`는 async `PackageManager.Client.List`를 쓰지 않고 `Packages/manifest.json`을 직접 읽어서 빠르다 (선언된 버전만, resolved graph 필요하면 `exec`로 우회). 에셋 카운트는 `AssetDatabase.FindAssets`로, 12k 에셋 프로젝트에서도 수백 ms 안에 응답.

`project validate`는 모든 prefab을 순회하며 `GameObjectUtility.GetMonoBehavioursWithMissingScriptCount`로 깨진 참조를 센다. 응답에 `scan_ms` 포함 — 에이전트가 캐싱 여부 판단 가능. `--limit`은 severity당 기본 100.

`project preflight`는 `validate` + 빌드 전 위생 체크: 빈 `productName`, 기본값 `"DefaultCompany"`, 컴파일 중 상태 경고. 후속 slice에서 추가될 `udit build player` 걸기 전에 이름 누락 / 씬 누락 / 컴파일 문제를 먼저 잡는 용도.

### Package (UPM 패키지 관리)

Unity Package Manager (UPM) 조작. 에이전트가 선언된 의존성을 조회하고, registry 또는 git URL에서 패키지를 설치/제거하고, 메타데이터를 조회하고, manifest를 강제 재해결할 수 있다 — `exec`로 우회하거나 `Packages/manifest.json`을 직접 편집할 필요 없음.

```bash
# 선언된 의존성, Packages/manifest.json 직접 파싱 (1초 미만)
udit package list

# 전이 의존성 포함 resolved graph (1-3초, registry 호출)
udit package list --resolved

# 설치 — 이름, name@version, git URL 모두 수용
udit package add com.unity.cinemachine
udit package add com.unity.cinemachine@2.9.7
udit package add https://github.com/dbrizov/NaughtyAttributes.git

# 제거 + 메타데이터 + 검색
udit package remove com.unity.cinemachine
udit package info com.unity.cinemachine
udit package search cinemachine

# 강제 재해결 (manifest.json을 외부에서 편집한 후)
udit package resolve
```

`package list` (기본)은 `Packages/manifest.json`을 직접 읽어 1초 미만 응답. 반환: `{ source: "manifest", count, packages[] }` (각 항목 `{ name, version_declared, kind }`, kind는 `registry` / `git` / `file`). `--resolved`는 `PackageManager.Client.List`로 전환해 전이 의존성 + 실제 설치 source까지 — 1-3초 소요.

`package add`는 id를 그대로 `Client.Add`에 넘긴다. Unity가 형식(레지스트리 이름, 버전 핀, git URL)을 파싱하고 성공 시 도메인 리로드 트리거. 응답에 resolved `{ name, version, source, package_id }`. `remove`는 대칭의 `Client.Remove`. 둘 다 registry에 따라 몇 초 걸리고 Editor 재컴파일 발생.

`package info`는 단일 패키지 메타 (현재 버전, latest, latest_release, 설명, registry, 최근 10 버전). `package search`는 전체 registry 카탈로그를 substring 매칭하고 50개로 cap — 에이전트 컨텍스트를 넘치지 않게 패키지 발견 가능.

`package resolve`는 `Client.Resolve` 호출 (없으면 `AssetDatabase.Refresh` fallback) — `manifest.json`을 외부에서 편집했거나 이전 resolve가 중단됐을 때 사용.

모든 async 동작은 editor tick에서 폴링. `add`/`remove` 도중 도메인 리로드가 발생하면 응답이 잘릴 수 있음 — 이 경우 `package list`로 사후 상태 재확인.

### Build (플레이어 빌드)

Unity의 `BuildPipeline`을 CLI에서 직접 다룬다. 지원 가능한 타겟 발견, 스탠드얼론 플레이어 빌드, Addressables 콘텐츠 빌드, 진행 중 빌드 취소를 — `exec` 우회나 커스텀 빌드 에디터 작성 없이.

```bash
# 로컬 에디터가 빌드 가능한 타겟 발견
udit build targets

# 스탠드얼론 플레이어 빌드. Long-running — 보통 30초 ~ 수 분
udit build player --target win64 --output builds/win64/
udit build player --target android --output builds/app.apk \
    --scenes Assets/Scenes/Main.unity,Assets/Scenes/Boot.unity
udit build player --target win64 --output builds/dev/ --development

# Addressables (com.unity.addressables 패키지 필요)
udit build addressables
udit build addressables --profile MobileRelease

# 진행 중 빌드 취소
udit build cancel
```

`build targets`는 모든 `BuildTarget` enum을 순회해 `{ name, group, supported }` 항목 + active 타겟 + supported_count를 반환. `supported`는 현재 에디터 설치본의 `BuildPipeline.IsBuildTargetSupported` 결과 — 에이전트가 `build player` 시도 전 이 값으로 필터해야.

`build player`는 `BuildPipeline.BuildPlayer` 래퍼. `--target`은 alias (`win64` / `win32` / `mac` / `linux` / `android` / `ios` / `webgl`) + full enum 이름 (`StandaloneWindows64` 등) 모두 수용. `--output` 상대경로는 CLI cwd 기준 해석 (`test --output` / `screenshot --output_path`와 동일 관례), 부모 디렉토리는 자동 생성. `--scenes`는 콤마 구분 리스트 — 미명시면 Build Settings의 enabled scene 사용 (File > Build Settings 동작 동일). `--development`는 `BuildOptions.Development`. CLI는 `build player`에 무한 timeout 사용 — 글로벌 `--timeout`이 빌드 도중 발동하지 않음.

응답은 `BuildReport` summary 전체: `{ result, platform, output_path, total_size, total_errors, total_warnings, duration_sec, build_started_at, build_ended_at, steps_count, scenes_count }`. 실패/취소 빌드는 같은 payload를 가진 `ErrorResponse`로 — 호출자가 다른 shape 파싱 불필요.

`build addressables`는 reflection으로 `AddressableAssetSettings.BuildPlayerContent` 호출 (connector 자체는 `com.unity.addressables` 하드 의존 X). 패키지 미설치면 명확한 `UCI-011` 에러 + `udit package add com.unity.addressables` 안내. `--profile`은 임시로 `activeProfileId` 변경 후 빌드, 종료 시 이전 값 복원 (best-effort).

`build cancel`은 `BuildPipeline.CancelBuild` 호출. 진행 중 빌드 없으면 silent no-op (public API에 "빌드 진행 중?" 조회 없음) — 응답은 항상 success, 재실행 안전.

`--il2cpp` 와 `--config <name>` (`.udit.yaml`의 build preset) 은 이번 슬라이스 미포함 — v0.5.x patch에서 추가. IL2CPP는 현재 PlayerSettings에서 scripting backend 설정 (또는 `exec`로) 후 `build player` 호출.

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

Unity Test Framework로 EditMode와 PlayMode 테스트 실행 + 실행 없이 enumerate.

```bash
# EditMode 테스트 실행 (기본)
udit test run               # 또는 back-compat alias: `udit test`

# PlayMode 테스트 실행
udit test run --mode PlayMode

# 전체 테스트 이름으로 필터 (TestRunnerApi의 test path 기준 substring)
udit test run --filter MyNamespace.MyTests

# JUnit XML 리포트 동시 작성 (CI-friendly). 상대 경로는 CLI cwd 기준 해석
# (Unity 프로젝트 루트 아님); 실행 완료 후 작성됨.
udit test run --mode PlayMode --output test-results/playmode.xml

# 실행 없이 테스트 enumerate — 필터링 전 discovery에 유용
udit test list
udit test list --mode PlayMode
```

`test list`는 `{ mode, total, tests[] }`를 반환하며 각 leaf는 `full_name / name / class_name / type_info / run_state`. `full_name`을 후속 `test run`의 `--filter` 값으로 사용 가능.

JUnit XML 출력은 GitHub Actions / GitLab CI의 JUnit 파서와 그대로 호환. 실패 테스트는 Unity 실패 메시지 + 스택 트레이스가 `<failure>` 안에 들어가고, skipped/inconclusive는 `<skipped/>`로 매핑.

Unity Test Framework 패키지 필요. PlayMode 테스트는 도메인 리로드 트리거 — CLI가 결과를 자동 폴링.

### Tool 목록

```bash
# 모든 가용 tool (빌트인 + 프로젝트 커스텀)을 파라미터 스키마와 함께 표시
udit list
```

### 프로젝트 스캐폴드 (v0.6.1+)

`udit init`은 **Unity 프로젝트 루트**에 `.udit.yaml` 스캐폴드를 떨어뜨립니다. 경로 결정 순서 (먼저 맞는 것 사용):

1. `--output PATH` — 명시 override.
2. **연결된 Unity 인스턴스** — `udit status`가 보여주는 그 프로젝트. 여러 에디터가 떠있을 땐 `--port` / `--project` 전역 플래그로 선택.
3. 파일시스템 walk-up — `Assets/` + `ProjectSettings/` 둘 다 있는 디렉토리.
4. cwd fallback.

```bash
# 아무 디렉토리에서 — Unity가 init에게 어디에 만들지 알려줌
udit init                     # 탐지된 프로젝트 루트에 최소 스캐폴드
udit init --watch             # + 바로 쓰는 watch 섹션
udit --project MyGame init    # 여러 에디터 중 하나 선택
udit init --output ./my.yaml  # 명시 경로 (자동 탐지 건너뜀)
udit init --force --watch     # 기존 파일 덮어쓰기
```

`--watch` 옵션은 샘플 hook 두 개 (`compile_cs`, `reserialize_yaml`)를 포함 — `paths:` 리스트만 손보면 그대로 동작.

### Watch (v0.6.0+)

`udit watch`는 `.udit.yaml`에 정의된 후크를 파일 변경 시점에 자동 실행하는 장기 실행 워처입니다. LLM 호출 없이 에디터 루프 안에서 동작하는 로컬 CI 스타일 자동화.

```bash
# .udit.yaml이 프로젝트 루트(또는 상위 디렉토리)에 있어야 함
udit watch

# 실제 실행 없이 어떤 명령이 돌지만 출력
udit watch --no-exec

# NDJSON 이벤트 로그 출력
udit watch --json | tee watch.log
```

`.udit.yaml` 예시:

```yaml
watch:
  debounce: 300ms
  on_busy: queue          # queue(기본) 또는 ignore
  max_parallel: 4
  case_insensitive: true
  ignore:
    - "**/*.generated.cs"
  hooks:
    - name: compile
      paths: [Assets/**/*.cs, Packages/**/*.cs]
      run: refresh --compile
    - name: reserialize
      paths: [Assets/**/*.prefab, Assets/**/*.unity]
      run: reserialize $RELFILE
```

`run` 안의 변수:

| 토큰 | 의미 |
|---|---|
| `$FILE` | 절대 경로 (forward slash). **파일별** 개별 호출 발동. |
| `$RELFILE` | 프로젝트 루트 상대 경로 (예: `Assets/Scripts/Foo.cs`). 파일별. |
| `$FILES` / `$RELFILES` | argv에는 리터럴로 남음. 경로 목록은 환경변수 `UDIT_CHANGED_FILES` / `UDIT_CHANGED_RELFILES` (줄바꿈 구분)로 주입. 배치당 1회 호출. |
| `$EVENT` | 배치의 대표 이벤트 타입: `create` / `write` / `remove` / `rename`. |
| `$HOOK` | 후크 이름 (로깅용). |

하나의 `run` 안에 `$FILE` 계열과 `$FILES` 계열을 **함께 쓰면 config 로드 에러** — 파일별 or 배치 중 하나를 선택.

빌트인 Unity 무시 패턴 (사용자 `ignore:`에 추가로 병합, `defaults_ignore: false`로 끌 수 있음):
`Library/`, `Temp/`, `Logs/`, `MemoryCaptures/`, `UserSettings/`,
`Build/`, `Builds/`, `obj/`, `.git/`, `.vs/`, `.idea/`, `.vscode/`,
`*.csproj`, `*.sln`, `*~`, `*.tmp`, `.#*`.

**안전 장치**:
- **서킷 브레이커**: 후크가 10초 안에 10번 발동하면 자동으로 비활성화됩니다 (self-trigger 루프 방지). 후크가 쓰는 출력 경로를 `ignore:`에 추가하세요.
- **max_parallel**이 여러 후크가 동시에 매칭될 때 동시 실행 수를 제한합니다.

**시그널**:
- `Ctrl+C` 1회 — 실행 중 후크 완료 대기 후 정상 종료.
- `Ctrl+C` 2회 (2초 이내) — 강제 종료.

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
