# codex-orchestrator

Codex 다중 세션 협업을 위한 상태/락/작업 재개 오케스트레이터입니다.

## 제공 기능

- SQLite 기반 작업 상태 저장 (`.codex-orch/state.db`)
- 계획/실행 통합 그래프 저장 (`graph.node.*`, `plan.*`)
- 계층형 작업 단위 (`Epic → Feature → TestGroup → Case → Step`)
- Case 단위 체크포인트 및 재개 (`resume.next`)
- 세션별 session-root worktree 자동 생성 (`session.open`)
- 재개 후보 조회/attach (`resume.candidates.list`, `resume.candidates.attach`)
- compact-safe 현재 작업 참조 (`work.current_ref`)
- 경로 prefix + 파일 락 혼합 제어
- worktree 필요성 점수 판정
- main 병합 큐 + 전역 병합 락 (`merge.main.*`)
- tmux 런타임 준비/자동설치 + fallback 안내 (`runtime.tmux.ensure`)
- session-root/child thread 오케스트레이션 (`thread.*`)
- merge reviewer thread 자동 디스패치 (`merge.review.request_auto`, `merge.review.thread_status`)
- Markdown 미러 지연 동기화 (`mirror.status`, `mirror.refresh`)

## Zero-Setup 실행 방식

이 디렉터리에는 **로컬 Go 자동 설치 스크립트**가 포함됩니다.

- `scripts/bootstrap-go.sh`: Go 툴체인을 `.toolchains/`에 자동 설치
- `scripts/go.sh`: Go 명령 래퍼 (`go test`, `go run` 등 실행)
- `Makefile`: 테스트/실행 명령 제공

즉, 사용자는 전역 Go 설치 없이 실행할 수 있습니다.

```bash
cd features/codex-collab-orchestrator/components/mcp/servers/codex-orchestrator
make test
```

첫 실행 시 네트워크로 Go 아카이브를 내려받고, 이후 재사용합니다.

## LLM에게 맡기는 사용 예시

아래처럼 Codex(또는 다른 LLM 에이전트)에게 그대로 요청하면 됩니다.

- `features/codex-collab-orchestrator/components/mcp/servers/codex-orchestrator에서 make test 실행해줘. 필요한 Go 환경은 로컬 스크립트로 자동 구성해줘.`
- `codex-orchestrator 서버를 실행해줘. mode=serve, repo는 현재 레포 루트로 설정해줘.`
- `현재 DB 상태 미러가 outdated인지 확인하고, 필요하면 doc-mirror-manager role로 mirror.refresh 실행해줘.`

## 주요 명령

```bash
cd features/codex-collab-orchestrator/components/mcp/servers/codex-orchestrator

# 테스트
make test

# 의존성 정리
make tidy

# 포맷
make fmt

# 서버 실행(JSONL stdio)
make run-serve

# 초기화 1회 호출
make run-init
```

## 환경 변수

- `GO_VERSION` (기본: `1.24.0`)
  - 예: `GO_VERSION=1.24.0 make test`

## JSONL 요청 예시

```json
{"id":"1","method":"task.create","params":{"level":"case","title":"/api/users endpoint smoke test","priority":3}}
```

세션 시작(신규 작업):

```json
{"id":"2","method":"session.open","params":{"intent":"new_work","owner":"cayde","terminal_fingerprint":"tty-1"}}
```

재개 후보 조회:

```json
{"id":"3","method":"resume.candidates.list","params":{"requester_session_id":11}}
```

현재 작업 참조 조회:

```json
{"id":"4","method":"work.current_ref","params":{"session_id":11,"mode":"resume"}}
```

tmux 준비 + root thread 보장:

```json
{"id":"5","method":"runtime.tmux.ensure","params":{"session_id":11,"auto_install":true}}
{"id":"6","method":"thread.root.ensure","params":{"session_id":11,"ensure_tmux":true,"tmux_window_name":"root","initial_prompt":"You are the root orchestrator. Start by planning this request and delegating child threads.","launch_codex":true}}
```

응답의 `attach_info.attach_command` 또는 `child_attach_hint`를 사용자에게 안내해서 `tmux attach-session -t ...`로 진행상황/상호작용을 이어가게 합니다. 호출한 CLI는 root 세션 생성 후 즉시 반환할 수 있습니다.

child thread 생성:

```json
{"id":"7","method":"thread.child.spawn","params":{"session_id":11,"role":"main-worker","title":"api/users case","split_direction":"vertical","tmux_window_name":"children","max_concurrent_children":6,"launch_codex":true}}
```

merge reviewer 자동 스레드 생성:

```json
{"id":"8","method":"merge.review.request_auto","params":{"session_id":11,"merge_request_id":3,"reviewer_role":"merge-reviewer"}}
```
