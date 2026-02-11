# Codex Collaborator Orchestrator

`codex-collab-orchestrator`는 한 레포에서 여러 Codex 세션이 동시에 작업할 때 충돌을 줄이고, context compact 이후에도 작업을 이어갈 수 있도록 설계된 기능 번들입니다.

## 번들 구성

- `components/mcp/servers/codex-orchestrator/`: 상태/락/worktree/재개 오케스트레이션 MCP 서버
- `components/skills/codex/codex-work-orchestrator/`: Codex 실행 워크플로우 강제 스킬
- `components/agents/codex/*.md`: `main-worker`, `merge-reviewer`, `doc-mirror-manager`, `plan-architect` 템플릿
- `components/mcp/docs/codex-orchestrator-plan.md`: 설계 문서

추가 고도화:

- `runtime.tmux.ensure`로 tmux 자동 준비(실패 시 수동 설치 fallback 안내)
- `thread.root.ensure` / `thread.child.*`로 nested agent thread + tmux pane 관리
- `merge.review.request_auto`로 merge reviewer thread 자동 디스패치

## 설치 (선택 기능 설치)

레포 루트에서 기능 단위 설치 스크립트를 사용합니다.

```bash
./scripts/install-feature.sh codex-collab-orchestrator /path/to/target-repo
```

주의: 로컬 MCP가 정상 인식되려면 대상 저장소를 Codex trusted projects에 등록해야 합니다.

- `--sandbox danger-full-access`로 실행하면 trusted projects 검증이 생략되어 로컬 MCP 서버가 표시되지 않을 수 있습니다.
- 설치 직후에는 대상 저장소를 trusted로 승인하고 Codex 세션을 재시작하세요.

드라이런:

```bash
./scripts/install-feature.sh codex-collab-orchestrator /path/to/target-repo --dry-run
```

## 설치 후 주요 경로

설치 대상 저장소 기준(Codex 공식 경로 반영):

- `AGENTS.md` (managed block 추가)
- `.agents/skills/codex-work-orchestrator/`
- `.codex/agents/codex-collab-orchestrator/`
- `.codex/mcp/features/codex-collab-orchestrator/`
- `.codex/config.toml` (MCP 서버 설정 블록 추가)

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
