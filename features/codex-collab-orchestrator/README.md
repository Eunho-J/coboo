# Codex Collaborator Orchestrator

`codex-collab-orchestrator`는 한 레포에서 여러 Codex 세션이 동시에 작업할 때 충돌을 줄이고, context compact 이후에도 작업을 이어갈 수 있도록 설계된 기능 번들입니다.

## 번들 구성

- `components/mcp/servers/codex-orchestrator/`: 상태/락/worktree/재개 오케스트레이션 MCP 서버
- `components/skills/codex/codex-work-orchestrator/`: Codex 실행 워크플로우 강제 스킬
- `components/agents/codex/*.md`: `main-worker`, `merge-reviewer`, `doc-mirror-manager`, `plan-architect` 템플릿
- `components/mcp/docs/codex-orchestrator-plan.md`: 설계 문서

## 설치 (선택 기능 설치)

레포 루트에서 기능 단위 설치 스크립트를 사용합니다.

```bash
./scripts/install-feature.sh codex-collab-orchestrator /path/to/target-workspace
```

드라이런:

```bash
./scripts/install-feature.sh codex-collab-orchestrator /path/to/target-workspace --dry-run
```

## 설치 후 주요 경로

설치 대상 워크스페이스 기준:

- `agents/codex/main-worker.md`
- `agents/codex/merge-reviewer.md`
- `agents/codex/doc-mirror-manager.md`
- `skills/codex/codex-work-orchestrator/`
- `mcp/docs/codex-orchestrator-plan.md`
- `mcp/servers/codex-orchestrator/`

## 서버 개발/검증

```bash
cd features/codex-collab-orchestrator/components/mcp/servers/codex-orchestrator
make test
```
