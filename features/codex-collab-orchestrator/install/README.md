# Install Notes

이 기능 번들은 루트 스크립트로 설치하며, Codex 공식 경로(`.agents`, `.codex`, `AGENTS.md`)에 배치됩니다.

```bash
./scripts/install-feature.sh codex-collab-orchestrator /path/to/target-repo
```

중요: 로컬 MCP 서버를 프로젝트 단위로 사용할 때는 **해당 대상 저장소를 Codex의 trusted projects에 먼저 등록**해야 합니다.

- `--sandbox danger-full-access`로 세션을 시작하면 trusted projects 검증 프롬프트가 생략되어, 로컬 MCP가 목록에 안 뜨는 상황이 발생할 수 있습니다.
- 설치 후 Codex에서 대상 저장소(`/path/to/target-repo`)를 trusted로 승인한 다음 세션을 재시작하세요.

설치 후 Codex 세션을 재시작하면 새 Skill/MCP/AGENTS 지시사항이 반영됩니다.

설치 산출물이 저장소에 섞이지 않도록 `.gitignore` 등록을 권장합니다.

```gitignore
# codex-collab-orchestrator install/runtime artifacts
.agents/
.codex/
.codex-orch/
AGENTS.md
```

`AGENTS.md`를 저장소에서 직접 관리할 계획이면 해당 항목은 제외하세요.

설치 검증:

```bash
cd /path/to/target-repo
codex mcp list
```

첫 실행에서 Go toolchain 다운로드/컴파일로 초기 기동이 느릴 수 있으므로, 로컬 MCP에 아래 설정을 권장합니다.

```toml
[mcp_servers.codex_collab_orchestrator_codex_orchestrator]
startup_timeout_sec = 120
```

공식 문서:

- Skills: https://developers.openai.com/codex/skills/
- MCP: https://developers.openai.com/codex/mcp/
- Config: https://developers.openai.com/codex/config/
- AGENTS.md: https://developers.openai.com/codex/agents/
