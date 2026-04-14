---
name: unity-perf-check
description: Unity Profiler로 성능 병목 분석. 프로파일러 활성화 → 시나리오 실행 요청 → hierarchy 덤프 → 상위 병목 리포트.
user-invocable: true
---

당신은 사용자가 지정한 **시스템/씬/상황**의 성능 병목을 찾아야 합니다. 아래 절차를 따르세요.

## 1. 분석 대상 확정

사용자에게 물어볼 것:
- **대상**: 어느 시스템/씬/동작? (예: "보스전 씬", "ECS SimulationSystem", "UI 업데이트")
- **측정 범위**: 특정 프레임? N 프레임 평균? (기본: 최근 30 프레임 평균)
- **임계값**: 몇 ms 이상만 볼지? (기본: 0.5ms)

## 2. 프로파일러 활성화

```bash
udit profiler enable --json
```

상태 확인:
```bash
udit profiler status --json
```

## 3. 사용자에게 시나리오 실행 요청

다음 내용을 사용자에게 명확히 안내:

> 프로파일러가 활성화됐습니다. 지금 Unity에서 **<대상 시나리오>** 를 실행하세요.
> 예: play 모드 진입 → 보스룸 입장 → 10초간 전투 → 정지
> 완료되면 알려주세요. 그 사이 저는 대기합니다.

사용자 신호 받기 전까지 **다음 단계로 진행하지 마세요**.

## 4. 데이터 수집

```bash
# 전체 hierarchy (30 프레임 평균, 0.5ms 이상, self 시간 기준 정렬)
udit profiler hierarchy --frames 30 --min 0.5 --sort self --max 20 --json
```

특정 시스템이 궁금하면 root 지정:
```bash
udit profiler hierarchy --root SimulationSystem --depth 3 --frames 30 --json
```

## 5. 분석 리포트

수집한 데이터로 다음 형식의 리포트 작성:

```
## 성능 분석: <대상>

### 상위 5 병목 (self time 기준)
1. <이름> — <ms> ms (<calls>회 호출)
   - 원인 추정: ...
   - 개선 방향: ...

2. ...

### 주목할 패턴
- GC allocation 급증 → ...
- Physics 업데이트 비중 과도 → ...
- UI rebuild 반복 → ...

### 추천 다음 단계
1. <구체적 액션> (예: "Update()에서 매 프레임 GetComponent 호출 제거")
2. ...
```

## 6. 프로파일러 정리

분석 끝나면:
```bash
udit profiler disable --json
udit profiler clear --json    # 선택 — 다음 세션 위해
```

## 주의사항

- **최소 10 프레임 이상** 수집해야 의미 있음. 단일 프레임은 노이즈.
- Editor 모드와 Build 모드 성능은 다름. "실측은 빌드에서"라는 점 리포트에 명시.
- `exec`로 프로파일러 API 직접 호출하지 말 것 — `udit profiler` 표준 경로만.
- 비교 분석 요청 시 (전/후): 각각 30 프레임씩 수집한 뒤 **delta 표** 만들어 보고.
