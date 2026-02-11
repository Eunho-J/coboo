# Temporary Implementation Plan (Session-root + Resume + Merge Queue)

This temporary plan is created before implementation and used as the execution anchor.

## Scope

- Session-root worktree auto creation per terminal session
- Resume candidate listing and explicit attach flow
- Current-work fast reference endpoint for compact-safe resume
- Main merge queue + global merge lock
- Case lifecycle integration with current-ref updates

## Ordered tasks

1. Extend SQLite schema for sessions/worktrees/current_refs/merge queue
2. Add store-layer APIs for session lifecycle and resume attach
3. Add worktree spawn/merge-to-parent operations
4. Add main merge queue and lock APIs
5. Add work.current_ref and work.current_ref.ack APIs
6. Wire new methods in MCP service router
7. Update skill/agent docs to enforce new runtime workflow
8. Run tests and validate backward compatibility

## Notes

- Keep existing task/case/step methods available.
- Use session intent:
  - `new_work`: create new session-root automatically
  - `resume_work`: list candidates and require user selection before attach

