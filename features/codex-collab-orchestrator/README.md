# Codex Collaborator Orchestrator

`codex-collab-orchestrator`는 한 레포에서 여러 Codex 세션이 동시에 작업할 때 충돌을 줄이고, context compact 이후에도 작업을 이어갈 수 있도록 설계된 기능 번들입니다.

## 상세 사용법

MCP 메서드 전체 목록, Provider 시스템, tmux Client, v2 신규 기능, Dispatch 패턴, 리소스 라이프사이클 등 상세 사용법은 **[bundle-description.md](./bundle-description.md)**를 참조하세요.

## 핵심 설계: 역할 기반 스킬 분리

각 에이전트는 시작 시점에 자신의 역할을 명확히 알 수 있도록 **역할별 전용 스킬**을 사용합니다:

| 스킬 | 대상 | 역할 인식 |
|------|------|-----------|
| `codestrator` | Root 세션 | "나는 Root Orchestrator다" |
| `codestrator-worker` | Child 세션 (worker) | "나는 Child Worker다" |
| `codestrator-reviewer` | Child 세션 (reviewer) | "나는 Merge Reviewer다" |

Root가 child를 생성할 때 `agent_guide_path`로 해당 스킬을 전달하여, child가 스스로의 역할을 즉시 인지합니다.

## 번들 구성

### 스킬 (Skills)
- `components/skills/codex/codestrator/`: Root Orchestrator 스킬 (부모용)
- `components/skills/codex/codestrator-worker/`: Child Worker 스킬 (자식-구현용)
- `components/skills/codex/codestrator-reviewer/`: Merge Reviewer 스킬 (자식-리뷰용)

### 에이전트 템플릿 (Agent Templates)
- `components/agents/codex/root-orchestrator.md`: Root Orchestrator 상세 가이드
- `components/agents/codex/main-worker.md`: Child Worker 상세 가이드
- `components/agents/codex/merge-reviewer.md`: Merge Reviewer 상세 가이드
- `components/agents/codex/plan-architect.md`: Plan Architect 상세 가이드
- `components/agents/codex/doc-mirror-manager.md`: Doc Mirror Manager 상세 가이드

### Agents SDK 설정
- `components/agents/codex/agents-sdk/worker.yaml`
- `components/agents/codex/agents-sdk/merge-reviewer.yaml`
- `components/agents/codex/agents-sdk/plan-architect.yaml`
- `components/agents/codex/agents-sdk/doc-mirror.yaml`

### MCP 서버
- `components/mcp/servers/codex-orchestrator/`: 상태/락/worktree/재개 오케스트레이션 MCP 서버

### 설계 문서
- `components/mcp/docs/codex-orchestrator-plan.md`

## 아키텍처

```
User
└── Root Orchestrator (codestrator 스킬)
    │
    ├── Worker (codestrator-worker 스킬)
    │   └── 격리된 worktree에서 scoped 구현 실행
    │
    ├── Merge Reviewer (codestrator-reviewer 스킬)
    │   └── merge lock 보유 상태에서 품질 검토
    │
    ├── Plan Architect (agent SDK)
    │   └── 대규모 이니셔티브 분해
    │
    └── Doc Mirror Manager (agent SDK)
        └── SQLite → Markdown 미러 갱신
```

### Dispatch 패턴

| 패턴 | 설명 | 용도 |
|------|------|------|
| **Handoff** (blocking) | spawn + 완료까지 대기 | 결과가 필요한 순차 작업 |
| **Assign** (async) | spawn + 즉시 진행, 나중에 확인 | 병렬 독립 작업 |

### 통신 흐름

| 패턴 | 메커니즘 | 방향 |
|------|----------|------|
| 작업 할당 | `thread.child.spawn` + `task_spec` | Root → Child |
| 중간 지시 | `thread.child.directive` | Root → Child |
| 상태 확인 | `thread.child.list` | Root ← Child |
| 사용자 관찰 | `tmux attach -r` | User ← Child (읽기 전용) |

## 설치

레포 루트에서 기능 단위 설치 스크립트를 사용합니다.

```bash
./scripts/install-feature.sh codex-collab-orchestrator /path/to/target-repo
```

주의: 로컬 MCP가 정상 인식되려면 대상 저장소를 Codex trusted projects에 등록해야 합니다.

드라이런:

```bash
./scripts/install-feature.sh codex-collab-orchestrator /path/to/target-repo --dry-run
```

설치 동기화 검증:

```bash
./scripts/verify-feature-sync.sh codex-collab-orchestrator /path/to/target-repo
```

## 설치 후 주요 경로

설치 대상 저장소 기준(Codex 공식 경로 반영):

- `AGENTS.md` (managed block 추가)
- `.agents/skills/codestrator/` (부모 스킬)
- `.agents/skills/codestrator-worker/` (자식 worker 스킬)
- `.agents/skills/codestrator-reviewer/` (자식 reviewer 스킬)
- `.codex/agents/codex-collab-orchestrator/`
- `.codex/mcp/features/codex-collab-orchestrator/`
- `.codex/config.toml` (MCP 서버 설정 블록 추가)
- `.codex/features/codex-collab-orchestrator/install-manifest.json`

권장: 초기 기동 타임아웃 방지를 위해 `.codex/config.toml`의 해당 MCP 엔트리에 `startup_timeout_sec = 120` 설정

권장: 설치/실행 산출물은 `.gitignore`에 등록

```gitignore
# codex-collab-orchestrator install/runtime artifacts
.agents/
.codex/
.codex-orch/
AGENTS.md
```

`AGENTS.md`를 저장소 정책상 추적해야 하면 해당 줄은 제외

## 공식 문서 기준

- Skills: https://developers.openai.com/codex/skills/
- MCP: https://developers.openai.com/codex/mcp/
- Config: https://developers.openai.com/codex/config/
- AGENTS.md: https://developers.openai.com/codex/agents/

## 서버 개발/검증

```bash
cd features/codex-collab-orchestrator/components/mcp/servers/codex-orchestrator
make test
```
