---
name: unity-verify
description: Unity 스크립트 변경 후 안전 검증 절차 — refresh --compile → console 에러 체크 → test 실행. 컴파일/런타임 에러 있으면 즉시 보고하고 중단.
user-invocable: true
---

당신은 udit으로 Unity Editor를 제어 중입니다. 스크립트가 수정된 후 아래 절차를 **순서대로** 수행하세요. 각 단계가 실패하면 **다음 단계로 진행하지 말고** 즉시 사용자에게 원인과 수정 제안을 보고하세요.

## 1. 컴파일 수행

```bash
udit editor refresh --compile
```

- 성공 시 다음 단계로.
- 에러 시 즉시 [단계 4 — 에러 분석]으로 이동.

## 2. 콘솔 에러 확인

컴파일 자체는 성공했지만 에러 로그가 남아있을 수 있음:

```bash
udit console --type error --lines 20 --stacktrace user --json
```

- 결과가 빈 배열이면 다음 단계로.
- 에러 로그 있으면 [단계 4]로.

## 3. EditMode 테스트 실행

```bash
udit test --mode EditMode
```

- 전체 통과면 **"✅ verified"** 로 종료.
- 실패 건이 있으면 실패한 테스트 이름과 에러 메시지를 정리해 보고.

## 4. 에러 분석 (실패 시)

에러 발생 시 다음 정보를 수집해 사용자에게 보고:

1. **어느 단계에서 실패했는지** (compile / console / test)
2. **에러 메시지 요약** — 파일명과 줄 번호 포함
3. **가능한 원인** — 자주 보이는 패턴:
   - `CS` 에러 코드 → 문법/타입 오류
   - `NullReferenceException` → 초기화 누락 또는 missing reference
   - `MissingComponentException` → GetComponent 호출 대상 없음
   - `Assembly` 로드 실패 → asmdef references 또는 순환 의존
4. **제안하는 다음 액션** — 수정안 또는 추가 조사 필요한 파일 경로

## 주의사항

- Unity가 **응답하지 않으면** (UCI-002, UCI-020) 2-3초 기다렸다 재시도. 3회 실패 시 `udit status`로 상태 확인 후 보고.
- `udit exec`로 우회하는 검증은 **금지**. 반드시 위 표준 플로우.
- 실패를 절대 **자동으로 넘기지 말 것**. 사용자 확인 없이 "수정해볼게요" 하고 코드 고치지 말 것 — 원인 보고부터.
