# Install Notes

이 기능 번들은 루트 설치 스크립트로 배포되며, Codex 공식 경로(`.agents`, `.codex`, `AGENTS.md`)에 설치됩니다.

```bash
./scripts/install-feature.sh playwright /path/to/target-repo
```

설치 후 Codex 세션을 재시작하면 Skill/MCP/AGENTS 지시사항이 반영됩니다.

설치 검증:

```bash
cd /path/to/target-repo
codex mcp list
```

기대 항목:

- `playwright` MCP 서버가 목록에 표시
- `.agents/skills/playwright/SKILL.md` 존재

필수 런타임:

- Node.js + npm
- `npx -y @anthropic-ai/claude-code-mcp-server-playwright@latest` 실행 가능 환경

권장 `.gitignore`:

```gitignore
# codex feature install/runtime artifacts
.agents/
.codex/
.codex-orch/
AGENTS.md
```
