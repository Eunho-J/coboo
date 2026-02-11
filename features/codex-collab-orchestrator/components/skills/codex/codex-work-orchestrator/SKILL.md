---
name: codex-work-orchestrator
description: Coordinate parallel Codex sessions in a single repository with conflict-safe execution, hierarchical task checklists, compact-safe resume points, and selective context loading. Use when multiple Codex sessions or users work concurrently, when worktree vs shared-workspace isolation must be decided, when file/prefix lock control is required, or when case-level progress must survive context compaction and session restarts.
---

# Codex Work Orchestrator

Use this skill to enforce a strict execution loop for multi-session Codex work:

1. Split work into `Epic → Feature → TestGroup → Case → Step`.
2. Start every terminal session with a session-root worktree.
3. Execute one Case at a time and checkpoint after every Step.
4. Never depend on another Case output unless it is declared as fixture.
5. Resume from orchestrator state instead of re-reading broad docs.

## Required Workflow

### 1) Initialize workspace and open session
- Call `workspace.init`.
- Confirm repository path and db path.
- Open a session with `session.open`.
  - new work: `intent=new_work` (or `auto`)
  - resume request: `intent=resume_work`
- If resume was requested in a new session:
  - call `resume.candidates.list`
  - ask user which suspended session to attach
  - call `resume.candidates.attach`

### 2) Register work items before implementation
- Create tasks with `task.create` using strict levels.
- Recommended:
  - `epic` for large initiative
  - `feature` for deliverable slices
  - `test_group` for endpoint/domain group
  - `case` for one independent executable unit
- For variable planning, create planning graph nodes:
  - `plan.bootstrap`
  - `plan.slice.generate`
  - `plan.rollup.preview` / `plan.rollup.submit`

### 3) Decide branching before touching files
- Default execution root is session-root worktree.
- Call `scheduler.decide_worktree` with workload estimates.
- If branch split is needed, create child worktree via `worktree.spawn`.
- If shared file edits are used in the same worktree, guard via `lock.acquire`.

### 4) Execute Case lifecycle
- Start Case: `case.begin` with `session_id`, explicit `input_contract`, `fixtures`.
- Record each validation step: `step.check` with `session_id`.
- Complete Case immediately: `case.complete` with `session_id`.
- Release lock immediately when shared mode is used.

### 5) Resume after compact/session restart
- Call `work.current_ref` first.
- Load only that minimal ref context.
- If no active ref is returned, fallback to `resume.next`.
- Continue the next unchecked Step only.

### 6) Handle merge for parallel worktrees
- Merge child worktree back to session-root via `worktree.merge_to_parent`.
- Queue main branch merge via `merge.main.request`.
- Acquire global main-merge lock via `merge.main.acquire_lock`.
- Process queue one-by-one (`merge.main.next`, `merge.main.status`).
- Release main-merge lock via `merge.main.release_lock`.
- Use dedicated reviewer session to validate merge context.
- Read only related feature/case context, not full project docs.

### 7) Refresh Markdown mirror only on demand
- Use `mirror.status` for outdated detection.
- Request `mirror.refresh` with role `doc-mirror-manager` only when user asks for readable mirror docs.
- Keep worker sessions focused on DB-backed state operations.

## Non-Negotiable Rules

- Process exactly one active Case at a time per worker session.
- Use one session-root worktree per terminal session.
- Do not run a Case that requires outputs from another unfinished Case.
- Do not read unrelated docs during Case execution.
- Update state immediately after each meaningful action.
- Prefer short checkpoints over large narrative summaries.

## Minimal Call Sequence

```text
workspace.init
session.open
  -> resume.candidates.list/attach (if intent=resume_work)
task.create (epic/feature/test_group/case)
scheduler.decide_worktree
  -> worktree.spawn (if split needed)
  -> lock.acquire (if shared)
case.begin
step.check (repeat)
case.complete
work.current_ref (on compact/restart)
worktree.merge_to_parent (if child worktree)
merge.main.request
merge.main.acquire_lock/release_lock
lock.release (if acquired)
```

## References

- Load `references/method-contracts.md` only when API payload shape is needed.
