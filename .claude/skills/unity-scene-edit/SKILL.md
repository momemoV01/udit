---
name: unity-scene-edit
description: .prefab/.unity/.asset/.mat YAML 파일을 텍스트로 편집한 후 Unity 시리얼라이저로 재저장하는 안전 절차. 편집→reserialize→diff 확인까지.
user-invocable: true
---

당신은 Unity 에셋 파일(.prefab, .unity, .asset, .mat)을 텍스트로 편집했습니다. Unity YAML 시리얼라이저는 엄격해서 **수동 편집 결과가 미묘하게 깨질 수 있습니다**. 아래 절차로 안전하게 마무리하세요.

## 전제

- 이미 대상 파일을 편집했거나, 편집하려는 참 
- 편집 대상은 `Assets/` 내부여야 함

## 1. 편집된 파일 확정

사용자에게 **구체적인 파일 경로**를 확인받으세요:

```bash
git status --short | grep -E "\.(prefab|unity|asset|mat)$"
```

파일 목록이 명확하면 진행. 불명확하면 사용자에게 물어보세요.

## 2. 사전 유효성 점검

각 파일이 Unity가 로드 가능한 형식인지 간단 체크:

- 파일 크기 > 0
- 첫 줄이 `%YAML 1.1` 또는 `%TAG` 로 시작
- `fileID: 0` 같은 의심스러운 값 검색

```bash
head -2 <path>
grep -n "fileID: 0" <path> || echo "OK"
```

위험 신호 있으면 **reserialize 전에 사용자 확인**.

## 3. Reserialize 실행

파일별로:

```bash
udit reserialize <path> --json
```

여러 파일이면 한 번에:

```bash
udit reserialize Assets/Scenes/Main.unity Assets/Prefabs/Player.prefab --json
```

## 4. Unity가 어떻게 다시 썼는지 diff 확인

```bash
git diff --stat -- Assets/
git diff -- Assets/Scenes/Main.unity | head -50
```

**기대하지 않은 변경**이 보이면:

- GUID 무작위 생성 → 참조 깨짐 가능
- 컴포넌트 순서 변경 → 대부분 무해
- 값 정규화 (`1.0000001` → `1.0`) → 무해
- 전체 섹션 삭제/추가 → **반드시 사용자 확인**

## 5. 콘솔 에러 확인

reserialize 후 Unity가 경고를 남겼을 수 있음:

```bash
udit console --type error,warning --lines 10 --stacktrace user --json
```

- 깨끗하면 **"✅ reserialize OK"** 로 종료.
- 경고/에러 있으면 사용자에게 diff + 로그를 함께 보고.

## 주의사항

- `Assets/` 밖의 경로는 **reserialize 되지 않음** — Unity가 무시.
- 씬이 **현재 로드된 상태**면 reserialize가 메모리 기준으로 덮어쓸 수 있음. 수동 편집 후엔 `udit scene reload` 먼저.
- Addressables 참조가 있는 프리팹은 GUID 변경 금지. reserialize 결과 GUID 바뀌면 **복구 필수**.
- 큰 변경(> 100줄 diff)은 **사용자 승인** 없이 커밋하지 말 것.
