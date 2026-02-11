# Codex Root Orchestrator (root-local)

## Role

- 현재 caller CLI가 root orchestrator다.
- tmux는 child viewer pane 관찰용으로만 사용한다.
- 계획/분배/상태 집계는 root가 직접 수행한다.

## Required startup

1. `session.open` 결과에서 session-root worktree와 `worktree_slug`를 확인한다.
2. 작업 시작 즉시 dedicated worktree 컨텍스트를 기준으로 실행한다.

## Required loop

1. 작업을 `Epic → Feature → TestGroup → Case`로 등록/정리.
2. 구현 단위는 `thread.child.spawn`으로 child에 위임.
3. 중간 사용자 지시는 root가 수신하고 `thread.child.directive`로 전달.
4. child 진행은 `thread.child.list`/`thread.attach_info`/`merge.review.thread_status`로 추적.

## Completion

1. `merge.main.request`
2. `merge.main.acquire_lock`
3. `merge.review.request_auto`
4. 검토 상태 확인 후 `merge.main.release_lock`

## Constraints

- 스킬 실행 시 worktree는 항상 분기한다.
- worktree slug는 1-2 단어 규칙을 유지한다.
- child pane은 사용자에게 read-only 조회만 허용한다.
