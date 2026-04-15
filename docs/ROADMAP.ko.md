# udit Roadmap

[English](ROADMAP.md) | [한국어](ROADMAP.ko.md)

> Living plan. 버전/우선순위는 실사용 피드백에 따라 조정됨.
> Last updated: 2026-04-15 (v0.4.2 release — Phase 3 complete)

## Vision

**udit은 AI 에이전트가 단독으로 Unity 게임을 개발·빌드·배포할 수 있는 CLI 도구다.**

현재(v0.1.0)는 상위 프로젝트 [unity-cli](https://github.com/youngwoocho02/unity-cli)의 얇은 HTTP 브리지만 포함한다. 이는 "실행 레이어"로는 훌륭하지만, 에이전트가 진짜로 원하는 **관찰 레이어 / 변경 레이어 / 자동화 레이어 / 스트리밍 레이어**가 빠져 있다. 이 문서는 그 갭을 메우는 계획이다.

종착점(v1.0.0)에서 달성하고 싶은 것:

- 에이전트가 **`exec` 없이** 90% 이상의 Unity 작업을 할 수 있다
- 인디 개발자가 **빌드부터 배포까지** CLI 한 줄로 자동화한다
- 에이전트가 파일 변경을 **실시간으로 감지**하고 Unity 상태에 반응한다
- 단 한 명의 유지보수자가 **5년간** 관리 가능한 복잡도에 머무른다

## Design Principles (불변)

아래 원칙은 **모든 단계에서 깨지지 않는다**. 이게 udit을 가볍게 유지하는 골격이다.

1. **Stateless HTTP 유지** — 상주 서버나 세션 상태 추가 금지. 매 요청이 독립적.
2. **Go CLI는 얇게** — 복잡한 로직은 전부 C# 쪽에. Go는 포워딩 + 폴링 + 파싱만.
3. **모든 출력은 에이전트 파싱 가능** — `--json` 기본 지원, 정형 에러 코드.
4. **기존 API 파괴 금지** — v1.0까지 새 파라미터/커맨드만 추가. 기존 것 제거·변경 금지.
5. **10k LOC 상한** — 한 사람이 머릿속에 담을 수 있는 크기. 초과하면 리팩터 또는 외부 분리.

## Timeline at a Glance

| 단계 | 버전 | 테마 | 상태 | 핵심 가치 |
|---|---|---|---|---|
| 0 | `v0.1.0` | Initial Fork | ✅ **Done** | unity-cli 리브랜드 기반선 |
| 1 | `v0.2.0` | **Foundation** | ✅ **Done** | 버그 + JSON + 설정 파일 |
| 2a | `v0.3.0` | **Observe — Scene & GO** | ✅ **Done** | Stable ID + `scene` + `go` |
| 2b | `v0.3.1` | **Observe — Component & Asset** | ✅ **Done** | `component` + `asset` |
| 3a | `v0.4.0` | **Mutate — GO & Component** | ✅ **Done** | `go create/destroy/move/rename/setactive` + `component add/remove/set/copy` + Undo + `--dry-run` |
| 3b | `v0.4.1` | **Mutate — ObjectRef + Prefab + Asset** | ✅ **Done** | `component set` ObjectReference 쓰기, `prefab instantiate/unpack/apply/find-instances`, `asset create/move/delete/label` |
| 3c | `v0.4.2` | **Mutate — Transactions** | ✅ **Done** | `tx begin/commit/rollback/status` — `Undo.CollapseUndoOperations` / `RevertAllDownToGroup` |
| 4 | `v0.5.0` | **Automate** | 📋 Planned (다음) | 빌드/패키지 관리 |
| 5 | `v0.6.0` | **Stream** | 📋 Planned | watch + log tail |
| 6 | `v1.0.0` | **Polish & Freeze** | 📋 Planned | 테스트/문서/API 동결 |

Phase 2는 원래 단일 릴리스였으나 실제 작업하며 **scene + go** 블록이 에이전트 체감 가치 라인(`exec` 의존도 급감)을 이미 넘는 것을 확인해 2a/2b로 분할. 2a를 v0.3.0, 2b를 v0.3.1로 짧게 끊어 출시 — 같은 4월 15일에 일어났지만 분리 이유는 (i) v0.3.0 직후 Public 전환 미결로 발견된 문제와 분리, (ii) v0.3.1에서 추가된 commands가 의미 있는 단위로 묶임.

Phase 3도 같은 이유로 3a/3b/3c 분할. **GO + Component mutation (3a, v0.4.0)** 만으로 에이전트가 씬을 새로 구성할 수 있는 기본 loop(`create GO → addComponent → setField`)가 완성. 피드백 받으며 ObjectReference 쓰기 + Prefab + Asset mutation (3b, v0.4.1)을 같은 날 patch로 추가, transactions (3c)는 cross-cutting이라 다른 것 다 들어간 후 마지막으로.

---

## Phase 0: v0.1.0 — Initial Fork (DONE)

**완료일**: 2026-04-14

unity-cli v0.3.9의 **리브랜드 복사본**. 기능 변경 없음. 자세한 변경사항은 [CHANGELOG.md](../CHANGELOG.md) 참고.

핵심 산출물:
- Go 모듈: `github.com/momemoV01/udit`
- Unity 패키지: `com.momemov01.udit-connector`
- C# 네임스페이스: `UditConnector`, 어트리뷰트 `[UditTool]`
- 기본 포트: `8590` (unity-cli와 공존)
- GitHub Release v0.1.0 (5개 플랫폼 바이너리)

---

## Phase 1: v0.2.0 — Foundation

**목표**: 후속 기능의 **공통 인프라** 구축. 이 단계를 건너뛰면 이후 모든 단계가 기술부채를 누적한다.

### 1.1 크리티컬 버그 픽스 (from unity-cli 분석)

| 버그 | 파일 | 수정 |
|---|---|---|
| `ExecuteCsharp` 타임아웃 시 프로세스 미종료 | `udit-connector/Editor/Tools/ExecuteCsharp.cs:169` | `proc.Kill()` 추가 |
| `EditorScreenshot` 차원 무제한 | `udit-connector/Editor/Tools/EditorScreenshot.cs:36-38` | 8192×8192 상한 |
| `CommandRouter` 컴파일/업데이트 중 명령 수용 | `udit-connector/Editor/CommandRouter.cs` | `isCompiling`/`isUpdating` 가드 |
| `buildParams` 불린 강제 변환 | `cmd/root.go:221-229` | 파라미터 화이트리스트 |

### 1.2 글로벌 `--json` 플래그 (최중요)

**현재**: 응답이 JSON일 때도 있고 raw text일 때도 있음 — 파싱 불가.

**변경**: `--json` 지정 시 stdout은 **100% 정형 JSON**.
```json
{
  "success": true,
  "command": "console",
  "data": { ... },
  "message": "Read 5 entries",
  "error_code": null,
  "unity": {
    "port": 8590,
    "project": "E:/Games/MyGame",
    "state": "ready",
    "version": "6000.1.0f1"
  }
}
```

구현:
- `cmd/root.go`의 `printResponse()`에 JSON 분기
- 모든 help 텍스트에 `--json` 섹션 추가
- 에러도 JSON으로 (exit code는 유지)

### 1.3 에러 코드 레지스트리

에이전트가 **재시도 여부를 구조적으로 판단**하기 위함.

```
UCI-001  NoUnityRunning           → 사용자가 Unity 시작 필요
UCI-002  ConnectionRefused        → 재시도 대상
UCI-003  CommandTimeout           → 재시도 대상
UCI-010  UnknownCommand           → 재시도 불가 (명령 오타)
UCI-011  InvalidParams            → 재시도 불가
UCI-020  UnityBusy (compiling)    → 2-3초 후 재시도
UCI-021  UnityBusy (updating)     → 2-3초 후 재시도
UCI-030  ExecCompileError         → 재시도 불가 (사용자 코드 문제)
UCI-031  ExecRuntimeError         → 재시도 불가
UCI-040  AssetNotFound            → 재시도 불가
UCI-041  SceneNotFound            → 재시도 불가
```

구현: C# 쪽 `ErrorResponse`에 `ErrorCode` 필드 추가, Go 쪽 에러 매핑.

### 1.4 설정 파일 `.udit.yaml`

프로젝트 루트에 두면 자동 로드.

```yaml
# .udit.yaml 예시
default_port: 8590
default_timeout_ms: 120000

exec:
  usings: [Unity.Entities, Unity.Mathematics, MyGame.Core]

watch:
  paths: [Assets/Scripts]
  debounce_ms: 300
  on_change: ["refresh --compile", "console --type error"]

build:
  targets:
    win64:
      output: builds/win64/MyGame.exe
      scenes: [Assets/Scenes/Main.unity]
    android:
      output: builds/android/MyGame.apk
      il2cpp: true
```

구현: Go 쪽 YAML 로드 (`gopkg.in/yaml.v3`), cwd 부터 상위로 search.

### 1.5 Shell 자동완성

```bash
udit completion bash | sudo tee /etc/bash_completion.d/udit
udit completion powershell > $PROFILE.CurrentUserAllHosts
```

### Phase 1 성공 기준

- [x] 기존 모든 테스트 통과
- [x] `--json` 포함 모든 응답이 schema-valid
- [x] 설정 파일 로드 실패해도 기본값으로 동작
- [x] 에러 코드 문서화 (`docs/ERROR_CODES.md`)

**완료 commit 체인** (2026-04-14, v0.2.0):
- 1.1 `e0b9f5e` 크리티컬 버그 4건 + `273afc0` Unity 6000 deprecation
- 1.2 `894d958` 글로벌 `--json` + 메타 envelope
- 1.3 `657911b` 에러 코드 레지스트리 + `bffd175` ERROR_CODES.md
- 1.4 `4d8758d` `.udit.yaml` 설정 파일 (cwd → home walk-up)
- 1.5 `5711aab` shell completion (bash/zsh/powershell/fish)

---

## Phase 2a: v0.3.0 — Observe (Scene & GameObject)

**완료일**: 2026-04-15

**목표**: 에이전트가 **`exec` 없이** 씬과 GameObject를 조회할 수 있게. 에이전트 체감 가치 라인(`exec` 의존도 급감)을 넘기는 최소 세트.

### 2.1 `scene` — 씬 관리 ✅

```bash
udit scene list                      # 프로젝트 내 모든 씬 (Assets + Packages, path 정렬, 빌드 인덱스)
udit scene active                    # 현재 활성 씬 (path/guid/dirty/root_count/build_index)
udit scene open <path> [--force]     # 씬 전환 (--force로 dirty 가드 우회)
udit scene save                      # 열린 dirty 씬만 저장, 저장된 경로 리포트
udit scene reload [--force]          # 활성 씬 재로드 (dirty면 --force 필요)
udit scene tree [--depth N]          # 하이어라키 JSON 트리 + stable ID 부여
```

### 2.2 `go` — GameObject 쿼리 ✅

```bash
udit go find [--name PAT] [--tag T] [--component C] [--limit N --offset M] [--active-only]
udit go inspect go:a1b2c3d4          # 모든 컴포넌트 + serialized 값 덤프
udit go path go:a1b2c3d4             # 하이어라키 경로 문자열
```

**Stable ID 설계**: Unity `InstanceID`는 세션 스코프 → 재시작 시 바뀜. `GlobalObjectId`를 SHA1 해시해 `go:{8자 hex}` 포맷으로 축약. 충돌 시 10/12/14/16자로 확장. 결정적이므로 같은 GO는 세션 넘어도 동일 ID.

**Pagination**: `go find`는 `--limit N --offset M` 지원. 결과는 하이어라키 경로 기준 정렬되어 페이지 간 결정적.

**UCI-042 GameObjectNotFound**: 만료/알 수 없는 stable ID → 명확한 에러 코드 + "`go find`로 재스캔" 가이드.

### 구현 구조

```
udit-connector/Editor/Tools/
  ManageScene.cs          (manage_scene: list/active/open/save/reload/tree)
  ManageGameObject.cs     (manage_game_object: find/inspect/path)
  Common/
    StableIdRegistry.cs   (GlobalObjectId → go:hash 매핑 + 역매핑)
    SerializedInspect.cs  (Component → JSON, Transform 특별 처리, 모든 SerializedPropertyType)
```

### Phase 2a 성공 기준

- [x] 에이전트가 `exec` 없이 씬/GO "읽기" 작업을 완수 (scene + go 전체 커버)
- [x] Stable ID로 커맨드 체이닝 가능 (`go find` → 결과 id → `go inspect`)
- [x] Pagination 지원 (`--limit N --offset M`)
- [x] 결정적 재현: 동일 GO는 동일 ID (세션 간 불변)
- [ ] 대규모 씬(GameObject 10,000+)에서 응답 < 2초 *(실측 프로젝트 미확보, 후속 검증)*

**완료 commit 체인** (2026-04-14 ~ 2026-04-15, v0.3.0):
- `e8d7b62` feat(connector): StableIdRegistry 인프라
- `570b178` feat(scene): list/active/open/save/reload
- `8840341` feat(scene): tree (StableIdRegistry 첫 소비자)
- `6a2d929` feat(go): find/inspect/path + pagination + UCI-042

---

## Phase 2b: v0.3.1 — Observe (Component & Asset)

**완료일**: 2026-04-15

**목표**: Observe 완성. `go inspect` / `scene tree`가 덤프 위주라면 2b는 **field-level zoom-in (`component get`)** 과 **프로젝트 자산 그래프 (`asset *`)** 추가.

### 2.4 `component` — 컴포넌트 쿼리 ✅

```bash
udit component list go:a1b2c3d4                  # 붙은 컴포넌트 요약
udit component get go:a1b2c3d4 Transform         # 한 컴포넌트 전체 덤프
udit component get go:a1b2c3d4 Transform position           # 단일 필드
udit component get go:a1b2c3d4 Transform position.z         # 점표기 중첩
udit component get go:a1b2c3d4 BoxCollider --index 1        # 같은 타입 여러 개
udit component schema Camera                     # 타입 스키마 (live 인스턴스 필요)
```

`SerializedInspect`가 이미 `go inspect`에서 쓰이는 컨버터라 `component get`은 그 결과를 JObject로 받아 dotted path traversal 하는 얇은 래퍼. 결과적으로 같은 vocabulary가 chain 전체에 일관 적용 — `go inspect` 응답에 보이는 필드 이름을 그대로 `component get`에 넘겨도 됨.

`schema` v1은 **씬에 live 인스턴스가 있어야** 함 (`AddComponent`가 RequireComponent 체인 등 부작용 있어 spawn 회피). reflection-only fallback은 후속.

새 에러 코드 **UCI-043 ComponentNotFound** — GO에 해당 타입 없음 / `--index` 범위 초과 / `schema` 인스턴스 없음 세 케이스 모두 이 코드. 메시지에 실제 붙은 타입 또는 실제 카운트 포함해서 에이전트가 자가 교정 가능.

### 2.3 `asset` — 에셋 쿼리 ✅

```bash
udit asset find [--type Prefab] [--label boss] [--name "*Enemy*"] [--folder F]
udit asset inspect Assets/Materials/Player.mat   # 타입별 details 블록
udit asset dependencies Assets/Scenes/Main.unity [--recursive]
udit asset references Assets/Prefabs/Enemy.prefab [--limit N]
udit asset guid <path>
udit asset path <guid>
```

`inspect`의 타입별 detail 핸들러: **Texture2D** (크기/포맷/필터/wrap/mip), **Material** (쉐이더 + ShaderUtil 기반 프로퍼티 enumeration), **AudioClip** (length/freq/channels/load), **Prefab root** (root_components/child_count), **ScriptableObject** (full SerializedInspect 덤프), **TextAsset** (length + 500자 preview + truncated). 다른 타입은 common header(`{path, guid, name, type, labels}`) + `details:null`.

`references`는 Unity가 역인덱스 없어 **전체 스캔** 필요. 응답에 `scan_ms` + `scanned_assets` 노출하므로 에이전트가 비용 인지 가능. `--limit` 기본 100, 최대 1000.

**SerializedInspect.ObjectToJson** API 추가 — 기존 `ComponentToObject`는 Component만 받지만 ScriptableObject/Material 등 일반 `UnityEngine.Object` 덤프 필요. 내부 `WalkProperties(UnityEngine.Object)` 헬퍼로 둘 다 공유.

**UCI-040 AssetNotFound** 활성화 (v0.3.0에서 reserved 였음). 모든 unknown path/GUID 케이스를 단일 코드로.

### 구현 구조 (전체 Phase 2)

```
udit-connector/Editor/Tools/
  ManageScene.cs          (manage_scene: list/active/open/save/reload/tree)
  ManageGameObject.cs     (manage_game_object: find/inspect/path)
  ManageComponent.cs      (manage_component: list/get/schema)
  ManageAsset.cs          (manage_asset: find/inspect/dependencies/references/guid/path)
  Common/
    StableIdRegistry.cs   (GlobalObjectId → go:hash 매핑 + 역매핑)
    SerializedInspect.cs  (Component/Object → JSON, Transform 특별 처리)
```

### Phase 2b 성공 기준

- [x] `component get`으로 임의 필드 dotted-path 조회
- [x] `component schema`로 타입의 SerializedProperty 메타 (live 인스턴스 기반)
- [x] `asset find` 모든 필터 조합 (type/label/name/folder + paginate)
- [x] `asset dependencies` (direct + recursive)
- [x] `asset references` 전체 스캔 + `scan_ms` 비용 노출
- [x] `asset inspect` 6개 타입별 detail handler
- [x] UCI-040 AssetNotFound + UCI-043 ComponentNotFound 활성화 + 양국어 문서

**완료 commit 체인** (2026-04-15, v0.3.1):
- `df2b7fa` feat(component): list/get/schema + UCI-043
- `194ddde` feat(asset): find/inspect/dependencies/references/guid/path

---

## Phase 3a: v0.4.0 — Mutate (GameObject & Component)

**완료일**: 2026-04-15

**목표**: 에이전트가 씬을 **쓸 수** 있게. 기본 loop `create GO → addComponent → setField` 완성 — "AI 게임 개발"의 분기점.

### 3.1 GameObject 생성/삭제/수정 ✅

```bash
udit go create --name "Boss" [--parent go:XXX] [--pos x,y,z] [--dry-run]
udit go destroy go:XXX [--dry-run]
udit go move go:XXX [--parent go:YYY] [--dry-run]      # YYY 생략 시 root로
udit go rename go:XXX <newname> [--dry-run]
udit go setactive go:XXX --active true|false [--dry-run]
```

**Undo 통합**: 각 mutation마다 `Undo.IncrementCurrentGroup()` + `Undo.SetCurrentGroupName(...)` + 전용 Undo API. live-test 중 "Undo가 여러 op을 묶어 취소" 버그를 발견하고 명시적 group-increment로 해결. 결과: Editor의 Edit → Undo 메뉴에 단계별 descriptive 이름 표시되어 Ctrl+Z가 한 번에 한 논리 연산씩 되돌림.

**Cycle guard**: `go move`는 reparent 후보의 자손 체인을 scan해 자기 자신/자손 아래로 이동하는 케이스를 UCI-011로 사전 거부.

### 3.2 컴포넌트 조작 ✅

```bash
udit component add go:XXX --type Rigidbody [--dry-run]
udit component remove go:XXX Rigidbody [--index N] [--dry-run]
udit component set go:XXX Type field value [--index N] [--dry-run]
udit component copy go:SRC Type go:DST [--index N] [--dry-run]
```

**값 파서** (set): SerializedPropertyType 별 전용 parse. Integer/LayerMask/ArraySize/Character, Boolean (true/false/1/0/yes/no/on/off), Float, String, Vector2/3/4/Quaternion (comma-separated), Color (`"r,g,b[,a]"` 또는 `"#RRGGBB[AA]"`), Enum (display name 또는 value index). ObjectReference/Curve/Gradient/ManagedReference는 이 버전에서 **읽기 전용**.

**Transform virtual fields**: `component set`에서도 `position`/`local_position`/`rotation_euler`/`local_rotation_euler`/`local_scale`이 `component get`과 동일한 이름으로 작동 (Transform API 직접 호출).

**안전장치**:
- 모든 mutation Unity Undo 통합
- Transform remove → UCI-011 "use `go destroy` instead"
- Transform add → UCI-011 "already has one"
- 없는 field → UCI-011 + **전체 유효 필드 이름 목록** (에이전트 자가 교정)
- 타입 mismatch → UCI-011 + 기대 타입 이름 명시

### 3.5 Dry-run (cross-cutting) ✅

모든 mutation이 `--dry-run` 지원. 응답 shape은 실제 실행 시와 동일 하지만 side-effect 없음.

### Phase 3a 성공 기준

- [x] 에이전트가 `exec` 없이 **씬 구성 기본 loop** 완수 (create GO, addComponent, setField)
- [x] 모든 변경이 Unity Undo로 단계별 되돌림 가능
- [x] Dry-run 응답 shape = 실제 실행 응답 shape
- [x] Unity 6 deprecation 정리 (FindObjectsByType, ShaderUtil, CopySerialized)

**완료 commit 체인** (2026-04-15, v0.4.0):
- `7451c77` feat(go): GameObject mutation
- `da9a282` feat(component): component mutation

---

## Phase 3b: v0.4.1 — Mutate (ObjectRef + Prefab + Asset)

**완료일**: 2026-04-15

**목표**: 3a의 기본 loop를 프로젝트 구조 관리까지 확장. v0.4.0의 "쓰기 반쪽" 상태(ObjectReference read-only) 해소 + Prefab/Asset 자체 mutation.

### 3.2+ `component set` ObjectReference 쓰기 ✅

```bash
udit component set go:X SpriteRenderer m_Sprite Assets/Sprites/Player.png
udit component set go:X Material m_MainTex Assets/Textures/wall.jpg
udit component set go:X AudioSource m_audioClip Assets/Sounds/hit.wav
udit component set go:X Camera m_TargetTexture null
```

**Sub-asset auto-pick**: `.png` 경로가 `Texture2D` main + `Sprite` sub-asset으로 import됐을 때, `component set`이 target 필드 타입에 assign 가능한 **첫 sub-asset**을 자동 선택.

**타입 체크**: `SerializedProperty.type`이 `"PPtr<$Sprite>"` 형태 → wrapper strip → reflection으로 타입 resolve → `IsAssignableFrom` 확인. 실패 시 UCI-011 + 기대 타입 + 실제 발견된 타입 목록.

**Clear**: `null`, `none`, `""` 모두 참조 clear.

**씬 레퍼런스**: `go:XXXX`는 SerializedProperty의 별도 payload라 이 버전은 read-only + UCI-011 "use exec for now".

### 3.3 Prefab 인스턴싱 ✅

```bash
udit prefab instantiate <path> [--parent go:P] [--pos x,y,z] [--dry-run]
udit prefab unpack go:X [--mode root|completely] [--dry-run]
udit prefab apply go:X [--dry-run]
udit prefab find-instances <path>
```

새 `[UditTool]` 클래스 `ManagePrefab` → `manage_prefab`. `PrefabUtility.InstantiatePrefab`으로 에셋 링크 유지. `apply`/`unpack`은 nested GO 받아도 outermost root로 자동 resolve. `find-instances`는 전체 씬 스캔.

**Stable ID 변경 주의**: unpack 시 prefab 링크가 identity의 일부라서 `GlobalObjectId`가 바뀌고 udit `go:` id도 바뀜. unpack 응답이 새 id를 반환. 옛 id는 UCI-042.

### 3.4 에셋 생성/이동/삭제/라벨 ✅

```bash
udit asset create --type <TypeName> --path <path>   # ScriptableObject 또는 sentinel "Folder"
udit asset move <src> <dst>
udit asset delete <path> [--permanent]              # 기본 trash
udit asset label <add|remove|list|set|clear> <path> [labels...]
```

**No Unity Undo**: AssetDatabase 연산(Create/Move/Delete/SetLabels)은 씬 Undo에 참여 안 함. Ctrl+Z 불가. 안전장치: (i) 모든 mutation `--dry-run`, (ii) `delete` 기본 `MoveAssetToTrash` (OS 휴지통에서 복구), (iii) `delete --permanent --dry-run`은 full-project 스캔으로 `referenced_by: N` 보고.

**Path auto-resolve**: `--path`가 `/`로 끝나거나 기존 폴더면 `<TypeName>.asset` 자동 추가.

### Phase 3b 성공 기준

- [x] `prefab instantiate`로 생성된 인스턴스가 stable ID로 추적 가능
- [x] `component set`이 ObjectReference 쓰기 지원
- [x] Sub-asset auto-pick
- [x] `asset move`가 GUID 유지
- [x] `asset delete` 기본 trash (복구 가능), `--permanent` 시 referenced_by 보고
- [x] 모든 mutation dry-run

**완료 commit 체인** (2026-04-15, v0.4.1):
- `87ef711` feat(component): ObjectReference write
- `3959995` feat(prefab): instantiate/unpack/apply/find-instances
- `46d6d1f` feat(asset): create/move/delete/label

---

## Phase 3c: v0.4.2 — Mutate (Transactions) ✅

**완료일**: 2026-04-15

**목표**: 복합 변경의 원자성. 각 mutation이 독립 Undo group이라 복합 변경 취소에 N번 Ctrl+Z 필요했던 문제 해결.

```bash
udit tx begin [--name "..."]
udit go create --name Boss
udit component add go:X --type Rigidbody
udit component set go:X Rigidbody m_Mass 5.5
udit tx commit [--name "..."]         # 한 번의 Ctrl+Z로 3개 변경 전부 reverse
udit tx rollback                      # 또는 begin 이후 전부 되감기
udit tx status                        # 활성 tx 여부 확인
```

**구현**: 새 `[UditTool]` 클래스 `ManageTransaction`. begin 시 `Undo.GetCurrentGroup()` 캡처 (static 필드), commit 시 `Undo.CollapseUndoOperations(savedGroup)` → 모든 sub-group이 하나로 합쳐짐. rollback 시 `Undo.RevertAllDownToGroup(savedGroup)`으로 Undo 스택을 pre-tx 상태로 되감음.

**Unity-native 접근 덕분에 state가 극히 최소**: Connector 쪽은 `{group index, name, started timestamp}` 3개 필드만 static, 실제 변경 내역은 전부 Unity Undo 스택에.

**제약**:
- **인스턴스당 1개 tx만** — Unity Undo 스택이 전역이라 nesting 불가. 활성 tx 중 begin → UCI-011 + 기존 tx 이름/age.
- **도메인 리로드가 핸들 폐기** — static 상태는 스크립트 재컴파일로 사라짐. 부분 mutation은 Undo 스택에 남음. `tx status`로 확인 후 re-begin.
- **AssetDatabase 연산 미참여** — `asset create/move/delete/label`은 디스크에 즉시 쓰기라 씬 Undo 그룹에 못 묶임. 트랜잭션 내 실행은 되나 rollback으로 되돌릴 수 없음.

### Phase 3c 성공 기준

- [x] 트랜잭션 begin/commit/rollback/status 구현
- [x] 트랜잭션 rollback 시 상태 완벽 복구 (live-test: scene count 5 → 3 원상 복구)
- [x] 트랜잭션 내 mutation이 단일 Undo 엔트리로 합쳐짐 (live-test: 3 mutation → 1 PerformUndo로 전부 reverse)
- [x] Double-begin / 활성 없는 commit·rollback 방어 (UCI-011 + helpful message)

**완료 commit** (2026-04-15, v0.4.2):
- `10ae76c` feat(tx): add transactions (begin/commit/rollback/status)

---

## Phase 4: v0.5.0 — Automate

**목표**: CI/배포 자동화. 인디에게 "빌드 버튼 자동화"는 시간 절약 1순위.

### 4.1 `build` — 플레이어 빌드

```bash
udit build player --target win64 --output builds/win64/
udit build player --config production        # .udit.yaml의 build.targets.production 사용
udit build player --scenes Main,Level1 --target android --il2cpp
udit build targets                            # 사용 가능한 타겟 목록
udit build addressables [--profile Default]
udit build cancel                             # 진행 중 빌드 취소
```

**빌드 진행도**: SSE 스트리밍 (Phase 5와 맞물림).
```
[build] Compiling scripts...          ████████░░  80%
[build] Writing player...             ███░░░░░░░  30%
```

### 4.2 `package` — UPM 패키지 관리

```bash
udit package list
udit package add com.unity.cinemachine
udit package add com.unity.cinemachine@2.9.7
udit package add https://github.com/dbrizov/NaughtyAttributes.git
udit package remove com.unity.cinemachine
udit package info com.unity.cinemachine
udit package search cinemachine
udit package resolve                          # manifest.json 재해결
```

### 4.3 `test` 확장

```bash
udit test run --mode PlayMode --filter "Level.*" --output junit.xml
udit test list [--mode EditMode]              # 실행 전 테스트 목록
udit test coverage                            # Code Coverage 패키지 연동
```

### 4.4 `project` — 프로젝트 메타

```bash
udit project info                             # 버전, 패키지, 씬, LOC
udit project validate                         # missing references, 누락 에셋 스캔
udit project preflight                        # 빌드 전 헬스체크 (컴파일 에러, missing refs, 에셋 무결성)
```

### Phase 4 성공 기준

- [ ] 인디가 **CI/GitHub Actions에서 udit만으로** 빌드-테스트-배포 완수
- [ ] `build player` 진행도 실시간 리포트
- [ ] `project validate`가 missing reference 100% 탐지
- [ ] `test run` JUnit XML 출력으로 CI 통합

---

## Phase 5: v0.6.0 — Stream

**목표**: **긴 시간 단위 반응형 워크플로** — watch 모드 + 로그 스트리밍.

### 5.1 `watch` — 파일 변경 감시 + 자동화

```bash
udit watch                                    # .udit.yaml 설정대로
udit watch --path Assets/Scripts --on-change "refresh --compile"
udit watch --path Assets --on-change "reserialize $FILE"
```

내부 플로우:
```
fsnotify (Go) → 디바운스 300ms → udit refresh --compile
  → console --type error → (에러 있으면 stderr 출력)
  → 성공 시 "OK"
```

Ctrl+C 시 진행 중 커맨드 완료 후 정상 종료.

### 5.2 `log tail -f` — 콘솔 로그 스트리밍

```bash
udit log tail --follow [--type error,warning]
udit log tail --follow --since 5m             # 최근 5분부터
udit log tail --filter "Boss"                 # 정규식 필터
```

**아키텍처**: udit-connector에 SSE 엔드포인트 추가.
```
GET /logs/stream?types=error,warning  (Accept: text/event-stream)
→ event: log
  data: {"timestamp": 123, "type": "Error", "message": "...", "stack": "..."}
```

도메인 리로드 중 자동 재연결.

### 5.3 `run` — 스크립트 러너 (선택)

```bash
udit run scripts/bootstrap.sh
```

설정 파일에 정의된 복합 워크플로 실행. `make` 느낌.

### Phase 5 성공 기준

- [ ] `watch` 중 1000회 변경에도 메모리/CPU 안정
- [ ] `log tail`에서 도메인 리로드 중 **끊김 없이** 재연결
- [ ] SSE 스트림이 네트워크 일시 단절에 자동 복구

---

## Phase 6: v1.0.0 — Polish & Freeze

**목표**: 프로덕션 신뢰도 확보. **API 동결** + **장기 유지보수 가능한 상태**.

### 6.1 테스트 커버리지

- C# 유닛 테스트 50% 이상 (현재 거의 0)
- E2E 테스트 스위트 (Unity 자동 기동 → 시나리오 → 검증)
- `test-harness` 저장소 별도 분리 (CI Unity 라이선스 풀)

### 6.2 문서화

- **Tool Reference** 자동 생성 (`udit list --json` → `docs/TOOLS.md`)
- **Cookbook** — 실전 시나리오 20개 (씬 생성부터 빌드까지)
- **Claude Code 통합 가이드** — `.claude/` 템플릿 제공
- **Migration from unity-cli** 문서

### 6.3 API 동결

- v1.0 이후 **breaking change 금지**
- 새 기능은 새 파라미터/커맨드로만
- 5년 유지보수 commitment

### 6.4 에이전트 친화 기능

```bash
udit context                                  # 프로젝트 맥락 요약 (에이전트용)
# → { "unity_version", "packages", "scenes", "scripts_count", "assemblies" }

udit explain <topic>                          # 짧은 개념 설명
# → "Addressables: Unity's asset management system..."
```

### Phase 6 성공 기준

- [ ] C# 테스트 커버리지 ≥ 50%
- [ ] Cookbook 시나리오 ≥ 20개
- [ ] 최소 3개 인디 프로젝트에 프로덕션 사용 확인
- [ ] 1년간 breaking change 0건

---

## Cross-Cutting Architecture

이 항목들은 **여러 단계에 걸쳐** 적용되는 공통 인프라.

### C-1. 버전화된 API

모든 응답에 `"api_version": "1"` 필드. 클라이언트가 호환성 판단.

### C-2. Pagination

큰 응답(`go find`, `asset find`)은 자동 페이지네이션.
```json
{ "data": [...100 items], "next_cursor": "abc", "total": 5234 }
```

### C-3. 공통 ID 네이밍

- `go:{8자 hash}` — GameObject (GlobalObjectId 해시)
- `asset:{guid}` — Asset (Unity GUID)
- `scene:{guid}` — Scene (Unity GUID)

### C-4. `--output` 옵션

```bash
udit scene tree --output yaml      # JSON 대신 YAML
udit go inspect go:... --output csv
```

### C-5. 텔레메트리 (opt-in)

익명 사용량 수집 — `--telemetry on`으로 명시 활성화 시만. **기본은 꺼짐**.

---

## Success Metrics (KPIs)

측정 가능한 지표로 진척도 추적.

### 개발 효율성
| 지표 | 현재 | 목표 (v1.0) |
|---|---|---|
| 에이전트가 `exec` 없이 완수 가능한 작업 비율 | ~40% | 90% |
| 인디 프로젝트 1회 빌드 자동화 시간 | 수동 5분 | udit 30초 |

### 안정성
| 지표 | 목표 |
|---|---|
| P95 응답 시간 (소규모 명령) | < 500ms |
| P95 응답 시간 (큰 쿼리) | < 2초 |
| 24시간 세션 메모리 증가 | < 100MB |
| Unity 재시작 없이 연속 명령 수 | 10,000+ |

### 생태계
| 지표 | 목표 |
|---|---|
| 커스텀 `[UditTool]` 작성 예시 저장소 | 최소 10개 |
| Claude Code 스킬 템플릿 | 최소 5개 |

---

## Risk Register

| 리스크 | 영향 | 대응 |
|---|---|---|
| **Unity API breaking change** (6000 → 6001) | 중 | `#if UNITY_6000_1_OR_NEWER` 조건부 컴파일 |
| **도메인 리로드 중 명령 유실** | 상 | Heartbeat 감지 + CLI 자동 재시도 |
| **대규모 씬 성능 저하** | 중 | Pagination + Lazy loading |
| **코드 복잡도 폭증** | 상 | 10k LOC 상한 엄수, 기능별 독립 어셈블리 |
| **커스텀 툴과 충돌** | 하 | `[UditTool(Namespace="myteam")]` 네임스페이스 지원 |
| **에이전트가 파괴적 커맨드 남용** | 상 | `--dry-run` 기본 + 권한 선언 메타데이터 |
| **Private → Public 전환 타이밍** | 중 | v0.2.0 후 결정 (현재 Private) |

---

## Contributing

현재는 **solo maintainer** (momemo / `momemoV01`). v1.0 전까지 외부 기여 제한.

### 로컬 개발 흐름

```bash
git clone https://github.com/momemoV01/udit
cd udit
go build -o udit.exe .
go test ./...
```

상세 흐름은 [CLAUDE.md](../CLAUDE.md)의 "Verification" / "릴리스 플로우" 섹션 참고.

### Claude Code와 작업 시

프로젝트 루트 `CLAUDE.md`에 udit 개발 컨벤션이 있음. 에이전트가 자동 참고.

### 이슈 트래킹

현재는 GitHub Issues로 단순 관리. v1.0 이후 labels/milestones로 세분화.

---

## Upstream Relationship

udit은 [unity-cli](https://github.com/youngwoocho02/unity-cli) by DevBookOfArray의 fork. 자세한 attribution은 [NOTICE.md](../NOTICE.md) 참고.

### 업스트림 정책

1. **원본에 중요 버그픽스가 나오면 cherry-pick 검토** — 특히 Phase 1 스코프 버그들은 upstream에도 보고할 가치 있음.
2. **범용 개선은 upstream PR 우선** — udit 고유 기능이 아닌 것(예: Windows HOME 테스트 픽스)은 upstream에 기여.
3. **큰 방향성 분기는 udit 고유로** — 에이전트 중심 설계 결정(JSON 우선, 에러 코드, 설정 파일)은 upstream과 독립 진행.

### 업스트림 체크 커맨드

```bash
# 원본을 upstream으로 추가 (최초 1회)
git remote add upstream https://github.com/youngwoocho02/unity-cli

# 주기적으로 최근 변경 확인
git fetch upstream
git log upstream/master --oneline --since="2 weeks ago"
```

---

## Decision Log

프로젝트 중 내린 중요 결정을 기록. 나중에 "왜 이렇게 했지?" 고민 줄이기.

| 날짜 | 결정 | 이유 |
|---|---|---|
| 2026-04-14 | Fork 이름 `udit` | 4자, 타이핑 최적, "unity edit" 암시, 산스크리트 의미 (떠오른) |
| 2026-04-14 | 기본 포트 8590 | unity-cli (8090)과 공존 가능 |
| 2026-04-14 | Private 시작 | 초기 리네임 혼란 비공개, 안정화 후 Public 검토 |
| 2026-04-14 | `master` → `main` | 현대 표준 (원본은 `master`) |
| 2026-04-14 | README.ko.md 삭제 | 1인 유지보수, 단일 영어 README로 통일 |
| 2026-04-14 | v0.1.0 reset | fork 정체성 명확화 (upstream v0.3.9과 분리) |
| 2026-04-14 | 바이너리 설치 위치 `%LOCALAPPDATA%\udit\udit.exe` | Windows 관례 + User PATH 등록 편리. 단 Claude Desktop(MSIX) 샌드박스 이슈 있어 **외부 PowerShell에서 빌드 필수** — CLAUDE.md 참고 |
| 2026-04-14 | Unity 프로젝트에 `file:` 로컬 경로로 Connector 설치 | Private repo라 UPM Git URL 대신. 원본 unity-cli와 포트/네임스페이스 분리로 공존 검증 성공 |
| 2026-04-14 | `.udit.yaml` walk-up 검색 (cwd → $HOME 직전) | git-style 친숙. `$HOME` 이상은 안 올라가서 stray config가 다른 프로젝트에 새지 않음 |
| 2026-04-14 | Shell completion 정적 (cobra X) | 의존성 없이 stdlib `flag`로 충분. 4개 셸 스크립트가 string 상수 → 단순 + 즉시 검증 가능. 커스텀 `[UditTool]`은 런타임에 `udit list`로 발견 |
| 2026-04-14 | YAML 의존성 `gopkg.in/yaml.v3` | 사실상 표준, BSD-3, 단일 모듈, 활발한 유지보수 |
| 2026-04-14 | 한글 문서 정책 도입 | 사용자/에이전트가 읽을 가능성 있는 문서는 영어 + 한글 짝으로(`README.md` ↔ `README.ko.md`). CHANGELOG/NOTICE/LICENSE/CLAUDE.md는 영어 단일 |
| 2026-04-15 | Stable ID 포맷 `go:{8 hex}` (SHA1 of GlobalObjectId) | InstanceID는 세션 스코프라 부적합, GlobalObjectId는 80자로 CLI에 넘기기 과함. SHA1 해시 8자로 압축해 에이전트 친화적 + 결정적 (세션 간 동일) + 32비트 충돌 확률 낮음 (1만 GO에서 ~1/86). 충돌 시 10/12/14/16자로 확장하는 escalation 내장. 구현: `StableIdRegistry.cs` |
| 2026-04-15 | `Manage<Namespace>` + `action` 파라미터 패턴 고수 | `ManageEditor`/`ManageProfiler` 선례 유지. `ManageScene` + `ManageGameObject`도 같은 패턴. ROADMAP 예시의 `SceneTools.cs`/`GameObjectTools.cs` 분리는 fine-grained action별 UditTool 발행을 암시했으나 실제로는 단일 클래스 + switch dispatch가 코드 경제성 훨씬 좋음 |
| 2026-04-15 | Phase 2 분할 (2a = scene+go / 2b = asset+component) | 원래 단일 v0.3.0으로 잡았으나, 2a만으로 에이전트 체감 가치 라인(exec 의존도 급감)을 이미 넘는 것을 실제 구현·검증 중 확인. 2a를 v0.3.0으로 즉시 출시하고 asset/component는 피드백 받으며 v0.3.x 증분으로 추가하는 게 유지보수 건강에도 맞음 |
| 2026-04-15 | Dirty-scene 가드 (`--force` 요구) | `scene open`/`scene reload`가 dirty 씬에서 `EditorSceneManager.OpenScene`을 호출하면 Unity가 조용히 변경을 폐기함. 에이전트가 실수로 작업을 날리는 리스크. 기본 refuse + `--force`로 명시적 discard. Save 후 호출하거나 force로 명시 둘 중 선택하게 강제 |
| 2026-04-15 | `SerializedInspect` 유틸에서 Transform만 특별 처리 | `SerializedObject`는 `m_LocalPosition` 등만 노출하지만 에이전트는 world 좌표도 필요. 컴포넌트 전체 reflection은 overkill — Transform만 `t.position`/`t.eulerAngles` 직접 읽어 반환. 나머지 컴포넌트는 visible SerializedProperty walk. Enum `{value, name}`, Color `{r,g,b,a}`, ObjectRef `{type, name, path, guid}`, 배열은 20 clip + `{count, elements, truncated}` |
| 2026-04-15 | `component get` field path를 JObject traversal로 구현 | C# 쪽에 별도 path resolver를 두는 대신 SerializedInspect.ComponentToObject 결과를 JObject로 변환 후 점표기로 navigate. 장점: (1) 에이전트가 보는 필드 이름이 `go inspect`와 100% 일치 (단일 vocabulary), (2) 중첩 객체와 배열 인덱스를 같은 구문으로 (`m_Cameras.elements.0`), (3) Transform의 가상 필드(`position`, `local_position`)도 자동 지원 |
| 2026-04-15 | `component schema`는 live 인스턴스 probe 방식 (v1) | `AddComponent` spawn 시 RequireComponent 체인 + 일부 컴포넌트의 internal flag로 인한 add 거부 등 부작용 큼. 임시 GO를 만들고 destroy해도 하이라키 변경 noise 발생. 차선책으로 씬에 이미 있는 인스턴스를 `FindAnyObjectByType`로 찾아 SerializedObject 메타만 추출. 인스턴스 없으면 명확한 UCI-043 메시지로 안내. reflection-only fallback은 후속 슬라이스 |
| 2026-04-15 | `asset references`는 전체 스캔 + `scan_ms` 노출 | Unity는 reverse dependency 인덱스를 제공하지 않아 정직한 구현은 모든 에셋의 GetDependencies를 도는 O(n) scan. 이를 숨기지 않고 응답에 `scanned_assets` + `scan_ms` 필드를 포함시켜 에이전트가 비용을 인지하고 `--limit` 사용을 결정할 수 있게 함. 12k 에셋 프로젝트에서 ~1.8초 측정 — 큰 프로젝트에서는 캐싱 또는 인덱싱 추가 검토 |
| 2026-04-15 | `asset inspect`에 타입별 detail handler 둠 (6 types) | SerializedObject 한 가지로 모두 처리하면 Material의 ShaderUtil 메타 / Texture2D의 format/mip / AudioClip의 freq 등 타입 고유 정보를 놓침. switch에 핸들러 6개(Texture2D/Material/AudioClip/Prefab/ScriptableObject/TextAsset) 두고 그 외는 details:null + common header. 새 타입 추가 시 핸들러 한 개만 추가 |
| 2026-04-15 | Phase 2 분할 결정 후 v0.3.0 + v0.3.1을 같은 날 출시 | 원래 2a 출시 후 사용 피드백 받고 2b 진행 계획이었으나, 2a 직후 dog-food 단계에서 (i) Public 전환 미결로 `udit update` 못 쓰는 마찰 발견, (ii) component/asset도 SerializedInspect 재활용으로 빠르게 마무리 가능함 확인. Day-1 patch로 2b 묶어 출시 |
| 2026-04-15 | 모든 mutation은 `Undo.IncrementCurrentGroup()` 명시적 호출 | Unity는 기본적으로 editor tick마다 Undo group을 자동 증분하지만, udit처럼 HTTP로 연속 호출되는 경우 multiple 명령이 같은 tick에 묶여 한 group에 들어갈 수 있음. `create + destroy` 한 쌍이 같은 group이면 단일 PerformUndo가 둘 다 취소해 net 효과 0. slice 6 live-test 중 발견 후 모든 mutation 시작에 `Undo.IncrementCurrentGroup() + SetCurrentGroupName(...)` 삽입해 해결. 부산물로 Editor의 Edit → Undo 메뉴에 `"udit go destroy 'Boss'"` 식 설명적 레이블 표시 |
| 2026-04-15 | `--dry-run`을 Phase 3 전체에 cross-cutting으로 | 별도 feature로 붙이는 대신 **모든 mutation action 안에 uniformly 통합**. Go CLI가 `--dry-run` → `dry_run: true` 매핑, 각 C# action이 첫 side-effect 전에 분기. 응답 shape은 실제 실행 시와 동일 필드를 갖되 mutation만 skip → 에이전트가 preview/commit을 한 shape으로 처리 가능 |
| 2026-04-15 | `component set` 값 파서는 SerializedPropertyType로 분기 | JSON-typed value 대신 **모든 값을 문자열로** 받아 target field의 `SerializedPropertyType`에 따라 파싱. 장점: (i) CLI argv가 자연스럽게 문자열 → 추가 escape 없음, (ii) 같은 `"0,5,0"`이 Vector2/3/4 타겟에 따라 다르게 해석됨, (iii) Color `"#FF8800"` 같은 관례적 포맷 허용 |
| 2026-04-15 | Transform `position`/`local_position` 등을 `component set`에서 virtual field로 | `component get`에서 Transform 특별 처리로 world 좌표(`position`)를 노출했음. set에서도 **같은 이름 지원** 해야 read/write vocabulary 일관성 유지. 구현: `IsTransformVirtualField(name)` → Transform API 직접 호출 |
| 2026-04-15 | `component set` v1에서 ObjectReference/Curve/Gradient/ManagedReference는 read-only | set이 단순 파싱 이상 필요 — ObjectReference는 asset 경로 resolve + type check, Curve는 keyframe parse, ManagedReference는 runtime 타입 resolve. MVP는 primitives + Vector/Color/Enum까지만 cover하고 나머지는 명확한 UCI-011 + "read-only in this version" 메시지. 실사용 feedback 받은 뒤 v0.4.x에서 증분 |
| 2026-04-15 | Phase 3도 3a/3b 분할 — v0.4.0은 GO + Component만 | Phase 2와 동일 근거. **GO + Component mutation (3a)** 만으로 에이전트가 씬 구성의 기본 loop 실행 가능 = 에이전트 체감 가치 라인. Prefab/Asset mutation (3b)은 프로젝트 구조 관리에 가까워 우선순위 낮음 |
| 2026-04-15 | Unity 6 deprecation 정리 (FindObjectsByType/ShaderUtil/CopySerialized) | slice 7 live-test 중 발견. `EditorUtility.CopySerialized` 반환 타입 bool → void로 바뀌어 컴파일 에러 CS0023. `FindObjectsByType<T>(FindObjectsInactive, FindObjectsSortMode)`는 단일 인자 오버로드로. `ShaderUtil.GetPropertyCount/Name/Type`은 `Shader` 인스턴스 메서드 + `UnityEngine.Rendering.ShaderPropertyType`(TexEnv → Texture) |
| 2026-04-15 | `component set` ObjectReference 파싱: 에셋 경로 + sub-asset auto-pick | `SerializedProperty.type`이 `"PPtr<$Sprite>"` 형태로 기대 타입 노출. `LoadAllAssetsAtPath`로 main + sub-assets 순회해 **타입 호환 첫 에셋** 자동 선택. 에이전트가 `.png` 경로만 주면 `SpriteRenderer.m_Sprite`엔 Sprite, `RawImage.texture`엔 Texture2D가 자동 할당됨 |
| 2026-04-15 | 씬 오브젝트 참조(`go:XXX`)는 `component set`에서 rejected | ObjectReference는 asset PPtr payload, 씬 참조는 SceneObjectReference로 다른 payload. 같은 write path로 처리 시 silently broken. 명시적 UCI-011 + "use exec for now" 메시지로 막고 씬 참조 쓰기는 후속 슬라이스로 미룸 |
| 2026-04-15 | Prefab 연산은 `ManagePrefab` 별도 도구로 | `manage_game_object`에 얹을 수도 있었으나 prefab은 asset ↔ scene instance 관계를 다루는 독립 concept이고 서브커맨드 모두 prefab 고유 기능. 별도 도구가 코드 구성 + 에이전트 vocabulary 둘 다 깔끔 |
| 2026-04-15 | `prefab unpack` 후 stable ID가 바뀌는 것을 문서화 (숨기지 않음) | `GlobalObjectId`는 prefab 연결 정보를 identity에 포함 — unpack하면 id 자체가 변경됨. Stateless HTTP 원칙 + 정확성 위해 unpack 응답에 새 id 반환하고 README에 명시적으로 문서화 |
| 2026-04-15 | AssetDatabase 연산은 Unity Undo 없음 — 명시적 caveat + trash 기본 | `AssetDatabase.CreateAsset/MoveAsset/DeleteAsset/SetLabels`는 Undo에 참여 안 함 (Unity API의 본질). help + README에 명시. 안전장치: (i) 모든 mutation `--dry-run`, (ii) `delete` 기본 `MoveAssetToTrash` (OS 휴지통 복구), (iii) `delete --permanent --dry-run`은 full-project 스캔으로 `referenced_by: N` 보고 |
| 2026-04-15 | `asset create --type Folder` sentinel 방식 | 폴더는 `AssetDatabase.CreateFolder(parent, child)`로 별도 API — signature 다름. 별도 명령 대신 `create` 안에서 `--type Folder`를 sentinel로. 장점: 하나의 create surface로 통일, 에이전트가 `--type X --path Y` 패턴 하나로 외우면 됨 |
| 2026-04-15 | Phase 3 분할 세분화 (3a/3b/3c) — v0.4.0/v0.4.1 day-1 patch | transactions만 cross-cutting이라 묶기 불편해서 3b에 ObjectReference set + prefab + asset mutation을 담아 v0.4.1로 cut, transactions는 3c로 분리. Phase 2 때 v0.3.0 → v0.3.1 같은 날 릴리스 패턴 그대로 |
| 2026-04-15 | 트랜잭션은 Unity-native API 3개로 (`IncrementCurrentGroup` + `CollapseUndoOperations` + `RevertAllDownToGroup`) | 대안은 udit이 "begin 이후 실행된 명령 목록"을 자체 추적하고 rollback 시 역순 재실행. 단점: (i) Stateless HTTP 원칙 위반 큼, (ii) 비가역적 API(asset create/move 등)는 역재실행 불가, (iii) Unity Undo와 별도 추적이라 Ctrl+Z와 udit rollback이 다르게 동작. Unity Undo를 신뢰하면 state가 `{group, name, started}` 3개, commit 후 Ctrl+Z 한 번 = udit rollback 1회 = 대칭, Unity가 지원 못 하는 건 udit도 안 함이 일관. AssetDatabase 미참여는 docs에 명시 |
| 2026-04-15 | 트랜잭션 state는 static 필드 (도메인 리로드 시 자동 폐기) | 명시적 cleanup hook 없이 리로드 시 static wipe되는 Unity 특성 활용. 장점: 핸들이 stale 상태로 남지 않음. 단점: 부분 mutation이 Undo 스택에 남되 tx 핸들은 사라져 "묶기 미완성" 상태 — `tx status`가 no-active 반환하면 agent 인지 가능 |
| 2026-04-15 | Phase 4 분할 — 4a(project) + 4b(test) 를 v0.4.3 interim, 4c(build) + 4d(package) 를 v0.5.0 | Phase 2/3 day-1 patch 패턴 계승. project + test 둘만으로도 JUnit XML → CI 통합 경로가 열림 = 체감 가치 라인. `build`는 가장 큰 덩어리(진행도 스트리밍/다중 타겟/IL2CPP), `package`는 중간 크기. build/package 기다리느라 test/project 출시가 밀리는 걸 피하고, v0.5.0은 Phase 4 전체 완성으로 깨끗이 cut. v0.5.0 regression 범위도 두 슬라이스로 제한 |
| 2026-04-15 | `--output` / `--output_path` 상대경로는 **CLI cwd** 기준 (Unity 프로젝트 루트 X) | 초판은 C# 쪽에서 `Application.dataPath`의 부모(프로젝트 루트) 기준으로 resolve했음. POSIX CLI 관행 위반 — `udit <cmd> --output foo.xml`은 shell 현재 위치에 생기리라 기대. CI/GitHub Actions도 `$GITHUB_WORKSPACE` 기준 기대. 수정: Go CLI가 `filepath.Abs`로 상대→절대 변환 후 HTTP에 실음. C# 쪽은 절대경로 그대로 사용. Direct HTTP 호출자용 project-root fallback은 유지. 헬퍼 `absolutizePath` / `absolutizePathParam` (`cmd/paths.go`) 을 `test --output` (전용 핸들러)과 `screenshot --output_path` (default passthrough) 양쪽에 동일 적용. 미래 path-like 플래그도 동일 지점에서 처리 |
| 2026-04-15 | udit-connector .meta GUID를 unity-cli와 영구 분리 (Unity 자동 할당값 채택) | fork 시 .meta GUID 분리를 빼먹음. 두 connector를 같은 Unity 프로젝트에 동시 설치하면 GUID 중복 → Unity가 udit-connector 측 27개 GUID 자동 재할당 후 file: source에 write-back. 결과: 메인 디렉토리 working tree가 매번 dirty (27개 .meta modified). v0.4.3까지 commit 안 하고 방치. 해결: Unity가 이미 검증한 새 GUID를 그대로 정식 채택. risk: [UditTool] 클래스는 static, asmdef 외부 dep 없음, package.json.meta GUID는 UPM이 무시 → CLI-only 사용자에 무영향. CLI 변경 없어 새 tag 안 만들고 main push만. Connector 0.6.1 → 0.6.2 patch |

---

## References

- [unity-cli 원본 분석](https://github.com/youngwoocho02/unity-cli)
- [CLAUDE.md](../CLAUDE.md) — 개발 컨벤션
- [CHANGELOG.md](../CHANGELOG.md) — 버전별 변경사항
- [NOTICE.md](../NOTICE.md) — Attribution
- [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
- [Semantic Versioning](https://semver.org/)

---

## Next Actions

구체적으로 **지금 뭐부터 할지** 리마인더. 완료된 항목은 `[x]`로 남겨두어 진행을 추적.

- [x] 실제 Unity 프로젝트에 Connector 설치 + 연결 검증 (port 8590, 2026-04-14)
- [x] `.claude/skills/unity-verify/SKILL.md` 작성 (4개 스킬 포함)
- [x] Phase 1 전체 (1.1 ~ 1.5)
- [x] v0.2.0 태그 push + Release 검증 (2026-04-14)
- [x] v0.2.1 patch: sentinel markers + Node 20 actions 완전 제거 + 실전 검증
- [x] Phase 2a 착수 — StableIdRegistry + scene + go (2026-04-14 ~ 2026-04-15)
- [x] **v0.3.0 태그 push + Release 검증** (2026-04-15)
- [x] Phase 2b 착수 — component + asset (2026-04-15)
- [x] **v0.3.1 태그 push + Release 검증** (2026-04-15)
- [x] Phase 3a 착수 — GO + Component mutation + Undo + dry-run (2026-04-15)
- [x] **v0.4.0 태그 push + Release 검증** (2026-04-15)
- [x] Phase 3b 착수 — ObjectReference set + Prefab + Asset mutation (2026-04-15)
- [x] **v0.4.1 태그 push + Release 검증** (2026-04-15)
- [x] Phase 3c 착수 — Transactions (`tx begin/commit/rollback/status`) (2026-04-15)
- [x] **v0.4.2 태그 push + Release 검증** (2026-04-15)
- [x] Phase 4a 착수 — `project info/validate/preflight` (2026-04-15)
- [x] Phase 4b 착수 — `test list` + `test run --output junit.xml` + CLI-cwd path semantics fix (2026-04-15)
- [x] **v0.4.3 태그 push + Release 검증** (2026-04-15)
- [ ] Public 전환 여부 결정 (Unity Connector 설치 테스트 + `udit update` 정상화 위해)
- [ ] **Phase 4c/4d 착수** — `build player/targets/addressables` + `package add/remove/list/info/resolve` (v0.5.0)
- [ ] `component set`에서 Curve/Gradient/ManagedReference + 씬 오브젝트 참조 쓰기 지원 (v0.4.x 증분)
- [ ] 대규모 씬 성능 측정 (10k+ GO 프로젝트 확보 후 `scene tree`/`go find`/`asset references` 응답 시간 실측)
