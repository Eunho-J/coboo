---
name: codestrator
description: Coordinate parallel Codex sessions in a single repository with conflict-safe execution, hierarchical task checklists, compact-safe resume points, and selective context loading. Use when multiple Codex sessions or users work concurrently, when worktree vs shared-workspace isolation must be decided, when file/prefix lock control is required, or when case-level progress must survive context compaction and session restarts.
metadata:
  short-description: "Parallel Codex orchestration with lock, checkpoint, and worktree isolation"
---

# Codestrator

Use this skill to enforce a strict root-local orchestration loop:

1. Split work into `Epic → Feature → TestGroup → Case → Step`.
2. Use the current caller CLI as root orchestrator; tmux is child viewer pane only.
3. Always branch into a dedicated worktree when the skill starts.
4. Spawn child workers/reviewers in tmux panes (view-only for users).
5. Finish with lock-guarded merge review agent dispatch.

## Required Workflow

### 1) Initialize workspace and open root-local session
- Call `workspace.init`.
- Confirm repository path and db path.
- Call `session.open` with `intent=new_work` (or `auto`).
  - Include `user_request`.
  - Optionally include `worktree_name`.
  - Keep `always_branch=true` (default).
- `session.open` returns:
  - session context
  - generated `worktree_slug`
  - `viewer_tmux_session` using `{repository}-{worktree}`.

### 2) Always branch at skill start
- Start in the dedicated session-root worktree returned by `session.open` before any planning or implementation.
- Worktree naming policy:
  - 1-2 words
  - lowercase slug
  - task-representative terms
  - collision suffix handled automatically (`-2`, `-3`...).

### 3) Root-local orchestration only
- Root is this caller CLI.
- Root performs:
  - task planning (`task.create`, `plan.*`, `graph.*`)
  - case lifecycle control (`case.*`, `step.check`)
  - child dispatch (`thread.child.spawn`)
  - merge/review orchestration.

### 4) Child worker/reviewer management
- Spawn child via `thread.child.spawn`.
  - Default runtime: `runner_kind=agents_sdk_codex_mcp`.
  - Default user visibility: `interaction_mode=view_only`.
- Child tmux session naming: `{repository}-{worktree}`.
- Provide read-only attach hint to users:
  - `tmux attach -r -t <viewer-session>`.
- For mid-task changes:
  - Root receives user input.
  - Root sends updates with `thread.child.directive` using `mode=interrupt_patch` (default).
  - Use `restart` mode only when interruption cannot recover.

### 5) Case lifecycle
- Begin case: `case.begin`
- Check steps: `step.check` (repeat)
- Complete case: `case.complete`
- Update `work.current_ref` checkpoints as needed.

### 6) Completion and merge review
- After implementation completion:
  1. `merge.main.request`
  2. `merge.main.acquire_lock`
  3. `merge.review.request_auto` (merge agent via Agents runner)
  4. `merge.review.thread_status` to track
  5. `merge.main.release_lock` when review gate is complete
- Merge review dispatch runs while main merge lock is held.

### 7) Resume behavior
- On restart/compact:
  - call `work.current_ref`
  - continue only next unchecked step in scope.

## Non-Negotiable Rules

- Root orchestration always runs in current caller CLI.
- Every skill-triggered run starts in a dedicated worktree.
- Worktree slug is always 1-2 words.
- Child tmux viewing is read-only for users.
- Root owns all planning/dispatch decisions.
- One active Case per worker thread at a time.
- Merge review dispatch must run under main merge lock.

## Tool Groups

The orchestrator exposes 8 domain-specific MCP tools. Each tool constrains its `method` parameter to the group's allowed methods. The legacy `orchestrator.call` tool remains available for backward compatibility.

| Tool | Purpose |
|------|---------|
| `orch_session` | Session and workspace initialization |
| `orch_task` | Task CRUD and case lifecycle (begin/check/complete) |
| `orch_graph` | Planning graph nodes, edges, checklists, snapshots |
| `orch_workspace` | Worktree management, locks, isolation decisions |
| `orch_thread` | Child thread spawn, directive, lifecycle |
| `orch_lifecycle` | Work checkpoint references (current_ref) |
| `orch_merge` | Merge requests, review dispatch, main merge lock |
| `orch_system` | Runtime (tmux), mirror, planning operations |

## Minimal Call Sequence

```text
orch_session:   workspace.init
orch_session:   session.open (always_branch=true, user_request/worktree_name)
orch_task:      task.create (epic/feature/test_group/case)
orch_workspace: scheduler.decide_worktree (optional for nested splits)
orch_thread:    thread.child.spawn (runner_kind=agents_sdk_codex_mcp, interaction_mode=view_only)
orch_task:      case.begin
orch_task:      step.check (repeat)
orch_task:      case.complete
orch_merge:     merge.main.request
orch_merge:     merge.main.acquire_lock
orch_merge:     merge.review.request_auto
orch_merge:     merge.review.thread_status
orch_merge:     merge.main.release_lock
```

> **Backward compatibility**: All methods above can also be called via the legacy `orchestrator.call` tool with any method string.

## References

- Load `references/method-contracts.md` only when API payload shape is needed.
