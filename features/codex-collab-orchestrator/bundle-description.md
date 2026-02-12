# Codestrator v2 Bundle Description

## 개요

Codestrator는 단일 레포지토리에서 **복수의 Codex/Claude Code 에이전트를 병렬 관리**하는 오케스트레이션 시스템입니다. tmux 기반 세션 관리, 격리된 worktree, 역할 기반 스킬, 구조화된 작업 분해를 제공합니다.

## 아키텍처

```
Root Orchestrator (caller CLI - tmux 밖)
├── Worker 1 (tmux pane, 격리 worktree)
├── Worker 2 (tmux pane, 격리 worktree)
├── Worker N ...
├── Merge Reviewer (merge lock 시 spawn)
├── Plan Architect (대규모 기획 시 spawn)
└── Doc Mirror Manager (유틸리티)
```

**핵심 원칙:** One Case = One Worker = One Worktree

## MCP Tool Groups (9개 그룹, 63개 메서드)

| 그룹 | 메서드 수 | 용도 |
|------|----------|------|
| `orch_session` | 7 | 세션/워크스페이스 초기화 및 라이프사이클 |
| `orch_task` | 9 | 작업 생성/조회, 케이스 실행, 재개 |
| `orch_graph` | 5 | 의존성 그래프, 체크리스트, 스냅샷 |
| `orch_workspace` | 8 | Worktree CRUD, 스케줄링, 락 관리 |
| `orch_thread` | 8 | 자식 스레드 spawn/control/status |
| `orch_lifecycle` | 2 | 체크포인트, 재개 |
| `orch_merge` | 9 | 머지 큐, 리뷰 디스패치, 락 |
| `orch_inbox` | 4 | 스레드 간 메시징 |
| `orch_system` | 11 | 런타임, 미러, 플랜 부트스트랩 |

### 메서드 상세

**orch_session** (7)
- `workspace.init` - 레포/DB 경로 초기화
- `session.open` / `session.close` - 세션 라이프사이클
- `session.cleanup` / `session.list` / `session.context` / `session.heartbeat`

**orch_task** (9)
- `task.create` / `task.list` / `task.get` - 작업 관리
- `case.begin` / `case.complete` - 케이스 라이프사이클
- `step.check` - 스텝 완료 체크
- `resume.next` / `resume.candidates.*` - 재개 관리

**orch_graph** (5)
- `graph.node.create` / `graph.node.list` - 노드 관리
- `graph.edge.create` - 의존성 엣지
- `graph.checklist.upsert` / `graph.snapshot.create` - 스냅샷

**orch_workspace** (8)
- `scheduler.decide_worktree` - Worktree 스케줄링
- `worktree.create` / `worktree.list` / `worktree.spawn` / `worktree.merge_to_parent`
- `lock.acquire` / `lock.heartbeat` / `lock.release` - 락 관리

**orch_thread** (8)
- `thread.child.spawn` / `thread.child.directive` / `thread.child.list` - 자식 관리
- `thread.child.interrupt` / `thread.child.stop` / `thread.child.status` / `thread.child.wait_status` - 제어
- `thread.attach_info` - 사용자 접속 정보

**orch_lifecycle** (2)
- `work.current_ref` - 체크포인트 get/set
- `work.current_ref.ack` - 재개 확인

**orch_merge** (9)
- `merge.request` / `merge.review_context` - 머지 설정
- `merge.review.request_auto` / `merge.review.thread_status` - 리뷰 디스패치
- `merge.main.request` / `merge.main.next` / `merge.main.status` - 메인 머지 큐
- `merge.main.acquire_lock` / `merge.main.release_lock` - 락 제어

**orch_system** (11)
- `runtime.tmux.ensure` / `runtime.bundle.info` - 런타임 점검
- `mirror.status` / `mirror.refresh` - SQLite→Markdown 미러
- `plan.bootstrap` / `plan.slice.generate` / `plan.slice.replan` - 플랜 관리
- `plan.rollup.preview` / `plan.rollup.submit` / `plan.rollup.approve` / `plan.rollup.reject`

**orch_inbox** (4)
- `inbox.send` / `inbox.pending` / `inbox.list` / `inbox.deliver`

## 역할별 스킬 (3개)

### 1. Root Orchestrator (`codestrator`)

- **위치:** `.agents/skills/codestrator/SKILL.md`
- **도구 접근:** 전체 9개 그룹 (63개 메서드)
- **5-Phase 워크플로우:**

```
Phase 1: Initialize
  workspace.init → session.open → runtime.tmux.ensure

Phase 2: Plan & Decompose
  task.create (Epic → Feature → TestGroup → Case → Step)
  graph.node.create + graph.edge.create

Phase 3: Execute (Dispatch Children)
  ├── Handoff 패턴: spawn → wait_status → 완료까지 대기 (순차)
  └── Assign 패턴: spawn N개 → 주기적 poll (병렬)

Phase 4: Review & Merge
  merge.main.acquire_lock → merge.review.request_auto → poll → release_lock

Phase 5: Completion
  session.close → 사용자에게 요약 보고
```

### 2. Child Worker (`codestrator-worker`)

- **위치:** `.agents/skills/codestrator-worker/SKILL.md`
- **도구 접근:** 4개 그룹 (orch_task, orch_lifecycle, orch_workspace, orch_inbox)
- **3-Phase 워크플로우:**

```
Phase 1: Startup
  task_spec 읽기 → scope 확인 → 체크포인트 확인

Phase 2: Execute
  case.begin → (implement → step.check → checkpoint) 반복 → case.complete

Phase 3: Finalize
  worktree.merge_to_parent → 완료 보고
```

- **Directive 모드:**
  - `interrupt_patch` (기본): 즉시 중단, 패치 적용, 패치 지점부터 계속
  - `queue`: 현재 step 완료 후 적용
  - `restart`: 현재 작업 폐기, task_spec 재읽기, 처음부터 시작

### 3. Merge Reviewer (`codestrator-reviewer`)

- **위치:** `.agents/skills/codestrator-reviewer/SKILL.md`
- **도구 접근:** 4개 그룹 (orch_merge, orch_graph, orch_task, orch_inbox)
- **검사 영역:**
  1. **Merge Conflicts** - 파일 수준 및 의미적 충돌
  2. **Missing Changes** - 계획되었으나 미구현, 수용 기준 미충족
  3. **Contract Violations** - 마이그레이션 없는 API 변경, 타입 파괴, 버전 범프 누락
  4. **Quality Issues** - 데드 코드, 미테스트 경로, 보안, 성능 회귀
- **출력:** 증거 기반 Findings (file:line, severity, description, evidence) + APPROVE/BLOCK verdict

## 에이전트 템플릿 (5개)

| 에이전트 | 파일 | 역할 |
|---------|------|------|
| Root Orchestrator | `root-orchestrator.md` | 세션 초기화, 작업 계층, 자식 라이프사이클, 머지, 에러 복구 |
| Main Worker | `main-worker.md` | 시작 프로토콜, 실행, 코딩 표준, directive 응답, blocker 보고 |
| Merge Reviewer | `merge-reviewer.md` | 리뷰 체크리스트, verdict 기준, 증거 표준, inbox 통신 |
| Plan Architect | `plan-architect.md` | 4-phase (분석→분해→검증→전달), 재계획 트리거, 계층 검증 |
| Doc Mirror Manager | `doc-mirror-manager.md` | SQLite→Markdown 미러 갱신 유틸리티 |

## Agents SDK 설정 (4개 YAML)

| YAML | 도구 화이트리스트 |
|------|-----------------|
| `worker.yaml` | orch_task, orch_lifecycle, orch_workspace, orch_inbox |
| `merge-reviewer.yaml` | orch_merge, orch_graph, orch_task, orch_inbox |
| `plan-architect.yaml` | orch_graph, orch_system, orch_task |
| `doc-mirror.yaml` | orch_system |

모든 에이전트는 `model: codex` 바인딩 사용.

## Provider 시스템

두 가지 CLI 도구를 추상화하여 상태 감지:

| Provider | Idle 패턴 | Exit 명령 | 상태 감지 |
|----------|----------|-----------|----------|
| `codex` | `❯`, `›`, `codex>` | `/exit` | regex 기반 |
| `claude_code` | `>` | `/exit` | `⏺` 응답마커, `✶✢✽✻·✳` 스피너 |

**Status Enum:** `idle` | `processing` | `completed` | `waiting_user_answer` | `error`

**2-Tier 상태 감지:**
1. **Tier 1 (Fast):** pipe-pane 로그 파일 tail (4KB) → regex 매칭
2. **Tier 2 (Full):** tmux capture-pane (200줄) → provider.GetStatus() 분석

## tmux Client 라이브러리

`load-buffer` + `paste-buffer` 패턴으로 멀티라인 안정 전송:

| 카테고리 | 메서드 |
|---------|--------|
| 세션 관리 | `NewSession`, `KillSession`, `ListSessions`, `ListOwnedSessions` |
| Pane 조작 | `SplitWindow`, `KillPane`, `ListPanes`, `PaneExists` |
| 입출력 | `SendKeys` (load-buffer→paste-buffer→Enter), `StartPipePane`, `StopPipePane` |
| 검사 | `CaptureHistory`, `GetPaneWorkingDirectory` |

## v2 신규 기능

### 1. `waitUntilStatus()` - 내부 폴링 메서드

```
backoff: 500ms → 1s → 2s → 4s → 5s (cap)
Tier 1: log tail → provider.GetStatus()
Tier 2: tmux capture → provider.GetStatus()
context 취소 지원, detached cleanup context (context.WithoutCancel + 5s timeout)
```

### 2. Post-Spawn Readiness Check

- `thread.child.spawn` 후 자동으로 30s idle 대기
- `skip_ready_check: true`로 비활성화 가능
- 결과: `idle` → "running", `timeout` → "initializing", `error` → "initializing"

### 3. Parent Thread CWD Inheritance

- `worktree_id` 미지정 시 부모 pane의 현재 작업 디렉토리 상속
- 실패 시 graceful fallback (기존 resolveThreadWorkdir 결과 사용)

### 4. `thread.child.wait_status` MCP 메서드

```json
// 요청
{
  "method": "thread.child.wait_status",
  "params": {
    "thread_id": 5,
    "target_statuses": ["idle", "completed"],
    "timeout_seconds": 60
  }
}

// 응답 (성공)
{
  "thread_id": 5,
  "achieved_status": "idle",
  "last_response": "Task completed successfully.",
  "elapsed_ms": 3200
}

// 응답 (타임아웃)
{
  "thread_id": 5,
  "error": "thread 5 did not reach target status within 1m0s",
  "elapsed_ms": 60002
}
```

## 작업 계층 구조

```
Epic (최상위 목표)
└── Feature (배포 단위)
    └── TestGroup (검증 범위)
        └── Case (원자적 작업 단위 → 1 Worker)
            └── Step (체크리스트 항목)
```

## Dispatch 패턴

### Handoff (순차 실행)

```
spawn worker → wait_status(idle) → directive → wait_status(idle) → ... → stop
```

하나의 작업이 완료될 때까지 대기 후 다음 진행. 결과가 필요한 순차 작업에 적합.

### Assign (병렬 실행)

```
spawn worker1, worker2, worker3
loop:
  thread.child.list → 상태 확인
  완료된 worker에 새 case 할당 (directive)
  전부 완료 시 종료
```

독립적인 작업을 동시에 처리할 때 적합.

## 통신 체계

### Inbox 메시징

```
Root → Worker:  task 할당, directive
Worker → Root:  상태 보고, blocker 보고
Reviewer → Root: merge verdict (APPROVE/BLOCK)
```

### 도구 접근 행렬

| 역할 | session | task | graph | workspace | thread | lifecycle | merge | inbox | system |
|------|:-------:|:----:|:-----:|:---------:|:------:|:---------:|:-----:|:-----:|:------:|
| Root | O | O | O | O | O | O | O | O | O |
| Worker | - | O | - | O | - | O | - | O | - |
| Reviewer | - | O | O | - | - | - | O | O | - |
| Plan Architect | - | O | O | - | - | - | - | - | O |
| Doc Mirror | - | - | - | - | - | - | - | - | O |

## 리소스 라이프사이클

Spawn 실패 시 6단계 방어:

| 실패 지점 | 존재하는 리소스 | Cleanup |
|-----------|--------------|---------|
| provider.Create | pane | KillPane + DB "failed" |
| StartPipePane | pane + provider | provider.Remove + KillPane + DB |
| metadata UpdateThread | pane + provider + pipe | cleanupOnFailure() |
| SendKeys | pane + provider + pipe | cleanupOnFailure() |
| final UpdateThread | pane + provider + pipe | cleanupOnFailure() |
| buildAttachInfo | pane + provider + pipe | cleanupOnFailure() |

모든 cleanup은 **detached context** (`context.WithoutCancel` + 5s timeout) 사용. caller context 취소 시에도 안전하게 리소스 정리.

## 설치 후 생성 파일

```
대상 레포/
├── AGENTS.md                            # 번들 설명 (managed block)
├── .agents/skills/codestrator/          # Root 스킬
├── .agents/skills/codestrator-worker/   # Worker 스킬
├── .agents/skills/codestrator-reviewer/ # Reviewer 스킬
├── .codex/agents/                       # Agent SDK YAML
├── .codex/mcp/                          # MCP 서버 바이너리
├── .codex/config.toml                   # MCP 서버 등록
└── .codex-orch/                         # 런타임 (DB, 로그)
    ├── orchestrator.db                  # SQLite 상태 저장소
    └── logs/                            # pipe-pane 로그
```

## 서버 개발/검증

```bash
cd features/codex-collab-orchestrator/components/mcp/servers/codex-orchestrator
make test       # 유닛 테스트
make run-serve  # MCP 서버 기동
```
