# LLM Ops Workspace

Codex, Claude, Gemini를 더 효율적으로 쓰기 위한 **Skill / Agent / MCP 자산**을 한 곳에 모아 관리하는 레포지토리입니다.

## 목적

- 반복 작업을 재사용 가능한 스킬/에이전트로 축적
- 모델별(Codex/Claude/Gemini) 설정 차이를 명확히 분리
- MCP 서버/설정/문서를 공통 기준으로 관리

## 디렉터리 구조

```text
.
├── README.md
├── agents/
│   ├── claude/
│   ├── codex/
│   └── gemini/
├── mcp/
│   ├── configs/
│   ├── docs/
│   └── servers/
└── skills/
    ├── claude/
    ├── codex/
    └── gemini/
```

## 작업 규칙 (간단)

1. **새 항목은 목적별로 분리**
   - Skill은 `skills/<provider>/...`
   - Agent 프롬프트/설정은 `agents/<provider>/...`
   - MCP 관련 자산은 `mcp/{servers|configs|docs}/...`
2. **작은 단위로 커밋**
   - 하나의 커밋은 하나의 변경 의도만 담기
3. **문서 우선**
   - 새 자산 추가 시 사용 방법을 함께 기록

## 시작 가이드

- 새 스킬 추가: `skills/<provider>/<skill-name>/`
- 새 에이전트 추가: `agents/<provider>/<agent-name>/`
- MCP 서버 추가: `mcp/servers/<server-name>/`

필요해지면 이후에 템플릿, 예시, 검증 스크립트를 단계적으로 확장합니다.
