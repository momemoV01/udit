# udit

[English](README.md) | [한국어](README.ko.md)

> Unity 에디터를, 커맨드 라인에서. AI 에이전트를 위해 만들었지만, 무엇이든 사용할 수 있습니다.
>
> Udit (उदित) — 산스크리트어로 *떠오른*. [DevBookOfArray](https://github.com/youngwoocho02)의 [unity-cli](https://github.com/youngwoocho02/unity-cli) fork로, 에이전트 중심 게임 개발 워크플로를 위해 확장했습니다.

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/momemoV01/udit?sort=semver)](https://github.com/momemoV01/udit/releases/latest)
[![Go](https://img.shields.io/github/go-mod/go-version/momemoV01/udit)](go.mod)
[![CI](https://github.com/momemoV01/udit/actions/workflows/ci.yml/badge.svg)](https://github.com/momemoV01/udit/actions/workflows/ci.yml)

**실행할 서버도, 작성할 설정도, 관리할 프로세스도 없습니다. 그냥 명령어를 치면 됩니다.**

## 설치

```bash
# Linux / macOS
curl -fsSL https://raw.githubusercontent.com/momemoV01/udit/main/install.sh | sh

# Windows (PowerShell)
irm https://raw.githubusercontent.com/momemoV01/udit/main/install.ps1 | iex

# Go install (모든 플랫폼)
go install github.com/momemoV01/udit@latest
```

업데이트: `udit update` &nbsp;|&nbsp; 확인만: `udit update --check`

## Unity 설정

**Package Manager → Add package from git URL:**

```
https://github.com/momemoV01/udit.git?path=udit-connector
```

Connector는 자동으로 시작됩니다. 설정이 필요 없습니다.

> **팁:** **Edit → Preferences → General → Interaction Mode**을 **No Throttling**으로 설정하면 Unity가 백그라운드에 있을 때도 명령이 즉시 실행됩니다.

## 빠른 시작

```bash
udit status                              # Unity 연결 확인
udit editor play --wait                  # 플레이 모드 진입
udit exec "return Application.dataPath;" # Unity 안에서 C# 실행
udit console --type error                # 에러 로그 읽기
udit test run --output results.xml       # EditMode 테스트 실행
udit go find --name "Player*"            # GameObject 검색
udit build player --config production    # 플레이어 빌드
```

## 작동 원리

```
터미널                                 Unity Editor
──────                                 ────────────
$ udit editor play --wait
    │
    ├─ ~/.udit/instances/*.json 스캔
    │  → 포트 8590에서 Unity 발견
    │
    ├─ POST http://127.0.0.1:8590/command
    │  { "command": "manage_editor",
    │    "params": { "action": "play" }}
    │                                      │
    │                                  HttpServer 수신
    │                                  CommandRouter 디스패치
    │                                  ManageEditor.HandleCommand()
    │                                      │
    ├─ JSON 응답 수신  ←───────────────────┘
    │  { "success": true,
    │    "message": "Entered play mode." }
    │
    └─ 결과 출력
```

Unity Connector는 `localhost:8590`에 HTTP 서버를 열고, CLI가 연결할 수 있도록 heartbeat 파일을 작성하며, 메인 스레드의 `[UditTool]` 핸들러로 명령을 라우팅합니다. 도메인 리로드에도 살아남습니다. 외부 의존성 없음.

## 명령어

| 카테고리 | 명령어 | 설명 |
|----------|--------|------|
| **에디터** | `editor play\|stop\|pause\|refresh` | 플레이 모드 및 에셋 리프레시 제어 |
| **씬** | `scene list\|active\|open\|save\|reload\|tree` | 씬 조회 및 전환 |
| **게임오브젝트** | `go find\|inspect\|path\|create\|destroy\|move\|rename\|setactive` | GO 조회 및 변경 |
| **컴포넌트** | `component list\|get\|schema\|add\|remove\|set\|copy` | 컴포넌트 필드 읽기/쓰기 |
| **에셋** | `asset find\|inspect\|dependencies\|references\|guid\|path\|create\|move\|delete\|label` | 프로젝트 에셋 조회 및 변경 |
| **프리팹** | `prefab instantiate\|unpack\|apply\|find-instances` | 프리팹 인스턴스 작업 |
| **트랜잭션** | `tx begin\|commit\|rollback\|status` | 변경을 단일 Undo로 그룹화 |
| **프로젝트** | `project info\|validate\|preflight` | 프로젝트 상태 및 메타데이터 |
| **패키지** | `package list\|add\|remove\|info\|search\|resolve` | Unity Package Manager |
| **빌드** | `build player\|targets\|addressables\|cancel` | 스탠드얼론 플레이어 빌드 |
| **콘솔** | `console` | 콘솔 로그 읽기/지우기 |
| **실행** | `exec "<C# 코드>"` | Unity 안에서 임의 C# 실행 |
| **테스트** | `test run\|list` | EditMode/PlayMode 테스트 실행 |
| **프로파일러** | `profiler hierarchy\|enable\|disable\|status\|clear` | 성능 프로파일링 |
| **자동화** | `log tail`, `watch`, `run <task>` | 스트리밍, 파일 감시, 태스크 러너 |
| **설정** | `init`, `config show\|validate\|path\|edit` | 프로젝트 설정 |
| **유틸리티** | `status`, `update`, `doctor`, `list`, `completion` | 시스템 및 진단 |

**예시와 플래그가 포함된 전체 레퍼런스:** [`docs/COMMANDS.ko.md`](./docs/COMMANDS.ko.md)

모든 명령에 `--help` 사용 가능: `udit editor --help`, `udit asset --help` 등.

### 글로벌 옵션

| 플래그 | 설명 | 기본값 |
|--------|------|--------|
| `--port <N>` | Unity 인스턴스 포트 지정 | 자동 |
| `--project <path>` | 프로젝트 경로로 Unity 인스턴스 선택 | 최신 |
| `--timeout <ms>` | HTTP 요청 타임아웃 | 120000 |
| `--json` | 머신 판독 가능 JSON envelope 출력 | 끔 |

## 커스텀 Tool 작성

`[UditTool]` 속성이 있는 정적 클래스를 Editor 어셈블리에 만들면 Connector가 자동으로 발견합니다:

```csharp
[UditTool(Name = "spawn", Description = "적 소환")]
public static class SpawnEnemy
{
    public static object HandleCommand(JObject parameters)
    {
        var p = new ToolParams(parameters);
        float x = p.GetFloat("x", 0);
        var prefab = Resources.Load<GameObject>(p.Get("prefab", "Enemy"));
        var go = Object.Instantiate(prefab, new Vector3(x, 0, 0), Quaternion.identity);
        return new SuccessResponse("소환 완료", new { name = go.name });
    }
}
```

```bash
udit spawn --x 5 --prefab Goblin
```

**속성 레퍼런스가 포함된 전체 가이드:** [`docs/CUSTOM_TOOLS.ko.md`](./docs/CUSTOM_TOOLS.ko.md)

## 성능

10k GameObject 씬 (~10,762 에셋)에서 측정. 모든 쿼리가 1초 이내 응답.

| 쿼리 | ms (평균) | 비고 |
|---|---:|---|
| `scene tree` | ~550 | 전체 계층 |
| `go find --name` | ~760 | 10k 매치 |
| `go inspect` | ~450 | 단일 GO 덤프 |
| `asset references` | ~960 | 전체 프로젝트 스캔 |
| `asset dependencies` | ~440 | 직접 의존성 |

상세 내용은 [ROADMAP Decision Log](./docs/ROADMAP.ko.md#decision-log) 참고.

## 보안 & 신뢰 모델

udit은 **에디터가 열린 신뢰된 로컬 사용자**를 전제합니다.

- **전송:** 로컬호스트 전용 (`127.0.0.1`), 브라우저 `Origin` 헤더 거부
- **코드 실행은 기능:** `exec`, `menu`, `run`은 에디터 전체 권한 보유 — 신뢰할 수 없는 입력을 파이프하지 마세요
- **업데이트:** GitHub Releases에서 HTTPS + SHA256 체크섬 검증
- **보호하지 않는 것:** 공유 머신, 공급망 침해, 악의적인 `.udit.yaml` (Makefile처럼 취급)

취약점 보고: [GitHub Security Advisory](https://github.com/momemoV01/udit/security/advisories/new)

## MCP와 비교

| | MCP | udit |
|---|-----|-----------|
| **설치** | Python + uv + FastMCP + 설정 JSON | 단일 바이너리 |
| **의존성** | Python 런타임, WebSocket 릴레이 | 없음 |
| **프로토콜** | JSON-RPC 2.0 over stdio + WebSocket | 직접 HTTP POST |
| **세팅** | MCP 설정 생성, AI 도구 재시작 | Unity 패키지 추가, 끝 |
| **재연결** | 도메인 리로드 위한 복잡한 재연결 로직 | 요청마다 stateless |
| **호환성** | MCP 호환 클라이언트만 | shell 있는 모든 것 |

## API 안정성

**v1.0.0**부터 udit은 [Semantic Versioning](https://semver.org)을 엄격히 따릅니다.

| 대상 | v1.0부터 안정 | 예시 |
|---|---|---|
| CLI 명령 및 하위 명령 이름 | 예 | `udit console`, `udit go find` |
| CLI 플래그 이름 | 예 | `--json`, `--port`, `--limit` |
| JSON envelope 형태 | 예 | `{ success, message, data, error_code }` |
| 에러 코드 (UCI-xxx) | 예 — 코드는 재사용하지 않음 | `UCI-001`, `UCI-042` |
| 기존 응답 필드 이름 | 예 | `data.matches`, `data.count` |

하위 호환 추가는 minor 버전. 호환성 깨는 변경은 major 버전에서만 (먼저 deprecated 표시).

## Unity 호환성

| Unity 버전 | 상태 | 비고 |
|---|---|---|
| 6000.4.x (Unity 6.1) | 테스트됨 | 6000.4.2f1에서 벤치마크 |
| 6000.0.x -- 6000.3.x (Unity 6.0) | 최선 노력 | 동일 API 표면; 회귀 테스트 미수행 |
| 2022.3 LTS | 미테스트 | 호환 가능성 있음; 테스트 결과 PR 환영 |
| < 2022 | 미지원 | 내부 API 차이 |

## 문서

| 문서 | 설명 |
|------|------|
| [`docs/COMMANDS.ko.md`](./docs/COMMANDS.ko.md) | 예시가 포함된 전체 명령어 레퍼런스 |
| [`docs/CUSTOM_TOOLS.ko.md`](./docs/CUSTOM_TOOLS.ko.md) | 프로젝트 전용 tool 작성 가이드 |
| [`docs/COOKBOOK.ko.md`](./docs/COOKBOOK.ko.md) | 실전 워크플로우 레시피 |
| [`docs/ERROR_CODES.ko.md`](./docs/ERROR_CODES.ko.md) | UCI 에러 코드 레지스트리 |
| [`docs/ROADMAP.ko.md`](./docs/ROADMAP.ko.md) | 개발 로드맵 및 결정 로그 |

## 감사의 말

udit은 **DevBookOfArray** (youngwoocho02)가 만든 **[unity-cli](https://github.com/youngwoocho02/unity-cli)** 의 fork입니다. 원본의 아키텍처, HTTP 브리지, reflection 기반 tool 발견, heartbeat 설계, 도메인 리로드 처리가 이 프로젝트의 완전한 기반을 이룹니다. 자세한 attribution은 [NOTICE.md](./NOTICE.md) 참고.

[![Original](https://img.shields.io/badge/Original-unity--cli-success?logo=github)](https://github.com/youngwoocho02/unity-cli)
[![YouTube](https://img.shields.io/badge/YouTube-DevBookOfArray-red?logo=youtube&logoColor=white)](https://www.youtube.com/@DevBookOfArray)

## 메인테이너

**momemo** 가 유지보수합니다 ([![GitHub](https://img.shields.io/badge/GitHub-momemoV01-181717?logo=github)](https://github.com/momemoV01))

## 라이선스

MIT — [LICENSE](./LICENSE) 참고. 원본과 fork 양쪽의 저작권 표기는 모든 복사본에 보존되어야 합니다.
