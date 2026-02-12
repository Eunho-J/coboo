# codex-orchestrator method contracts — Worker scope

This reference covers only the tools available to Child Worker agents: `orch_task`, `orch_lifecycle`, `orch_workspace`.

## orch_task — Case lifecycle

- `case.begin`
  - input: case task ID
  - output: case started

- `step.check`
  - input: step ID, status (pass/fail)
  - output: step checked

- `case.complete`
  - input: case task ID
  - output: case completed

- `resume.next`
  - input: session context
  - output: next unchecked step

- `task.get`
  - input: task ID
  - output: task details (for reading your assigned scope)

- `task.list`
  - input: optional filters
  - output: tasks within your scope

## orch_lifecycle — Work checkpoints

- `work.current_ref`
  - input: optional checkpoint data
  - output: current checkpoint reference
  - usage: Call after each step.check to save progress

- `work.current_ref.ack`
  - input: checkpoint ID
  - output: acknowledged
  - usage: Call on resume to confirm restart point

## orch_workspace — Worktree operations

- `worktree.merge_to_parent`
  - input: session_id, worktree_id
  - output: merge result
  - usage: Call after case.complete if using a child worktree

- `lock.acquire`
  - input: resource, scope
  - output: lock handle
  - usage: Acquire file/prefix lock before modifying shared resources

- `lock.heartbeat`
  - input: lock handle
  - output: extended lock
  - usage: Keep lock alive during long operations

- `lock.release`
  - input: lock handle
  - output: released
  - usage: Always release locks when done

> **Note**: Methods not listed here (orch_session, orch_thread, orch_merge, orch_graph, orch_system) are root-only. Do not attempt to call them.
