# Playwright

`playwright`는 브라우저 자동화/E2E 워크플로우를 Codex에 추가하는 기능 번들입니다.

## 번들 구성

- `components/skills/codex/playwright/`: `oh-my-claude`의 Playwright 스킬 원본
- `components/mcp/config.toml`: Codex용 Playwright MCP 서버 설정 블록

MCP는 npm 기반 서버를 사용합니다.

```bash
npx -y @playwright/mcp@latest --headless
```

공식 기본 예시는 아래와 같습니다.

```toml
[mcp_servers.playwright]
command = "npx"
args = ["@playwright/mcp@latest"]
```

이 번들은 캐시 손상으로 인한 초기 handshake 실패를 줄이기 위해 전용 npm 캐시 경로를 함께 설정합니다.

## 설치

```bash
./scripts/install-feature.sh playwright /path/to/target-repo
```

드라이런:

```bash
./scripts/install-feature.sh playwright /path/to/target-repo --dry-run
```

## 설치 후 경로

설치 대상 저장소 기준:

- `.agents/skills/playwright/`
- `.codex/mcp/features/playwright/`
- `.codex/config.toml` (`codex-feature:playwright:mcp` managed block)
- `AGENTS.md` (`codex-feature:playwright:agents` managed block)

## 요구사항

- Node.js + npm (또는 npx) 실행 가능 환경
- 첫 실행 시 npm 네트워크 접근 필요

## 원본 출처

- Skill source: `https://github.com/Eunho-J/oh-my-claude/tree/main/.claude/skills/playwright`
