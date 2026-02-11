# Temporary Implementation Plan (Thread Runtime + Nested Review + tmux Fallback)

This temporary plan is created before implementation and used as the execution anchor.

## Scope

- Session-root worktree + session-root thread 자동 보장
- tmux 런타임 확인/자동설치/수동 fallback
- child thread spawn/list/interrupt/stop + attach 정보 제공
- merge reviewer thread 자동 디스패치 및 상태 추적
- compact-safe current ref + resume attach 유지

## Ordered tasks

1. Extend SQLite schema for session runtime + threads + review jobs
2. Add store-layer CRUD for threads/review jobs/runtime events
3. Implement `runtime.tmux.ensure` with auto-install and fallback
4. Implement root-local orchestration + `thread.child.*`
5. Implement `merge.review.request_auto` and status query
6. Integrate optional auto review dispatch into `merge.main.request`
7. Update skill method contracts and server docs
8. Run test suite and validate backward compatibility

## Notes

- Keep existing task/case/step/worktree methods available.
- Use session intent:
  - `new_work`: create new session-root automatically
  - `resume_work`: list candidates and require user selection before attach
- Parent/child thread execution is tmux-backed; if tmux not installable, return manual instructions and keep DB state traceable.
