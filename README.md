# LLM Ops Workspace

이 레포는 **기능 번들(feature bundle)** 단위로 Skill/Agent/MCP를 묶어 관리합니다.

핵심 원칙:

- 기능은 `mcp + skill + agent`가 유기적으로 동작하는 최소 단위
- 기능은 서로 독립적으로 추가/설치 가능
- 사용자는 필요한 기능만 선택 설치

## 디렉터리 구조

```text
.
├── README.md
├── features/
│   ├── README.md
│   └── <feature-id>/
│       ├── README.md
│       ├── feature.yaml
│       ├── install/
│       └── components/
│           ├── agents/
│           ├── skills/
│           └── mcp/
└── scripts/
    ├── list-features.sh
    └── install-feature.sh
```

## 기능 카탈로그

- `codex-collab-orchestrator`: 다중 Codex 세션 충돌 방지 + 상태 복구 + worktree/lock 오케스트레이션  
  - 상세: `features/codex-collab-orchestrator/README.md`

## 기능 선택 설치

설치 가능한 기능 목록:

```bash
./scripts/list-features.sh
```

특정 기능만 설치:

```bash
./scripts/install-feature.sh <feature-id> /path/to/target-workspace
```

예시:

```bash
./scripts/install-feature.sh codex-collab-orchestrator /path/to/target-workspace
```

드라이런:

```bash
./scripts/install-feature.sh codex-collab-orchestrator /path/to/target-workspace --dry-run
```

## LLM 사용 예시

- `features 목록 확인하고 codex-collab-orchestrator만 ~/my-workspace에 설치해줘.`
- `codex-collab-orchestrator의 MCP 서버 테스트를 실행해줘.`
