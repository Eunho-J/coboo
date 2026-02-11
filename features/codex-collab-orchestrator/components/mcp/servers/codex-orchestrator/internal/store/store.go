package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	defaultLockTTLSeconds = 600
)

type Store struct {
	database *sql.DB
	dbPath   string
}

func Open(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite db: %w", err)
	}
	database.SetMaxOpenConns(1)

	store := &Store{
		database: database,
		dbPath:   dbPath,
	}
	if err := store.migrate(context.Background()); err != nil {
		_ = database.Close()
		return nil, err
	}

	return store, nil
}

func (store *Store) Close() error {
	return store.database.Close()
}

func (store *Store) DBPath() string {
	return store.dbPath
}

func (store *Store) migrate(ctx context.Context) error {
	statements := []string{
		`PRAGMA foreign_keys = ON;`,
		`CREATE TABLE IF NOT EXISTS tasks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			level TEXT NOT NULL,
			parent_id INTEGER NULL,
			title TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'todo',
			priority INTEGER NOT NULL DEFAULT 0,
			assignee_session TEXT NULL,
			input_contract TEXT NULL,
			fixtures TEXT NULL,
			next_action TEXT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS steps (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id INTEGER NOT NULL,
			title TEXT NOT NULL,
			status TEXT NOT NULL,
			evidence_json TEXT NOT NULL,
			order_no INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			FOREIGN KEY(task_id) REFERENCES tasks(id)
		);`,
		`CREATE TABLE IF NOT EXISTS checkpoints (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id INTEGER NOT NULL,
			step_title TEXT NOT NULL,
			snapshot_json TEXT NOT NULL,
			created_at TEXT NOT NULL,
			FOREIGN KEY(task_id) REFERENCES tasks(id)
		);`,
		`CREATE TABLE IF NOT EXISTS locks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			scope_type TEXT NOT NULL,
			scope_path TEXT NOT NULL,
			owner_session TEXT NOT NULL,
			lease_until TEXT NOT NULL,
			heartbeat_at TEXT NOT NULL,
			state TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS worktrees (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id INTEGER NOT NULL,
			path TEXT NOT NULL,
			branch TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			merged_at TEXT NULL
		);`,
		`ALTER TABLE worktrees ADD COLUMN kind TEXT NULL;`,
		`ALTER TABLE worktrees ADD COLUMN parent_worktree_id INTEGER NULL;`,
		`ALTER TABLE worktrees ADD COLUMN owner_session_id INTEGER NULL;`,
		`ALTER TABLE worktrees ADD COLUMN merge_state TEXT NULL;`,
		`CREATE TABLE IF NOT EXISTS merge_requests (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			feature_task_id INTEGER NOT NULL,
			status TEXT NOT NULL,
			reviewer_session TEXT NULL,
			notes_json TEXT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(feature_task_id) REFERENCES tasks(id)
		);`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			agent_role TEXT NOT NULL,
			owner TEXT NOT NULL,
			started_at TEXT NOT NULL,
			last_seen_at TEXT NOT NULL,
			status TEXT NOT NULL
		);`,
		`ALTER TABLE sessions ADD COLUMN repo_path TEXT NULL;`,
		`ALTER TABLE sessions ADD COLUMN terminal_fingerprint TEXT NULL;`,
		`ALTER TABLE sessions ADD COLUMN intent TEXT NULL;`,
		`ALTER TABLE sessions ADD COLUMN main_worktree_id INTEGER NULL;`,
		`ALTER TABLE sessions ADD COLUMN session_root_worktree_id INTEGER NULL;`,
		`ALTER TABLE sessions ADD COLUMN root_thread_id INTEGER NULL;`,
		`ALTER TABLE sessions ADD COLUMN tmux_session_name TEXT NULL;`,
		`ALTER TABLE sessions ADD COLUMN runtime_state TEXT NULL;`,
		`CREATE TABLE IF NOT EXISTS current_refs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id INTEGER NOT NULL,
			node_type TEXT NOT NULL,
			node_id INTEGER NOT NULL,
			checkpoint_id INTEGER NULL,
			mode TEXT NOT NULL,
			status TEXT NOT NULL,
			next_action TEXT NULL,
			summary TEXT NULL,
			required_files_json TEXT NULL,
			acked_at TEXT NULL,
			version INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS graph_nodes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			node_type TEXT NOT NULL,
			facet TEXT NOT NULL,
			title TEXT NOT NULL,
			status TEXT NOT NULL,
			priority INTEGER NOT NULL DEFAULT 0,
			parent_id INTEGER NULL,
			worktree_id INTEGER NULL,
			owner_session_id INTEGER NULL,
			summary TEXT NULL,
			risk_level INTEGER NULL,
			token_estimate INTEGER NULL,
			affected_files_json TEXT NULL,
			approval_state TEXT NOT NULL DEFAULT 'none',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS graph_edges (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			from_node_id INTEGER NOT NULL,
			to_node_id INTEGER NOT NULL,
			edge_type TEXT NOT NULL,
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS node_checklists (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			node_id INTEGER NOT NULL,
			item_text TEXT NOT NULL,
			status TEXT NOT NULL,
			order_no INTEGER NOT NULL,
			facet TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS node_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			node_id INTEGER NOT NULL,
			snapshot_type TEXT NOT NULL,
			summary TEXT NULL,
			affected_files_json TEXT NULL,
			next_action TEXT NULL,
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS planning_rules (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			max_token_per_slice INTEGER NOT NULL,
			max_files_per_slice INTEGER NOT NULL,
			replan_triggers_json TEXT NOT NULL,
			approval_policy TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`INSERT OR IGNORE INTO planning_rules(id, max_token_per_slice, max_files_per_slice, replan_triggers_json, approval_policy, updated_at)
		 VALUES(1, 18000, 12, '["context_overflow","scope_change","blocked"]', 'merge-agent-required', strftime('%Y-%m-%dT%H:%M:%fZ','now'));`,
		`CREATE TABLE IF NOT EXISTS session_handoffs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			from_session_id INTEGER NOT NULL,
			to_session_id INTEGER NOT NULL,
			state TEXT NOT NULL,
			created_at TEXT NOT NULL,
			completed_at TEXT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS merge_main_queue (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id INTEGER NOT NULL,
			from_worktree_id INTEGER NOT NULL,
			target_branch TEXT NOT NULL,
			state TEXT NOT NULL,
			started_at TEXT NULL,
			completed_at TEXT NULL,
			error_message TEXT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS merge_main_lock (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			holder_session_id INTEGER NULL,
			lease_until TEXT NULL,
			state TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`INSERT OR IGNORE INTO merge_main_lock(id, holder_session_id, lease_until, state, updated_at)
		 VALUES(1, NULL, NULL, 'unlocked', strftime('%Y-%m-%dT%H:%M:%fZ','now'));`,
		`CREATE TABLE IF NOT EXISTS threads (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id INTEGER NOT NULL,
			parent_thread_id INTEGER NULL,
			role TEXT NOT NULL,
			status TEXT NOT NULL,
			title TEXT NULL,
			objective TEXT NULL,
			worktree_id INTEGER NULL,
			agent_guide_path TEXT NULL,
			agent_override TEXT NULL,
			tmux_session_name TEXT NULL,
			tmux_window_name TEXT NULL,
			tmux_pane_id TEXT NULL,
			launch_command TEXT NULL,
			created_at TEXT NOT NULL,
			started_at TEXT NULL,
			completed_at TEXT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_threads_session_parent ON threads(session_id, parent_thread_id, id DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_threads_status ON threads(status);`,
		`CREATE TABLE IF NOT EXISTS review_jobs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			merge_request_id INTEGER NOT NULL,
			session_id INTEGER NOT NULL,
			reviewer_thread_id INTEGER NULL,
			state TEXT NOT NULL,
			notes_json TEXT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			completed_at TEXT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_review_jobs_merge_request ON review_jobs(merge_request_id, id DESC);`,
		`CREATE TABLE IF NOT EXISTS runtime_prereq_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id INTEGER NULL,
			requirement TEXT NOT NULL,
			status TEXT NOT NULL,
			detail TEXT NULL,
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS mirror_meta (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			db_version INTEGER NOT NULL DEFAULT 0,
			md_version INTEGER NOT NULL DEFAULT 0,
			md_path TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		);`,
		`INSERT OR IGNORE INTO mirror_meta(id, db_version, md_version, md_path, updated_at)
		 VALUES(1, 0, 0, '', strftime('%Y-%m-%dT%H:%M:%fZ','now'));`,
	}

	for _, statement := range statements {
		if _, err := store.database.ExecContext(ctx, statement); err != nil {
			if strings.Contains(statement, "ALTER TABLE") && strings.Contains(err.Error(), "duplicate column name") {
				continue
			}
			return fmt.Errorf("migration failed: %w", err)
		}
	}
	return nil
}

func (store *Store) CreateTask(ctx context.Context, args TaskCreateArgs) (Task, error) {
	if strings.TrimSpace(args.Level) == "" {
		return Task{}, errors.New("level is required")
	}
	if strings.TrimSpace(args.Title) == "" {
		return Task{}, errors.New("title is required")
	}

	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, err
	}
	defer transaction.Rollback()

	timestamp := nowTimestamp()
	result, err := transaction.ExecContext(
		ctx,
		`INSERT INTO tasks(level, parent_id, title, status, priority, assignee_session, created_at, updated_at)
		 VALUES(?, ?, ?, 'todo', ?, ?, ?, ?)`,
		strings.TrimSpace(args.Level),
		args.ParentID,
		args.Title,
		args.Priority,
		nullableText(args.AssigneeSession),
		timestamp,
		timestamp,
	)
	if err != nil {
		return Task{}, err
	}

	taskID, err := result.LastInsertId()
	if err != nil {
		return Task{}, err
	}

	if err := store.bumpVersionTx(ctx, transaction); err != nil {
		return Task{}, err
	}

	task, err := store.getTaskByIDTx(ctx, transaction, taskID)
	if err != nil {
		return Task{}, err
	}

	if err := transaction.Commit(); err != nil {
		return Task{}, err
	}
	return task, nil
}

func (store *Store) ListTasks(ctx context.Context, filter TaskFilter) ([]Task, error) {
	baseQuery := `SELECT id, level, parent_id, title, status, priority, assignee_session, input_contract, fixtures, next_action, created_at, updated_at
		FROM tasks`
	whereClauses := make([]string, 0, 3)
	parameters := make([]any, 0, 3)

	if strings.TrimSpace(filter.Level) != "" {
		whereClauses = append(whereClauses, "level = ?")
		parameters = append(parameters, filter.Level)
	}
	if strings.TrimSpace(filter.Status) != "" {
		whereClauses = append(whereClauses, "status = ?")
		parameters = append(parameters, filter.Status)
	}
	if filter.ParentID != nil {
		whereClauses = append(whereClauses, "parent_id = ?")
		parameters = append(parameters, *filter.ParentID)
	}

	if len(whereClauses) > 0 {
		baseQuery += " WHERE " + strings.Join(whereClauses, " AND ")
	}
	baseQuery += " ORDER BY priority DESC, updated_at ASC, id ASC"

	rows, err := store.database.QueryContext(ctx, baseQuery, parameters...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := make([]Task, 0)
	for rows.Next() {
		task, scanErr := scanTask(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		tasks = append(tasks, task)
	}

	return tasks, rows.Err()
}

func (store *Store) GetTaskByID(ctx context.Context, taskID int64) (Task, error) {
	row := store.database.QueryRowContext(
		ctx,
		`SELECT id, level, parent_id, title, status, priority, assignee_session, input_contract, fixtures, next_action, created_at, updated_at
		 FROM tasks WHERE id = ?`,
		taskID,
	)
	return scanTask(row)
}

func (store *Store) getTaskByIDTx(ctx context.Context, transaction *sql.Tx, taskID int64) (Task, error) {
	row := transaction.QueryRowContext(
		ctx,
		`SELECT id, level, parent_id, title, status, priority, assignee_session, input_contract, fixtures, next_action, created_at, updated_at
		 FROM tasks WHERE id = ?`,
		taskID,
	)
	return scanTask(row)
}

func (store *Store) BeginCase(ctx context.Context, args CaseBeginArgs) (Task, error) {
	inputContractText := "{}"
	if len(args.InputContract) > 0 {
		inputContractText = string(args.InputContract)
	}
	fixturesBytes, err := json.Marshal(args.Fixtures)
	if err != nil {
		return Task{}, err
	}

	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, err
	}
	defer transaction.Rollback()

	timestamp := nowTimestamp()
	result, err := transaction.ExecContext(
		ctx,
		`UPDATE tasks
		 SET status = 'in_progress',
		     input_contract = ?,
		     fixtures = ?,
		     updated_at = ?
		 WHERE id = ? AND level = 'case'`,
		inputContractText,
		string(fixturesBytes),
		timestamp,
		args.TaskID,
	)
	if err != nil {
		return Task{}, err
	}
	if changedRows, _ := result.RowsAffected(); changedRows == 0 {
		return Task{}, fmt.Errorf("case task not found: %d", args.TaskID)
	}

	snapshotBytes, err := json.Marshal(map[string]any{
		"input_contract": json.RawMessage(inputContractText),
		"fixtures":       args.Fixtures,
		"event":          "case.begin",
	})
	if err != nil {
		return Task{}, err
	}
	if err := store.insertCheckpointTx(ctx, transaction, args.TaskID, "case.begin", string(snapshotBytes)); err != nil {
		return Task{}, err
	}

	if err := store.bumpVersionTx(ctx, transaction); err != nil {
		return Task{}, err
	}

	task, err := store.getTaskByIDTx(ctx, transaction, args.TaskID)
	if err != nil {
		return Task{}, err
	}
	if err := transaction.Commit(); err != nil {
		return Task{}, err
	}
	return task, nil
}

func (store *Store) AddStepCheck(ctx context.Context, args StepCheckArgs) (Step, error) {
	if strings.TrimSpace(args.StepTitle) == "" {
		return Step{}, errors.New("step_title is required")
	}
	if strings.TrimSpace(args.Result) == "" {
		return Step{}, errors.New("result is required")
	}

	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return Step{}, err
	}
	defer transaction.Rollback()

	var nextOrderNo int64
	if err := transaction.QueryRowContext(ctx, "SELECT COALESCE(MAX(order_no), 0) + 1 FROM steps WHERE task_id = ?", args.TaskID).Scan(&nextOrderNo); err != nil {
		return Step{}, err
	}

	evidenceBytes, err := json.Marshal(map[string]any{
		"result":    args.Result,
		"artifacts": args.Artifacts,
	})
	if err != nil {
		return Step{}, err
	}

	timestamp := nowTimestamp()
	result, err := transaction.ExecContext(
		ctx,
		`INSERT INTO steps(task_id, title, status, evidence_json, order_no, created_at)
		 VALUES(?, ?, 'done', ?, ?, ?)`,
		args.TaskID,
		args.StepTitle,
		string(evidenceBytes),
		nextOrderNo,
		timestamp,
	)
	if err != nil {
		return Step{}, err
	}

	stepID, err := result.LastInsertId()
	if err != nil {
		return Step{}, err
	}

	snapshotBytes, err := json.Marshal(map[string]any{
		"step_title": args.StepTitle,
		"result":     args.Result,
		"artifacts":  args.Artifacts,
		"event":      "step.check",
	})
	if err != nil {
		return Step{}, err
	}
	if err := store.insertCheckpointTx(ctx, transaction, args.TaskID, args.StepTitle, string(snapshotBytes)); err != nil {
		return Step{}, err
	}

	if err := store.bumpVersionTx(ctx, transaction); err != nil {
		return Step{}, err
	}
	if err := transaction.Commit(); err != nil {
		return Step{}, err
	}

	return Step{
		ID:         stepID,
		TaskID:     args.TaskID,
		Title:      args.StepTitle,
		Status:     "done",
		Evidence:   string(evidenceBytes),
		OrderNo:    nextOrderNo,
		RecordedAt: timestamp,
	}, nil
}

func (store *Store) CompleteCase(ctx context.Context, args CaseCompleteArgs) (Task, error) {
	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, err
	}
	defer transaction.Rollback()

	timestamp := nowTimestamp()
	result, err := transaction.ExecContext(
		ctx,
		`UPDATE tasks
		 SET status = 'done',
		     next_action = ?,
		     updated_at = ?
		 WHERE id = ? AND level = 'case'`,
		nullableText(args.NextAction),
		timestamp,
		args.TaskID,
	)
	if err != nil {
		return Task{}, err
	}
	if changedRows, _ := result.RowsAffected(); changedRows == 0 {
		return Task{}, fmt.Errorf("case task not found: %d", args.TaskID)
	}

	snapshotBytes, err := json.Marshal(map[string]any{
		"summary":     args.Summary,
		"next_action": args.NextAction,
		"event":       "case.complete",
	})
	if err != nil {
		return Task{}, err
	}
	if err := store.insertCheckpointTx(ctx, transaction, args.TaskID, "case.complete", string(snapshotBytes)); err != nil {
		return Task{}, err
	}

	if err := store.bumpVersionTx(ctx, transaction); err != nil {
		return Task{}, err
	}

	task, err := store.getTaskByIDTx(ctx, transaction, args.TaskID)
	if err != nil {
		return Task{}, err
	}

	if err := transaction.Commit(); err != nil {
		return Task{}, err
	}
	return task, nil
}

func (store *Store) ResumeNextCase(ctx context.Context) (ResumeState, error) {
	row := store.database.QueryRowContext(
		ctx,
		`SELECT id, level, parent_id, title, status, priority, assignee_session, input_contract, fixtures, next_action, created_at, updated_at
		 FROM tasks
		 WHERE level = 'case' AND status IN ('in_progress', 'blocked', 'todo')
		 ORDER BY
		 	CASE status
				WHEN 'in_progress' THEN 0
				WHEN 'blocked' THEN 1
				ELSE 2
			END,
			priority DESC,
			updated_at ASC
		 LIMIT 1`,
	)

	task, err := scanTask(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ResumeState{}, nil
		}
		return ResumeState{}, err
	}

	checkpoint, err := store.GetLatestCheckpoint(ctx, task.ID)
	if err != nil {
		return ResumeState{}, err
	}
	return ResumeState{
		Task:       &task,
		Checkpoint: checkpoint,
	}, nil
}

func (store *Store) GetLatestCheckpoint(ctx context.Context, taskID int64) (*Checkpoint, error) {
	row := store.database.QueryRowContext(
		ctx,
		`SELECT id, task_id, step_title, snapshot_json, created_at
		 FROM checkpoints
		 WHERE task_id = ?
		 ORDER BY id DESC
		 LIMIT 1`,
		taskID,
	)

	checkpoint, err := scanCheckpoint(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &checkpoint, nil
}

func (store *Store) insertCheckpointTx(ctx context.Context, transaction *sql.Tx, taskID int64, stepTitle string, snapshotJSON string) error {
	_, err := transaction.ExecContext(
		ctx,
		`INSERT INTO checkpoints(task_id, step_title, snapshot_json, created_at)
		 VALUES(?, ?, ?, ?)`,
		taskID,
		stepTitle,
		snapshotJSON,
		nowTimestamp(),
	)
	return err
}

func (store *Store) AcquireLock(ctx context.Context, args LockAcquireArgs) (Lock, error) {
	scopeType := strings.TrimSpace(strings.ToLower(args.ScopeType))
	scopePath := normalizeScopePath(args.ScopePath)
	ownerSession := strings.TrimSpace(args.OwnerSession)

	if scopeType != "prefix" && scopeType != "file" {
		return Lock{}, errors.New("scope_type must be one of: prefix, file")
	}
	if scopePath == "" {
		return Lock{}, errors.New("scope_path is required")
	}
	if ownerSession == "" {
		return Lock{}, errors.New("owner_session is required")
	}
	if args.TTLSeconds <= 0 {
		args.TTLSeconds = defaultLockTTLSeconds
	}

	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return Lock{}, err
	}
	defer transaction.Rollback()

	now := nowTimestamp()
	_, err = transaction.ExecContext(
		ctx,
		`UPDATE locks
		 SET state = 'expired'
		 WHERE state = 'active'
		   AND lease_until < ?`,
		now,
	)
	if err != nil {
		return Lock{}, err
	}

	rows, err := transaction.QueryContext(
		ctx,
		`SELECT id, scope_type, scope_path, owner_session, lease_until, heartbeat_at, state
		 FROM locks
		 WHERE state = 'active'`,
	)
	if err != nil {
		return Lock{}, err
	}
	defer rows.Close()

	for rows.Next() {
		activeLock, scanErr := scanLock(rows)
		if scanErr != nil {
			return Lock{}, scanErr
		}
		if scopesConflict(scopeType, scopePath, activeLock.ScopeType, activeLock.ScopePath) {
			return Lock{}, fmt.Errorf("lock conflict with #%d (%s:%s)", activeLock.ID, activeLock.ScopeType, activeLock.ScopePath)
		}
	}
	if err := rows.Err(); err != nil {
		return Lock{}, err
	}

	leaseUntil := time.Now().UTC().Add(time.Duration(args.TTLSeconds) * time.Second).Format(time.RFC3339Nano)
	result, err := transaction.ExecContext(
		ctx,
		`INSERT INTO locks(scope_type, scope_path, owner_session, lease_until, heartbeat_at, state)
		 VALUES(?, ?, ?, ?, ?, 'active')`,
		scopeType,
		scopePath,
		ownerSession,
		leaseUntil,
		now,
	)
	if err != nil {
		return Lock{}, err
	}

	lockID, err := result.LastInsertId()
	if err != nil {
		return Lock{}, err
	}

	if err := store.bumpVersionTx(ctx, transaction); err != nil {
		return Lock{}, err
	}
	if err := transaction.Commit(); err != nil {
		return Lock{}, err
	}

	return Lock{
		ID:           lockID,
		ScopeType:    scopeType,
		ScopePath:    scopePath,
		OwnerSession: ownerSession,
		LeaseUntil:   leaseUntil,
		HeartbeatAt:  now,
		State:        "active",
	}, nil
}

func (store *Store) HeartbeatLock(ctx context.Context, lockID int64, ttlSeconds int) (Lock, error) {
	if ttlSeconds <= 0 {
		ttlSeconds = defaultLockTTLSeconds
	}

	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return Lock{}, err
	}
	defer transaction.Rollback()

	now := nowTimestamp()
	leaseUntil := time.Now().UTC().Add(time.Duration(ttlSeconds) * time.Second).Format(time.RFC3339Nano)
	result, err := transaction.ExecContext(
		ctx,
		`UPDATE locks
		 SET heartbeat_at = ?, lease_until = ?
		 WHERE id = ? AND state = 'active'`,
		now,
		leaseUntil,
		lockID,
	)
	if err != nil {
		return Lock{}, err
	}
	if changedRows, _ := result.RowsAffected(); changedRows == 0 {
		return Lock{}, fmt.Errorf("active lock not found: %d", lockID)
	}

	if err := store.bumpVersionTx(ctx, transaction); err != nil {
		return Lock{}, err
	}

	row := transaction.QueryRowContext(
		ctx,
		`SELECT id, scope_type, scope_path, owner_session, lease_until, heartbeat_at, state
		 FROM locks WHERE id = ?`,
		lockID,
	)
	lock, err := scanLock(row)
	if err != nil {
		return Lock{}, err
	}

	if err := transaction.Commit(); err != nil {
		return Lock{}, err
	}
	return lock, nil
}

func (store *Store) ReleaseLock(ctx context.Context, lockID int64) (Lock, error) {
	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return Lock{}, err
	}
	defer transaction.Rollback()

	now := nowTimestamp()
	result, err := transaction.ExecContext(
		ctx,
		`UPDATE locks
		 SET state = 'released',
		     heartbeat_at = ?,
		     lease_until = ?
		 WHERE id = ? AND state = 'active'`,
		now,
		now,
		lockID,
	)
	if err != nil {
		return Lock{}, err
	}
	if changedRows, _ := result.RowsAffected(); changedRows == 0 {
		return Lock{}, fmt.Errorf("active lock not found: %d", lockID)
	}

	if err := store.bumpVersionTx(ctx, transaction); err != nil {
		return Lock{}, err
	}

	row := transaction.QueryRowContext(
		ctx,
		`SELECT id, scope_type, scope_path, owner_session, lease_until, heartbeat_at, state
		 FROM locks WHERE id = ?`,
		lockID,
	)
	lock, err := scanLock(row)
	if err != nil {
		return Lock{}, err
	}

	if err := transaction.Commit(); err != nil {
		return Lock{}, err
	}
	return lock, nil
}

func (store *Store) ListActiveLocks(ctx context.Context) ([]Lock, error) {
	now := nowTimestamp()
	_, _ = store.database.ExecContext(
		ctx,
		`UPDATE locks
		 SET state = 'expired'
		 WHERE state = 'active'
		   AND lease_until < ?`,
		now,
	)

	rows, err := store.database.QueryContext(
		ctx,
		`SELECT id, scope_type, scope_path, owner_session, lease_until, heartbeat_at, state
		 FROM locks
		 WHERE state = 'active'
		 ORDER BY id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	locks := make([]Lock, 0)
	for rows.Next() {
		lock, scanErr := scanLock(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		locks = append(locks, lock)
	}
	return locks, rows.Err()
}

func (store *Store) CreateWorktreeRecord(ctx context.Context, args WorktreeCreateArgs) (Worktree, error) {
	if strings.TrimSpace(args.Path) == "" {
		return Worktree{}, errors.New("path is required")
	}
	if strings.TrimSpace(args.Branch) == "" {
		return Worktree{}, errors.New("branch is required")
	}
	if strings.TrimSpace(args.Status) == "" {
		args.Status = "planned"
	}
	if strings.TrimSpace(args.Kind) == "" {
		args.Kind = "task_branch"
	}
	if strings.TrimSpace(args.MergeState) == "" {
		args.MergeState = "active"
	}

	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return Worktree{}, err
	}
	defer transaction.Rollback()

	timestamp := nowTimestamp()
	result, err := transaction.ExecContext(
		ctx,
		`INSERT INTO worktrees(task_id, path, branch, status, kind, parent_worktree_id, owner_session_id, merge_state, created_at, merged_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
		args.TaskID,
		args.Path,
		args.Branch,
		args.Status,
		args.Kind,
		args.ParentWorktree,
		args.OwnerSessionID,
		args.MergeState,
		timestamp,
	)
	if err != nil {
		return Worktree{}, err
	}
	worktreeID, err := result.LastInsertId()
	if err != nil {
		return Worktree{}, err
	}
	if err := store.bumpVersionTx(ctx, transaction); err != nil {
		return Worktree{}, err
	}

	row := transaction.QueryRowContext(
		ctx,
		`SELECT id, task_id, path, branch, status, kind, parent_worktree_id, owner_session_id, merge_state, created_at, merged_at
		 FROM worktrees
		 WHERE id = ?`,
		worktreeID,
	)
	worktree, err := scanWorktree(row)
	if err != nil {
		return Worktree{}, err
	}
	if err := transaction.Commit(); err != nil {
		return Worktree{}, err
	}
	return worktree, nil
}

func (store *Store) ListWorktrees(ctx context.Context) ([]Worktree, error) {
	rows, err := store.database.QueryContext(
		ctx,
		`SELECT id, task_id, path, branch, status, kind, parent_worktree_id, owner_session_id, merge_state, created_at, merged_at
		 FROM worktrees
		 ORDER BY id DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	worktrees := make([]Worktree, 0)
	for rows.Next() {
		worktree, scanErr := scanWorktree(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		worktrees = append(worktrees, worktree)
	}
	return worktrees, rows.Err()
}

func (store *Store) CreateMergeRequest(ctx context.Context, args MergeRequestArgs) (MergeRequest, error) {
	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return MergeRequest{}, err
	}
	defer transaction.Rollback()

	timestamp := nowTimestamp()
	result, err := transaction.ExecContext(
		ctx,
		`INSERT INTO merge_requests(feature_task_id, status, reviewer_session, notes_json, created_at, updated_at)
		 VALUES(?, 'requested', ?, ?, ?, ?)`,
		args.FeatureTaskID,
		nullableText(args.ReviewerSession),
		nullableJSON(args.NotesJSON),
		timestamp,
		timestamp,
	)
	if err != nil {
		return MergeRequest{}, err
	}

	mergeRequestID, err := result.LastInsertId()
	if err != nil {
		return MergeRequest{}, err
	}

	if err := store.bumpVersionTx(ctx, transaction); err != nil {
		return MergeRequest{}, err
	}

	row := transaction.QueryRowContext(
		ctx,
		`SELECT id, feature_task_id, status, reviewer_session, notes_json, created_at, updated_at
		 FROM merge_requests WHERE id = ?`,
		mergeRequestID,
	)
	mergeRequest, err := scanMergeRequest(row)
	if err != nil {
		return MergeRequest{}, err
	}
	if err := transaction.Commit(); err != nil {
		return MergeRequest{}, err
	}
	return mergeRequest, nil
}

func (store *Store) GetMergeRequest(ctx context.Context, mergeRequestID int64) (MergeRequest, error) {
	row := store.database.QueryRowContext(
		ctx,
		`SELECT id, feature_task_id, status, reviewer_session, notes_json, created_at, updated_at
		 FROM merge_requests WHERE id = ?`,
		mergeRequestID,
	)
	return scanMergeRequest(row)
}

func (store *Store) GetMirrorStatus(ctx context.Context) (MirrorStatus, error) {
	row := store.database.QueryRowContext(
		ctx,
		`SELECT db_version, md_version, md_path, updated_at
		 FROM mirror_meta
		 WHERE id = 1`,
	)
	var status MirrorStatus
	if err := row.Scan(&status.DBVersion, &status.MDVersion, &status.MDPath, &status.UpdatedAt); err != nil {
		return MirrorStatus{}, err
	}
	status.Outdated = status.DBVersion != status.MDVersion
	return status, nil
}

func (store *Store) MarkMirrorRefreshed(ctx context.Context, mirrorPath string) (MirrorStatus, error) {
	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return MirrorStatus{}, err
	}
	defer transaction.Rollback()

	_, err = transaction.ExecContext(
		ctx,
		`UPDATE mirror_meta
		 SET md_version = db_version,
		     md_path = ?,
		     updated_at = ?
		 WHERE id = 1`,
		mirrorPath,
		nowTimestamp(),
	)
	if err != nil {
		return MirrorStatus{}, err
	}

	row := transaction.QueryRowContext(
		ctx,
		`SELECT db_version, md_version, md_path, updated_at
		 FROM mirror_meta
		 WHERE id = 1`,
	)
	var status MirrorStatus
	if err := row.Scan(&status.DBVersion, &status.MDVersion, &status.MDPath, &status.UpdatedAt); err != nil {
		return MirrorStatus{}, err
	}
	status.Outdated = status.DBVersion != status.MDVersion

	if err := transaction.Commit(); err != nil {
		return MirrorStatus{}, err
	}
	return status, nil
}

func (store *Store) GetTaskStatusCounts(ctx context.Context) (map[string]int64, error) {
	rows, err := store.database.QueryContext(
		ctx,
		`SELECT status, COUNT(*) FROM tasks GROUP BY status`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := map[string]int64{}
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[status] = count
	}
	return counts, rows.Err()
}

func (store *Store) bumpVersionTx(ctx context.Context, transaction *sql.Tx) error {
	_, err := transaction.ExecContext(
		ctx,
		`UPDATE mirror_meta
		 SET db_version = db_version + 1,
		     updated_at = ?
		 WHERE id = 1`,
		nowTimestamp(),
	)
	return err
}

func nowTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func nullableText(value string) any {
	trimmedValue := strings.TrimSpace(value)
	if trimmedValue == "" {
		return nil
	}
	return trimmedValue
}

func nullableJSON(value json.RawMessage) any {
	if len(value) == 0 || strings.TrimSpace(string(value)) == "" {
		return nil
	}
	return string(value)
}

func normalizeScopePath(path string) string {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return ""
	}
	cleanedPath := filepath.ToSlash(filepath.Clean(trimmedPath))
	return strings.TrimSuffix(cleanedPath, "/")
}

func scopesConflict(newScopeType string, newScopePath string, existingScopeType string, existingScopePath string) bool {
	switch newScopeType {
	case "file":
		switch existingScopeType {
		case "file":
			return samePath(newScopePath, existingScopePath)
		case "prefix":
			return hasPathPrefix(newScopePath, existingScopePath)
		}
	case "prefix":
		switch existingScopeType {
		case "file":
			return hasPathPrefix(existingScopePath, newScopePath)
		case "prefix":
			return hasPathPrefix(newScopePath, existingScopePath) || hasPathPrefix(existingScopePath, newScopePath)
		}
	}
	return false
}

func samePath(leftPath string, rightPath string) bool {
	return normalizeScopePath(leftPath) == normalizeScopePath(rightPath)
}

func hasPathPrefix(path string, prefix string) bool {
	normalizedPath := normalizeScopePath(path)
	normalizedPrefix := normalizeScopePath(prefix)
	if normalizedPrefix == "." || normalizedPrefix == "" {
		return true
	}
	if normalizedPath == normalizedPrefix {
		return true
	}
	return strings.HasPrefix(normalizedPath, normalizedPrefix+"/")
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanTask(scanner rowScanner) (Task, error) {
	var task Task
	var parentID sql.NullInt64
	var assigneeSession sql.NullString
	var inputContract sql.NullString
	var fixtures sql.NullString
	var nextAction sql.NullString

	err := scanner.Scan(
		&task.ID,
		&task.Level,
		&parentID,
		&task.Title,
		&task.Status,
		&task.Priority,
		&assigneeSession,
		&inputContract,
		&fixtures,
		&nextAction,
		&task.CreatedAt,
		&task.UpdatedAt,
	)
	if err != nil {
		return Task{}, err
	}

	if parentID.Valid {
		task.ParentID = &parentID.Int64
	}
	if assigneeSession.Valid {
		task.AssigneeSession = &assigneeSession.String
	}
	if inputContract.Valid {
		task.InputContract = &inputContract.String
	}
	if fixtures.Valid {
		task.Fixtures = &fixtures.String
	}
	if nextAction.Valid {
		task.NextAction = &nextAction.String
	}
	return task, nil
}

func scanCheckpoint(scanner rowScanner) (Checkpoint, error) {
	var checkpoint Checkpoint
	err := scanner.Scan(
		&checkpoint.ID,
		&checkpoint.TaskID,
		&checkpoint.StepTitle,
		&checkpoint.Snapshot,
		&checkpoint.RecordedAt,
	)
	if err != nil {
		return Checkpoint{}, err
	}
	return checkpoint, nil
}

func scanLock(scanner rowScanner) (Lock, error) {
	var lock Lock
	err := scanner.Scan(
		&lock.ID,
		&lock.ScopeType,
		&lock.ScopePath,
		&lock.OwnerSession,
		&lock.LeaseUntil,
		&lock.HeartbeatAt,
		&lock.State,
	)
	if err != nil {
		return Lock{}, err
	}
	return lock, nil
}

func scanWorktree(scanner rowScanner) (Worktree, error) {
	var worktree Worktree
	var kind sql.NullString
	var parentWorktree sql.NullInt64
	var ownerSessionID sql.NullInt64
	var mergeState sql.NullString
	var mergedAt sql.NullString
	err := scanner.Scan(
		&worktree.ID,
		&worktree.TaskID,
		&worktree.Path,
		&worktree.Branch,
		&worktree.Status,
		&kind,
		&parentWorktree,
		&ownerSessionID,
		&mergeState,
		&worktree.CreatedAt,
		&mergedAt,
	)
	if err != nil {
		return Worktree{}, err
	}
	if kind.Valid {
		worktree.Kind = &kind.String
	}
	if parentWorktree.Valid {
		worktree.ParentWorktree = &parentWorktree.Int64
	}
	if ownerSessionID.Valid {
		worktree.OwnerSessionID = &ownerSessionID.Int64
	}
	if mergeState.Valid {
		worktree.MergeState = &mergeState.String
	}
	if mergedAt.Valid {
		worktree.MergedAt = mergedAt.String
	}
	return worktree, nil
}

func scanMergeRequest(scanner rowScanner) (MergeRequest, error) {
	var mergeRequest MergeRequest
	var reviewerSession sql.NullString
	var notesJSON sql.NullString
	err := scanner.Scan(
		&mergeRequest.ID,
		&mergeRequest.FeatureTaskID,
		&mergeRequest.Status,
		&reviewerSession,
		&notesJSON,
		&mergeRequest.CreatedAt,
		&mergeRequest.UpdatedAt,
	)
	if err != nil {
		return MergeRequest{}, err
	}
	if reviewerSession.Valid {
		mergeRequest.ReviewerSession = &reviewerSession.String
	}
	if notesJSON.Valid {
		mergeRequest.NotesJSON = &notesJSON.String
	}
	return mergeRequest, nil
}
