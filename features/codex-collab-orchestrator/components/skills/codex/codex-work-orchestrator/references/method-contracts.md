# codex-orchestrator method contracts (v1)

## Session and resume

- `session.open`
  - input: optional `agent_role`, optional `owner`, optional `terminal_fingerprint`, optional `intent(new_work|resume_work|auto)`, optional `heartbeat_timeout_seconds`
  - behavior:
    - `intent=new_work` or `auto`: create session-root worktree automatically
    - `intent=resume_work`: return suspended candidates for user selection
  - output: session context or `action_required=select_resume_candidate`
- `session.heartbeat`
  - input: `session_id`
  - output: updated session
- `session.close`
  - input: `session_id`
  - output: updated session
- `session.context`
  - input: `session_id`
  - output: session + main/session-root worktree + current_ref
- `resume.candidates.list`
  - input: `requester_session_id`, optional `heartbeat_timeout_seconds`
  - output: suspended candidates (heartbeat timeout + active current_ref)
- `resume.candidates.attach`
  - input: `requester_session_id`, `target_session_id`
  - output: attached session context
- `work.current_ref`
  - input: `session_id`, optional `mode(compact|resume|handoff)`, optional `required_files`
  - output: minimal active work reference for fast resume
- `work.current_ref.ack`
  - input: `session_id`, `ref_id`
  - output: acknowledged ref

## Task management

- `task.create`
  - input: `level`, `title`, optional `parent_id`, `priority`, `assignee_session`
  - output: created task row
- `task.list`
  - input: optional `level`, `status`, `parent_id`
  - output: filtered tasks
- `task.get`
  - input: `task_id`
  - output: one task

## Planning graph

- `graph.node.create`
  - input: `node_type`, `facet`, `title`, optional `status`, optional `priority`, optional `parent_id`, optional `worktree_id`, optional `owner_session_id`, optional `summary`, optional `risk_level`, optional `token_estimate`, optional `affected_files`, optional `approval_state`
  - output: created graph node
- `graph.node.list`
  - input: optional `node_type`, `facet`, `status`, `parent_id`
  - output: graph node list
- `graph.edge.create`
  - input: `from_node_id`, `to_node_id`, `edge_type`
  - output: created edge
- `graph.checklist.upsert`
  - input: `node_id`, `item_text`, optional `status`, optional `order_no`, optional `facet`
  - output: checklist item
- `graph.snapshot.create`
  - input: `node_id`, `snapshot_type`, optional `summary`, optional `affected_files`, optional `next_action`
  - output: snapshot
- `plan.bootstrap`
  - input: `initiative_title`, `plan_title`, optional `priority`, optional `owner_session_id`, optional `summary`
  - output: initiative + plan nodes
- `plan.slice.generate`
  - input: `plan_node_id`, optional `owner_session_id`, `slice_specs[]`
  - output: generated slices
- `plan.slice.replan`
  - input: `node_id`, `reason`, optional `affected_files`, optional `next_action`
  - output: replan snapshot
- `plan.rollup.preview`
  - input: `parent_node_id`
  - output: rollup preview
- `plan.rollup.submit`
  - input: `node_id`, optional `summary`, optional `affected_files`, optional `next_action`
  - output: pending rollup
- `plan.rollup.approve`
  - input: `node_id`
  - output: node with `approval_state=approved`
- `plan.rollup.reject`
  - input: `node_id`
  - output: node with `approval_state=rejected`

## Isolation and locking

- `scheduler.decide_worktree`
  - input: `changed_files`, `estimate_minutes`, `risk`, `parallel_workers`, `conflicting_paths`
  - output: `mode(shared|worktree)`, `score`, `reasons[]`
- `worktree.create`
  - input: `task_id`, `branch`, optional `path`, optional `base_ref`, optional `create_on_disk`
  - output: registered worktree record
- `worktree.spawn`
  - input: `session_id`, `parent_worktree_id`, optional `task_id`, optional `branch`, optional `path`, optional `base_ref`, optional `create_on_disk`
  - output: child task worktree
- `worktree.merge_to_parent`
  - input: `session_id`, `worktree_id`
  - output: merged child and parent worktree context
- `lock.acquire`
  - input: `scope_type(prefix|file)`, `scope_path`, `owner_session`, optional `ttl_seconds`
  - output: active lock
- `lock.heartbeat`
  - input: `lock_id`, optional `ttl_seconds`
  - output: updated lock
- `lock.release`
  - input: `lock_id`
  - output: released lock

## Case lifecycle

- `case.begin`
  - input: `case_id`, optional `session_id`, `input_contract`(json), `fixtures`(string[]), optional `required_files`
  - output: updated case task
- `step.check`
  - input: `case_id`, optional `session_id`, `step_title`, `result`, `artifacts`(string[]), optional `required_files`
  - output: recorded step
- `case.complete`
  - input: `case_id`, optional `session_id`, `summary`, `next_action`, optional `required_files`
  - output: updated case task
- `resume.next`
  - input: none
  - output: next case + latest checkpoint

## Merge and mirror

- `merge.request`
  - input: `feature_task_id`, optional `reviewer_session`, optional `notes_json`
  - output: merge request
- `merge.review_context`
  - input: `merge_request_id`
  - output: merge request + feature + child tasks + case checkpoints
- `merge.main.request`
  - input: `session_id`, `from_worktree_id`(session-root), optional `target_branch`
  - output: queued main-merge request
- `merge.main.next`
  - input: none
  - output: next queued main-merge request
- `merge.main.status`
  - input: `request_id`
  - output: main-merge request status
- `merge.main.acquire_lock`
  - input: `session_id`, optional `ttl_seconds`
  - output: merge lock state
- `merge.main.release_lock`
  - input: `session_id`
  - output: merge lock state
- `mirror.status`
  - input: none
  - output: `db_version`, `md_version`, `outdated`, `md_path`
- `mirror.refresh`
  - input: `requester_role` (must be `doc-mirror-manager`), optional `target_path`
  - output: refreshed mirror status and path
