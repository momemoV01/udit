# 커스텀 Tool 작성

> 설치 및 빠른 시작은 [README](../README.ko.md)를 참고하세요. 이 페이지는 프로젝트 전용 tool 확장 방법을 다룹니다.

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
