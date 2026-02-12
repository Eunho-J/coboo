# codex-orchestrator method contracts — Reviewer scope

This reference covers only the tools available to Merge Reviewer agents: `orch_merge`, `orch_graph`, `orch_task`.

## orch_merge — Merge and review

- `merge.review_context`
  - input: optional merge request ID
  - output: target branch diff, affected files, related cases
  - usage: Primary context source for review

- `merge.review.thread_status`
  - input: review thread ID
  - output: review status (pending/in_progress/completed)
  - usage: Update your review status for root to read

## orch_graph — Planning graph (read)

- `graph.node.list`
  - input: optional filters
  - output: planning graph nodes
  - usage: Cross-reference implementation against plan

- `graph.checklist.upsert`
  - input: node ID, checklist items
  - output: updated checklist
  - usage: Record review findings against plan items

## orch_task — Task verification (read)

- `task.list`
  - input: optional filters
  - output: tasks and their completion status
  - usage: Verify all planned cases are completed

- `task.get`
  - input: task ID
  - output: task details
  - usage: Check individual case completion and acceptance criteria

> **Note**: Methods not listed here (orch_session, orch_thread, orch_workspace, orch_lifecycle, orch_system) are not available to reviewers. Do not attempt to call them.
