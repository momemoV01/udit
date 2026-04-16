# udit 쿡북

일반적인 Unity 자동화 시나리오를 위한 실전 워크플로우 레시피 모음입니다.
각 레시피는 목표, 명령 시퀀스, 변형 팁을 제공합니다.

> **모든 레시피 전제 조건:** Unity Editor가 udit Connector 패키지가 설치된 상태로 실행 중이어야 합니다.
> 명령은 기본 포트(8590)를 사용합니다. 변경했다면 `--port N`을 사용하세요.

---

## 1. CI 스모크 테스트

**목표:** 머지 전에 스크립트 컴파일과 테스트 통과를 확인합니다.

```bash
# 1. 리컴파일 트리거 후 완료까지 대기
udit editor refresh --compile

# 2. 컴파일 에러 확인
ERRORS=$(udit console --type error --json | jq '.data.count')
if [ "$ERRORS" -gt 0 ]; then
  echo "컴파일 에러 발견:"
  udit console --type error
  exit 1
fi

# 3. EditMode 테스트를 JUnit 출력으로 실행
udit test run --mode EditMode --output test-results.xml

echo "스모크 테스트 통과."
```

**변형:**
- PlayMode 테스트는 `--mode PlayMode` 추가 (실행 중인 게임 루프 필요).
- 단계 2 대신 `udit project preflight --json`을 사용하면 더 넓은 범위의 상태 검사 가능 (컴파일 상태 + 빌드 설정 + 누락 스크립트).

---

## 2. Prefab 일괄 편집

**목표:** 현재 씬의 모든 프리팹 인스턴스에서 컴포넌트 필드를 업데이트합니다.

```bash
PREFAB="Assets/Prefabs/Enemy.prefab"
COMPONENT="Rigidbody"
FIELD="m_Mass"
VALUE="5.5"

# 1. 프리팹의 모든 씬 인스턴스 찾기
INSTANCES=$(udit prefab find-instances "$PREFAB" --json \
  | jq -r '.data.instances[].id')

# 2. 모든 편집을 하나의 Undo 그룹으로 묶기
udit tx begin --name "일괄 $FIELD 설정"

# 3. 각 인스턴스의 필드 설정
for ID in $INSTANCES; do
  udit component set "$ID" "$COMPONENT" "$FIELD" "$VALUE"
  echo "  업데이트: $ID"
done

# 4. 트랜잭션 커밋
udit tx commit --name "일괄 $FIELD 설정"

echo "완료. $(echo "$INSTANCES" | wc -l)개 인스턴스 업데이트."
```

**팁:**
- 먼저 `component set` 호출에 `--dry-run`을 추가하면 씬을 수정하지 않고 미리 확인할 수 있습니다.
- 프리팹에 같은 타입의 컴포넌트가 여러 개 있으면 `--index N`을 사용하세요.

---

## 3. 프리셋을 활용한 빌드 자동화

**목표:** `.udit.yaml`에 정의된 프리셋으로 프로덕션 플레이어를 빌드합니다.

먼저 `.udit.yaml`에 프리셋을 정의합니다:

```yaml
build:
  targets:
    production:
      target: StandaloneWindows64
      output: Builds/Production
      scenes:
        - Assets/Scenes/Main.unity
        - Assets/Scenes/Level1.unity
      il2cpp: true
```

빌드 실행:

```bash
# 1. 사전 검사 실행
udit project preflight
# 차단 이슈가 있으면 non-zero로 종료

# 2. 프리셋으로 빌드
udit build player --config production

# 3. 빌드 결과 확인
echo "빌드 출력: Builds/Production/"
ls -lh Builds/Production/
```

**변형:**
- `--development`를 추가하면 IL2CPP 대신 Mono로 빠른 이터레이션 빌드 가능.
- `udit build cancel`로 오래 걸리는 빌드를 중단할 수 있습니다.

---

## 4. 에셋 정리 — 미참조 에셋 찾기

**목표:** 프로젝트에서 아무것도 참조하지 않는 텍스처를 식별합니다.

```bash
# 1. 폴더 내 모든 텍스처 찾기
ASSETS=$(udit asset find --type Texture2D --folder Assets/Art --json \
  | jq -r '.data.matches[].path')

echo "$(echo "$ASSETS" | wc -l)개 텍스처의 참조 스캔 중..."

# 2. 각 에셋의 참조 확인
for ASSET in $ASSETS; do
  REFS=$(udit asset references "$ASSET" --limit 1 --json \
    | jq '.data.total')
  if [ "$REFS" -eq 0 ]; then
    echo "  미참조: $ASSET"
  fi
done
```

**팁:**
- 에셋당 전체 프로젝트 스캔이 수행됩니다. 대규모 프로젝트에서는 작업을 나누거나 `--timeout`을 늘리세요.
- 미참조 에셋 삭제: `udit asset delete "$ASSET"` (휴지통으로 이동) 또는 `--permanent`으로 바로 삭제.

---

## 5. 로그 모니터링

**목표:** 플레이 세션 중 Unity 콘솔 에러를 실시간으로 스트리밍합니다.

```bash
# 에러와 예외를 사용자 스택 프레임만 포함하여 스트리밍
udit log tail --type error --filter "NullReference|MissingComponent" --json
```

각 줄은 NDJSON 객체입니다. `jq`로 파싱하여 구조화된 알림 처리:

```bash
# 예시: 초당 에러 수 카운트
udit log tail --type error --json \
  | jq --unbuffered -r '.message' \
  | while read -r MSG; do
      echo "[$(date +%H:%M:%S)] $MSG"
    done
```

**변형:**
- `--since 5m`으로 라이브 전환 전 최근 5분 백필.
- `--type error,warning`으로 경고 포함.
- `--stacktrace full`로 전체 Unity 스택 트레이스.

---

## 6. 프로젝트 상태 리포트

**목표:** 빠른 프로젝트 상태 요약 생성 (데일리 스탠드업이나 대시보드에 유용).

```bash
echo "=== 프로젝트 정보 ==="
udit project info --json | jq '{
  unity: .data.unity_version,
  project: .data.project_name,
  scenes: .data.scenes_in_build,
  packages: (.data.packages | length)
}'

echo ""
echo "=== 유효성 검사 ==="
udit project validate --json | jq '{
  ok: .data.ok,
  errors: (.data.issues | map(select(.severity == "error")) | length),
  warnings: (.data.issues | map(select(.severity == "warning")) | length)
}'

echo ""
echo "=== 콘솔 에러 ==="
udit console --type error --json | jq '.data.count'
```

**팁:**
- `validate` 대신 `udit project preflight`를 사용하면 더 철저한 검사 가능 (빌드 설정과 컴파일 상태 포함).
- cron 작업이나 `watch --path` 훅으로 감싸면 지속적인 모니터링 가능.

---

## 7. `.udit.yaml run`을 활용한 씬 마이그레이션

**목표:** 씬을 열고, 변환을 실행하고, 저장하는 재사용 가능한 마이그레이션 태스크를 정의합니다.

`.udit.yaml`에 태스크 추가:

```yaml
run:
  tasks:
    migrate-lighting:
      steps:
        - udit scene open Assets/Scenes/Main.unity --force
        - udit exec "RenderSettings.ambientMode = UnityEngine.Rendering.AmbientMode.Trilight; return RenderSettings.ambientMode.ToString();"
        - udit scene save
        - udit scene open Assets/Scenes/Level1.unity --force
        - udit exec "RenderSettings.ambientMode = UnityEngine.Rendering.AmbientMode.Trilight; return RenderSettings.ambientMode.ToString();"
        - udit scene save
```

실행:

```bash
# 실행 없이 단계 미리보기
udit run migrate-lighting --dry-run

# 마이그레이션 실행
udit run migrate-lighting
```

**팁:**
- 단계 안에서 `udit tx begin` / `udit tx commit`을 사용하면 변경 사항을 하나의 Undo 항목으로 그룹화할 수 있습니다.
- 일부 씬의 유효성 검사가 실패할 수 있다면 태스크에 `continue_on_error: true`를 추가하세요.
- 태스크 중첩 가능: 단계에서 `udit run <다른-태스크>`를 호출할 수 있습니다 (순환 감지 내장).
