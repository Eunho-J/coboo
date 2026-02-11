# codex-orchestrator method contracts (v2 root-local)

## Tool Groups

Each method is accessible through its domain-specific tool or through the legacy `orchestrator.call` tool (backward compatible).

| Tool | Domain | Methods |
|------|--------|---------|
| `orch_session` | Session & workspace | workspace.init, session.open, session.heartbeat, session.close, session.context |
| `orch_task` | Task & case lifecycle | task.create, task.list, task.get, case.begin, step.check, case.complete, resume.next, resume.candidates.list, resume.candidates.attach |
| `orch_graph` | Planning graph | graph.node.create, graph.node.list, graph.edge.create, graph.checklist.upsert, graph.snapshot.create |
| `orch_workspace` | Worktree & lock | scheduler.decide_worktree, worktree.create, worktree.list, worktree.spawn, worktree.merge_to_parent, lock.acquire, lock.heartbeat, lock.release |
| `orch_thread` | Child threads | thread.child.spawn, thread.child.directive, thread.child.list, thread.child.interrupt, thread.child.stop, thread.attach_info |
| `orch_lifecycle` | Work checkpoints | work.current_ref, work.current_ref.ack |
| `orch_merge` | Merge & review | merge.request, merge.review_context, merge.review.request_auto, merge.review.thread_status, merge.main.request, merge.main.next, merge.main.status, merge.main.acquire_lock, merge.main.release_lock |
| `orch_system` | Runtime, mirror, plan | runtime.tmux.ensure, runtime.bundle.info, mirror.status, mirror.refresh, plan.bootstrap, plan.slice.generate, plan.slice.replan, plan.rollup.preview, plan.rollup.submit, plan.rollup.approve, plan.rollup.reject |

> **Backward compatibility**: All methods remain callable via the legacy `orchestrator.call` tool with a free-form `method` parameter. The `orch_*` tools add method validation and improved discoverability.

---

## orch_session — Session and workspace

- `workspace.init`
  - input: none
  - output: repo/db paths

- `session.open`
  - input:
    - optional `agent_role`, `owner`, `terminal_fingerprint`
    - optional `intent(new_work|resume_work|auto)`
    - optional `heartbeat_timeout_seconds`
    - optional `user_request`
    - optional `worktree_name`
    - optional `always_branch` (default `true`)
  - behavior:
    - `intent=new_work|auto`: always create dedicated session-root worktree
    - worktree slug is normalized to 1-2 words
    - viewer tmux session name is `{repository}-{worktree}`
  - output:
    - `session_context`
    - `root_mode=caller_cli`
    - `worktree_slug`
    - `viewer_tmux_session`
    - `child_attach_hint`

- `session.heartbeat`
  - input: `session_id`
  - output: updated heartbeat timestamp

- `session.close`
  - input: `session_id`
  - output: closed session status

- `session.context`
  - input: `session_id`
  - output: full session context with worktrees and current_ref

## orch_system — Runtime, mirror, and planning

- `runtime.tmux.ensure`
  - input: optional `session_id`, optional `auto_install`
  - output: tmux runtime status

- `runtime.bundle.info`
  - input: none
  - output: installed bundle/agents/skills/mcp paths and verify command

- `mirror.status`
  - input: none
  - output: mirror sync status

- `mirror.refresh` (restricted role: `doc-mirror-manager`)
  - input: none
  - output: refresh result

- `plan.bootstrap`
  - input: task/graph context
  - output: Initiative and Plan node hierarchy

- `plan.slice.generate`
  - input: plan node, affected files, token estimates
  - output: Slice nodes under Plan

- `plan.slice.replan`
  - input: slice node, reason
  - output: replan snapshot

- `plan.rollup.preview`
  - input: plan node
  - output: aggregated rollup data

- `plan.rollup.submit`
  - input: plan node, rollup data
  - output: rollup snapshot, approval_state=pending

- `plan.rollup.approve` / `plan.rollup.reject`
  - input: snapshot ID
  - output: updated approval state

## orch_task — Task and case lifecycle

- `task.create`, `task.list`, `task.get`

- `case.begin`
  - input: case task ID
  - output: case started

- `step.check`
  - input: step ID, status
  - output: step checked

- `case.complete`
  - input: case task ID
  - output: case completed

- `resume.next`
  - input: session context
  - output: next unchecked step

- `resume.candidates.list`, `resume.candidates.attach`

Root-local policy:
- caller/root CLI executes planning directly in current session.

## orch_graph — Planning graph

- `graph.node.create`, `graph.node.list`
- `graph.edge.create`
- `graph.checklist.upsert`
- `graph.snapshot.create`

## orch_workspace — Worktree and lock

- `scheduler.decide_worktree`
  - input: workload estimates
  - output: shared/worktree recommendation

- `worktree.create`, `worktree.list`

- `worktree.spawn`
  - input:
    - `session_id`, `parent_worktree_id`
    - optional `task_id`, `reason`, `slug`, `branch`, `path`, `base_ref`, `create_on_disk`
  - behavior:
    - if branch/path omitted, generate from `slug`
    - conflict-safe suffix allocation

- `worktree.merge_to_parent`
  - input: `session_id`, `worktree_id`
  - output: merge result

- `lock.acquire` / `lock.heartbeat` / `lock.release`

## orch_thread — Child thread management

- `thread.child.spawn`
  - input:
    - `session_id`
    - optional `parent_thread_id`, `worktree_id`
    - optional `role`, `title`, `objective`
    - optional `agent_guide_path`, `agent_override`
    - optional `launch_command`, `split_direction`
    - optional `ensure_tmux`, `auto_install`
    - optional `tmux_session_name`, `tmux_window_name`
    - optional `initial_prompt`, `codex_command`
    - optional `runner_kind` (default `agents_sdk_codex_mcp`)
    - optional `interaction_mode` (default `view_only`)
    - optional `launch_codex`, `max_concurrent_children`
    - optional `task_spec`, `scope_task_ids`, `scope_case_ids`, `scope_node_ids`
  - behavior:
    - spawns child pane in `{repository}-{worktree}` tmux session
    - default launch command uses Agents SDK runner wrapper
    - user attach info is read-only for child threads

- `thread.child.directive`
  - input: `thread_id`, `directive`, optional `mode(interrupt_patch|queue|restart)`
  - behavior:
    - default mode `interrupt_patch`
    - `queue`: inject directive without interrupt
    - `restart`: stop and respawn child with directive

- `thread.child.list`, `thread.child.interrupt`, `thread.child.stop`
- `thread.attach_info`

## orch_lifecycle — Work checkpoints

- `work.current_ref`, `work.current_ref.ack`

## orch_merge — Merge and review

- `merge.request`
- `merge.review_context`
- `merge.review.request_auto`
  - behavior:
    - acquires main merge lock before merge-agent dispatch
    - dispatches merge-review child thread
  - output includes `main_lock`
- `merge.review.thread_status`
- `merge.main.request`, `merge.main.next`, `merge.main.status`
- `merge.main.acquire_lock`, `merge.main.release_lock`
