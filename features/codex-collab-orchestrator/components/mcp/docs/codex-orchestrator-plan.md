# Codex 협업 충돌방지/상태복구 툴 설계 계획

## 1) 목표

- 한 레포지토리에서 다수 사용자/다수 Codex 세션이 동시에 작업해도 충돌을 최소화한다.
- 컨텍스트 컴팩트 또는 세션 재시작 시, 현재 작업 상태를 즉시 확인하고 이어서 진행할 수 있게 한다.
- 작업을 작은 단위의 계층형 체크리스트로 분해하고, 각 단위 완료 시 상태를 즉시 업데이트한다.

## 2) 구현 형태

- 조합: `MCP 서버 + Codex Skill + Agent 템플릿`
- 격리 전략: 하이브리드 기본
  - 대규모/고위험/장기 작업: 별도 `git worktree`
  - 경량/저위험 작업: 같은 worktree + file lock
- 상태 SoT: SQLite
- 문서 미러: Markdown은 요청 시에만 생성/갱신 (지연 동기화)
- 미러 갱신 주체: 별도 전용 Doc Agent

## 3) 작업 계층 모델

- `Epic → Feature → TestGroup → Case → Step`
- 복구 기준(anchor): `Case` 단위 체크포인트
- Case 규칙:
  - 시작 시 입력 계약(`input contract`)과 fixture를 명시
  - 다른 Case 산출물 참조 금지
  - 완료 시 즉시 상태 업데이트 + 다음 액션 기록

## 4) 충돌 방지/격리 정책

- worktree 전환 기준: 점수 기반 자동 판정
  - 예시 지표: 변경 파일 수, 예상 시간, 리스크, 병렬 참여자 수, 충돌 가능 경로
- lock 전략: 경로 prefix 락 + 파일 락 혼합
  - 기본은 prefix 락
  - 필요한 경우 파일 단위로 세분화

## 5) 병합 정책

- 병렬 worktree 결과 병합은 전용 Merge Agent가 필수 검토한다.
- 충돌 시에는 관련 기능 작업 문서만 로드해 보완한다.
- 작성 에이전트와 병합 검토 에이전트는 분리한다.

## 6) 상태 저장/미러 정책

- SQLite가 단일 진실원천(SoT)이다.
- 상태 변경 트랜잭션마다 `db_version`을 증가시킨다.
- 조회 시 `db_version`과 `md_version`을 비교해 outdated 여부를 판단한다.
- Markdown 미러는 요청 시에만 갱신한다.
- 작업 에이전트는 미러 갱신 요청만 하고, 갱신 처리 자체는 하지 않는다.

## 7) 초기 MCP API 초안

- `workspace.init(repo_path)`
- `task.create(...)`, `task.list(...)`, `task.get(task_id)`
- `scheduler.decide_worktree(task_id)`
- `worktree.create(task_id, branch_name)`, `worktree.list()`
- `lock.acquire(...)`, `lock.heartbeat(lock_id)`, `lock.release(lock_id)`
- `case.begin(...)`, `step.check(...)`, `case.complete(...)`
- `resume.next(owner_or_session)`
- `merge.request(feature_id, source_worktrees)`, `merge.review_context(merge_request_id)`
- `mirror.status()`, `mirror.refresh(target)`

## 8) SQLite 스키마 초안

- `sessions`
- `tasks`
- `cases`
- `steps`
- `checkpoints`
- `locks`
- `worktrees`
- `merge_requests`
- `mirror_meta`

## 9) 구현 순서

1. Go 기반 MCP 서버/스토리지 스캐폴드
2. 핵심 상태 API (task/case/step/checkpoint/lock)
3. worktree 판정/생성 흐름
4. resume 흐름
5. merge review 흐름
6. mirror status/refresh + Doc Agent 분리
7. Codex skill + agent 템플릿 정리
8. 운영 가이드 문서 정리

## 10) 고도화 반영 (v2)

- 다중 터미널 동일 repo 진입 시 세션별 `session-root` worktree 자동 생성
- `intent=resume_work`일 때 중단 세션 후보 조회 후 attach
- compact/세션 재시작 대응용 `work.current_ref` / `work.current_ref.ack` 추가
- child worktree → session-root 병합 API (`worktree.merge_to_parent`) 추가
- main 병합 큐/전역 락 API (`merge.main.*`) 추가
