---
name: udit-release
description: udit 자체의 릴리스 프로세스. Verification → 버전 올리기 → 커밋 → 태그 push → CI 완료 대기 → 로컬 바이너리 업데이트.
user-invocable: true
---

당신은 udit 프로젝트의 **새 버전을 배포**하려 합니다. 단계별로 진행하고, 각 검증을 건너뛰지 마세요.

## 0. 사용자 확인

새 버전 번호를 확인하세요:
- **Connector만 변경**: `udit-connector/package.json`의 `version` 올림. git tag는 **안 만듦**.
- **CLI만 변경**: git tag `vX.Y.Z` 생성. package.json은 **안 건드림**.
- **둘 다 변경**: 양쪽 모두.

SemVer 규칙:
- `MAJOR` — breaking change (v1.0 전엔 거의 쓸 일 없음)
- `MINOR` — 새 기능 (Phase 1~5의 각 단계)
- `PATCH` — 버그 픽스, 문서 수정

## 1. Verification (필수)

```bash
cd E:/Workspace/udit

go clean -testcache
gofmt -w .
go vet ./...
go test ./...
```

linter가 설치돼 있으면:
```bash
~/go/bin/golangci-lint run ./...
~/go/bin/golangci-lint fmt --diff
```

**하나라도 실패하면 릴리스 중단**. 원인 수정 후 처음부터 다시.

## 2. CHANGELOG.md 업데이트

새 버전 섹션 추가:
```markdown
## [0.2.0] - YYYY-MM-DD

### Added
- ...

### Changed
- ...

### Fixed
- ...
```

날짜는 **실제 배포일** 기준.

## 3. 버전 번호 올리기

**Connector 변경이면**:
```bash
# udit-connector/package.json 의 "version" 필드 수정
# 예: "0.1.0" → "0.2.0"
```

**CLI 변경이면**: git tag 단계에서.

## 4. 커밋

```bash
git add -A
git status                 # 포함될 파일 재확인
git commit -m "release: vX.Y.Z

<간단한 하이라이트>
"
```

커밋 메시지 규칙:
- 첫 줄: `release: vX.Y.Z`
- 본문: 주요 변경사항 3-5줄

## 5. Push (main) + Tag push

```bash
git push origin main
```

CI 워크플로 통과 확인:
```bash
export PATH="/c/Program Files/GitHub CLI:$PATH"
gh run list --repo momemoV01/udit --limit 2
gh run watch --exit-status <run-id>
```

**CI 실패면 여기서 중단**. 수정 후 재커밋.

CI 통과하면 태그:
```bash
git tag -a vX.Y.Z -m "udit vX.Y.Z — <한 줄 요약>"
git push origin vX.Y.Z
```

## 6. Release 워크플로 대기

```bash
gh run list --repo momemoV01/udit --limit 2
# Release가 queued → in_progress → completed 순서로 진행
gh run watch --exit-status <release-run-id>
```

5개 플랫폼 빌드 + Release 페이지 생성까지 보통 3-5분.

## 7. Release 검증

```bash
gh release view vX.Y.Z --repo momemoV01/udit
gh api repos/momemoV01/udit/releases/tags/vX.Y.Z --jq '.assets[].name'
```

5개 바이너리 모두 첨부됐는지 확인:
- udit-linux-amd64
- udit-linux-arm64
- udit-darwin-amd64
- udit-darwin-arm64
- udit-windows-amd64.exe

## 8. 로컬 캐시 정리 + 업데이트

```bash
go clean -cache -testcache
```

로컬 사용 바이너리를 새 버전으로:
```bash
# Private 저장소 중이면 gh 다운로드
gh release download vX.Y.Z --repo momemoV01/udit \
  --pattern "udit-windows-amd64.exe" \
  --output "$LOCALAPPDATA/udit/udit.exe"

udit version    # 새 버전 확인
```

## 9. 사후 확인

- Unity 프로젝트에 Connector 패키지가 있다면 새 버전으로 업데이트
- Release 페이지 URL을 사용자에게 공유
- 다음 버전 스코프가 `docs/ROADMAP.md`에 정의돼 있는지 확인

## 주의사항

- **`git push --force` 금지** (안전장치). 실수로 올린 커밋은 새 커밋으로 되돌림.
- CI 실패 시 `--no-verify`로 우회하지 말 것. 실패 원인이 진짜 버그.
- 태그는 **CI 통과 후에만** push. 태그 push하면 Release가 자동 생성되므로 되돌리기 번거로움.
- Connector 버전과 CLI 태그를 **반드시 일치시킬 필요 없음**. 변경된 쪽만 올림.
