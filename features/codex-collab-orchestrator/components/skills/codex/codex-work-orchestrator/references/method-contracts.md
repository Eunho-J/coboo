# codex-orchestrator method contracts (v2 root-local)

## Session and runtime

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

- `runtime.tmux.ensure`
  - input: optional `session_id`, optional `auto_install`
  - output: tmux runtime status

- `runtime.bundle.info`
  - input: none
  - output: installed bundle/agents/skills/mcp paths and verify command

## Task / planning

- `task.create`, `task.list`, `task.get`
- `graph.node.*`, `graph.edge.create`, `graph.checklist.upsert`, `graph.snapshot.create`
- `plan.bootstrap`, `plan.slice.generate`, `plan.slice.replan`, `plan.rollup.*`

Root-local policy:
- caller/root CLI executes planning directly in current session.

## Worktree / lock

- `scheduler.decide_worktree`
  - input: workload estimates
  - output: shared/worktree recommendation

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

## Thread APIs

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

## Case lifecycle

- `case.begin`
- `step.check`
- `case.complete`
- `work.current_ref`, `work.current_ref.ack`
- `resume.next`, `resume.candidates.list`, `resume.candidates.attach`

## Merge and review

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

## Mirror

- `mirror.status`
- `mirror.refresh` (restricted role: `doc-mirror-manager`)
