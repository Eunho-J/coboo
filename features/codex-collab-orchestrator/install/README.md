# Install Notes

이 기능 번들은 루트 스크립트로 설치하며, Codex 공식 경로(`.agents`, `.codex`, `AGENTS.md`)에 배치됩니다.

```bash
./scripts/install-feature.sh codex-collab-orchestrator /path/to/target-repo
```

설치 후 Codex 세션을 재시작하면 새 Skill/MCP/AGENTS 지시사항이 반영됩니다.

설치 검증:

```bash
cd /path/to/target-repo
codex mcp list
```

공식 문서:

- Skills: https://developers.openai.com/codex/skills/
- MCP: https://developers.openai.com/codex/mcp/
- Config: https://developers.openai.com/codex/config/
- AGENTS.md: https://developers.openai.com/codex/agents/
