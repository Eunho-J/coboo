package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func (store *Store) CreateThread(ctx context.Context, args ThreadCreateArgs) (Thread, error) {
	if args.SessionID <= 0 {
		return Thread{}, errors.New("session_id is required")
	}

	role := strings.TrimSpace(args.Role)
	if role == "" {
		role = "worker"
	}
	status := strings.TrimSpace(args.Status)
	if status == "" {
		status = "planned"
	}

	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return Thread{}, err
	}
	defer transaction.Rollback()

	now := nowTimestamp()
	var startedAt any = nil
	if strings.EqualFold(status, "running") {
		startedAt = now
	}

	result, err := transaction.ExecContext(
		ctx,
		`INSERT INTO threads(
			session_id, parent_thread_id, role, status, title, objective, worktree_id,
			agent_guide_path, agent_override, tmux_session_name, tmux_window_name, tmux_pane_id,
			launch_command, created_at, started_at, completed_at, updated_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, NULL, NULL, ?, ?, NULL, ?)`,
		args.SessionID,
		args.ParentThreadID,
		role,
		status,
		nullableText(args.Title),
		nullableText(args.Objective),
		args.WorktreeID,
		nullableText(args.AgentGuidePath),
		nullableText(args.AgentOverride),
		now,
		startedAt,
		now,
	)
	if err != nil {
		return Thread{}, err
	}
	threadID, err := result.LastInsertId()
	if err != nil {
		return Thread{}, err
	}

	if err := store.bumpVersionTx(ctx, transaction); err != nil {
		return Thread{}, err
	}

	row := transaction.QueryRowContext(
		ctx,
		`SELECT id, session_id, parent_thread_id, role, status, title, objective, worktree_id,
		        agent_guide_path, agent_override, tmux_session_name, tmux_window_name, tmux_pane_id,
		        launch_command, created_at, started_at, completed_at, updated_at
		   FROM threads
		  WHERE id = ?`,
		threadID,
	)
	thread, err := scanThread(row)
	if err != nil {
		return Thread{}, err
	}
	if err := transaction.Commit(); err != nil {
		return Thread{}, err
	}
	return thread, nil
}

func (store *Store) GetThreadByID(ctx context.Context, threadID int64) (Thread, error) {
	row := store.database.QueryRowContext(
		ctx,
		`SELECT id, session_id, parent_thread_id, role, status, title, objective, worktree_id,
		        agent_guide_path, agent_override, tmux_session_name, tmux_window_name, tmux_pane_id,
		        launch_command, created_at, started_at, completed_at, updated_at
		   FROM threads
		  WHERE id = ?`,
		threadID,
	)
	return scanThread(row)
}

func (store *Store) GetSessionRootThread(ctx context.Context, sessionID int64) (*Thread, error) {
	row := store.database.QueryRowContext(
		ctx,
		`SELECT id, session_id, parent_thread_id, role, status, title, objective, worktree_id,
		        agent_guide_path, agent_override, tmux_session_name, tmux_window_name, tmux_pane_id,
		        launch_command, created_at, started_at, completed_at, updated_at
		   FROM threads
		  WHERE session_id = ? AND parent_thread_id IS NULL
		  ORDER BY id DESC
		  LIMIT 1`,
		sessionID,
	)
	thread, err := scanThread(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &thread, nil
}

func (store *Store) ListThreads(ctx context.Context, filter ThreadFilter) ([]Thread, error) {
	query := strings.Builder{}
	query.WriteString(`SELECT id, session_id, parent_thread_id, role, status, title, objective, worktree_id,
		                      agent_guide_path, agent_override, tmux_session_name, tmux_window_name, tmux_pane_id,
		                      launch_command, created_at, started_at, completed_at, updated_at
		                 FROM threads
		                WHERE 1=1`)
	params := make([]any, 0, 4)

	if filter.SessionID > 0 {
		query.WriteString(" AND session_id = ?")
		params = append(params, filter.SessionID)
	}
	if filter.ParentThreadID != nil {
		query.WriteString(" AND parent_thread_id = ?")
		params = append(params, *filter.ParentThreadID)
	}
	if strings.TrimSpace(filter.Status) != "" {
		query.WriteString(" AND status = ?")
		params = append(params, strings.TrimSpace(filter.Status))
	}
	if strings.TrimSpace(filter.Role) != "" {
		query.WriteString(" AND role = ?")
		params = append(params, strings.TrimSpace(filter.Role))
	}
	query.WriteString(" ORDER BY id ASC")

	rows, err := store.database.QueryContext(ctx, query.String(), params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	threads := make([]Thread, 0)
	for rows.Next() {
		thread, scanErr := scanThread(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		threads = append(threads, thread)
	}
	return threads, rows.Err()
}

func (store *Store) UpdateThread(ctx context.Context, threadID int64, args ThreadUpdateArgs) (Thread, error) {
	setClauses := make([]string, 0, 8)
	params := make([]any, 0, 10)

	if args.Status != nil {
		status := strings.TrimSpace(*args.Status)
		if status == "" {
			return Thread{}, errors.New("status cannot be empty")
		}
		setClauses = append(setClauses, "status = ?")
		params = append(params, status)

		if strings.EqualFold(status, "running") {
			setClauses = append(setClauses, "started_at = COALESCE(started_at, ?)")
			params = append(params, nowTimestamp())
		}
		if isThreadTerminalStatus(status) {
			setClauses = append(setClauses, "completed_at = COALESCE(completed_at, ?)")
			params = append(params, nowTimestamp())
		}
	}
	if args.TmuxSessionName != nil {
		setClauses = append(setClauses, "tmux_session_name = ?")
		params = append(params, nullableText(*args.TmuxSessionName))
	}
	if args.TmuxWindowName != nil {
		setClauses = append(setClauses, "tmux_window_name = ?")
		params = append(params, nullableText(*args.TmuxWindowName))
	}
	if args.TmuxPaneID != nil {
		setClauses = append(setClauses, "tmux_pane_id = ?")
		params = append(params, nullableText(*args.TmuxPaneID))
	}
	if args.LaunchCommand != nil {
		setClauses = append(setClauses, "launch_command = ?")
		params = append(params, nullableText(*args.LaunchCommand))
	}
	if len(setClauses) == 0 {
		return store.GetThreadByID(ctx, threadID)
	}

	setClauses = append(setClauses, "updated_at = ?")
	params = append(params, nowTimestamp(), threadID)

	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return Thread{}, err
	}
	defer transaction.Rollback()

	query := fmt.Sprintf("UPDATE threads SET %s WHERE id = ?", strings.Join(setClauses, ", "))
	result, err := transaction.ExecContext(ctx, query, params...)
	if err != nil {
		return Thread{}, err
	}
	if changedRows, _ := result.RowsAffected(); changedRows == 0 {
		return Thread{}, fmt.Errorf("thread not found: %d", threadID)
	}

	if err := store.bumpVersionTx(ctx, transaction); err != nil {
		return Thread{}, err
	}

	row := transaction.QueryRowContext(
		ctx,
		`SELECT id, session_id, parent_thread_id, role, status, title, objective, worktree_id,
		        agent_guide_path, agent_override, tmux_session_name, tmux_window_name, tmux_pane_id,
		        launch_command, created_at, started_at, completed_at, updated_at
		   FROM threads
		  WHERE id = ?`,
		threadID,
	)
	thread, err := scanThread(row)
	if err != nil {
		return Thread{}, err
	}
	if err := transaction.Commit(); err != nil {
		return Thread{}, err
	}
	return thread, nil
}

func (store *Store) CreateReviewJob(ctx context.Context, args ReviewJobCreateArgs) (ReviewJob, error) {
	if args.MergeRequestID <= 0 {
		return ReviewJob{}, errors.New("merge_request_id is required")
	}
	if args.SessionID <= 0 {
		return ReviewJob{}, errors.New("session_id is required")
	}

	state := strings.TrimSpace(args.State)
	if state == "" {
		state = "requested"
	}

	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return ReviewJob{}, err
	}
	defer transaction.Rollback()

	now := nowTimestamp()
	result, err := transaction.ExecContext(
		ctx,
		`INSERT INTO review_jobs(merge_request_id, session_id, reviewer_thread_id, state, notes_json, created_at, updated_at, completed_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, NULL)`,
		args.MergeRequestID,
		args.SessionID,
		args.ReviewerThreadID,
		state,
		nullableJSON(args.NotesJSON),
		now,
		now,
	)
	if err != nil {
		return ReviewJob{}, err
	}
	reviewJobID, err := result.LastInsertId()
	if err != nil {
		return ReviewJob{}, err
	}

	if err := store.bumpVersionTx(ctx, transaction); err != nil {
		return ReviewJob{}, err
	}

	row := transaction.QueryRowContext(
		ctx,
		`SELECT id, merge_request_id, session_id, reviewer_thread_id, state, notes_json, created_at, updated_at, completed_at
		   FROM review_jobs
		  WHERE id = ?`,
		reviewJobID,
	)
	reviewJob, err := scanReviewJob(row)
	if err != nil {
		return ReviewJob{}, err
	}
	if err := transaction.Commit(); err != nil {
		return ReviewJob{}, err
	}
	return reviewJob, nil
}

func (store *Store) UpdateReviewJob(ctx context.Context, reviewJobID int64, args ReviewJobUpdateArgs) (ReviewJob, error) {
	setClauses := make([]string, 0, 5)
	params := make([]any, 0, 7)

	if args.State != nil {
		state := strings.TrimSpace(*args.State)
		if state == "" {
			return ReviewJob{}, errors.New("state cannot be empty")
		}
		setClauses = append(setClauses, "state = ?")
		params = append(params, state)
		if strings.EqualFold(state, "completed") || strings.EqualFold(state, "failed") || strings.EqualFold(state, "cancelled") {
			setClauses = append(setClauses, "completed_at = COALESCE(completed_at, ?)")
			params = append(params, nowTimestamp())
		}
	}
	if args.ReviewerThreadID != nil {
		setClauses = append(setClauses, "reviewer_thread_id = ?")
		params = append(params, *args.ReviewerThreadID)
	}
	if args.NotesJSON != nil {
		setClauses = append(setClauses, "notes_json = ?")
		params = append(params, nullableJSON(*args.NotesJSON))
	}
	if len(setClauses) == 0 {
		return store.GetReviewJobByID(ctx, reviewJobID)
	}

	setClauses = append(setClauses, "updated_at = ?")
	params = append(params, nowTimestamp(), reviewJobID)

	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return ReviewJob{}, err
	}
	defer transaction.Rollback()

	query := fmt.Sprintf("UPDATE review_jobs SET %s WHERE id = ?", strings.Join(setClauses, ", "))
	result, err := transaction.ExecContext(ctx, query, params...)
	if err != nil {
		return ReviewJob{}, err
	}
	if changedRows, _ := result.RowsAffected(); changedRows == 0 {
		return ReviewJob{}, fmt.Errorf("review job not found: %d", reviewJobID)
	}

	if err := store.bumpVersionTx(ctx, transaction); err != nil {
		return ReviewJob{}, err
	}

	row := transaction.QueryRowContext(
		ctx,
		`SELECT id, merge_request_id, session_id, reviewer_thread_id, state, notes_json, created_at, updated_at, completed_at
		   FROM review_jobs
		  WHERE id = ?`,
		reviewJobID,
	)
	reviewJob, err := scanReviewJob(row)
	if err != nil {
		return ReviewJob{}, err
	}
	if err := transaction.Commit(); err != nil {
		return ReviewJob{}, err
	}
	return reviewJob, nil
}

func (store *Store) GetReviewJobByID(ctx context.Context, reviewJobID int64) (ReviewJob, error) {
	row := store.database.QueryRowContext(
		ctx,
		`SELECT id, merge_request_id, session_id, reviewer_thread_id, state, notes_json, created_at, updated_at, completed_at
		   FROM review_jobs
		  WHERE id = ?`,
		reviewJobID,
	)
	return scanReviewJob(row)
}

func (store *Store) GetLatestReviewJobByMergeRequest(ctx context.Context, mergeRequestID int64) (*ReviewJob, error) {
	row := store.database.QueryRowContext(
		ctx,
		`SELECT id, merge_request_id, session_id, reviewer_thread_id, state, notes_json, created_at, updated_at, completed_at
		   FROM review_jobs
		  WHERE merge_request_id = ?
		  ORDER BY id DESC
		  LIMIT 1`,
		mergeRequestID,
	)
	reviewJob, err := scanReviewJob(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &reviewJob, nil
}

func (store *Store) RecordRuntimePrereqEvent(ctx context.Context, args RuntimePrereqEventCreateArgs) (RuntimePrereqEvent, error) {
	requirement := strings.TrimSpace(args.Requirement)
	if requirement == "" {
		return RuntimePrereqEvent{}, errors.New("requirement is required")
	}
	status := strings.TrimSpace(args.Status)
	if status == "" {
		return RuntimePrereqEvent{}, errors.New("status is required")
	}

	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return RuntimePrereqEvent{}, err
	}
	defer transaction.Rollback()

	result, err := transaction.ExecContext(
		ctx,
		`INSERT INTO runtime_prereq_events(session_id, requirement, status, detail, created_at)
		 VALUES(?, ?, ?, ?, ?)`,
		args.SessionID,
		requirement,
		status,
		nullableText(args.Detail),
		nowTimestamp(),
	)
	if err != nil {
		return RuntimePrereqEvent{}, err
	}
	eventID, err := result.LastInsertId()
	if err != nil {
		return RuntimePrereqEvent{}, err
	}

	if err := store.bumpVersionTx(ctx, transaction); err != nil {
		return RuntimePrereqEvent{}, err
	}

	row := transaction.QueryRowContext(
		ctx,
		`SELECT id, session_id, requirement, status, detail, created_at
		   FROM runtime_prereq_events
		  WHERE id = ?`,
		eventID,
	)
	event, err := scanRuntimePrereqEvent(row)
	if err != nil {
		return RuntimePrereqEvent{}, err
	}
	if err := transaction.Commit(); err != nil {
		return RuntimePrereqEvent{}, err
	}
	return event, nil
}

func scanThread(scanner rowScanner) (Thread, error) {
	var thread Thread
	var parentThreadID sql.NullInt64
	var title sql.NullString
	var objective sql.NullString
	var worktreeID sql.NullInt64
	var agentGuidePath sql.NullString
	var agentOverride sql.NullString
	var tmuxSessionName sql.NullString
	var tmuxWindowName sql.NullString
	var tmuxPaneID sql.NullString
	var launchCommand sql.NullString
	var startedAt sql.NullString
	var completedAt sql.NullString
	err := scanner.Scan(
		&thread.ID,
		&thread.SessionID,
		&parentThreadID,
		&thread.Role,
		&thread.Status,
		&title,
		&objective,
		&worktreeID,
		&agentGuidePath,
		&agentOverride,
		&tmuxSessionName,
		&tmuxWindowName,
		&tmuxPaneID,
		&launchCommand,
		&thread.CreatedAt,
		&startedAt,
		&completedAt,
		&thread.UpdatedAt,
	)
	if err != nil {
		return Thread{}, err
	}

	if parentThreadID.Valid {
		thread.ParentThreadID = &parentThreadID.Int64
	}
	if title.Valid {
		thread.Title = &title.String
	}
	if objective.Valid {
		thread.Objective = &objective.String
	}
	if worktreeID.Valid {
		thread.WorktreeID = &worktreeID.Int64
	}
	if agentGuidePath.Valid {
		thread.AgentGuidePath = &agentGuidePath.String
	}
	if agentOverride.Valid {
		thread.AgentOverride = &agentOverride.String
	}
	if tmuxSessionName.Valid {
		thread.TmuxSessionName = &tmuxSessionName.String
	}
	if tmuxWindowName.Valid {
		thread.TmuxWindowName = &tmuxWindowName.String
	}
	if tmuxPaneID.Valid {
		thread.TmuxPaneID = &tmuxPaneID.String
	}
	if launchCommand.Valid {
		thread.LaunchCommand = &launchCommand.String
	}
	if startedAt.Valid {
		thread.StartedAt = &startedAt.String
	}
	if completedAt.Valid {
		thread.CompletedAt = &completedAt.String
	}
	return thread, nil
}

func scanReviewJob(scanner rowScanner) (ReviewJob, error) {
	var reviewJob ReviewJob
	var reviewerThreadID sql.NullInt64
	var notesJSON sql.NullString
	var completedAt sql.NullString
	err := scanner.Scan(
		&reviewJob.ID,
		&reviewJob.MergeRequestID,
		&reviewJob.SessionID,
		&reviewerThreadID,
		&reviewJob.State,
		&notesJSON,
		&reviewJob.CreatedAt,
		&reviewJob.UpdatedAt,
		&completedAt,
	)
	if err != nil {
		return ReviewJob{}, err
	}
	if reviewerThreadID.Valid {
		reviewJob.ReviewerThreadID = &reviewerThreadID.Int64
	}
	if notesJSON.Valid {
		reviewJob.NotesJSON = &notesJSON.String
	}
	if completedAt.Valid {
		reviewJob.CompletedAt = &completedAt.String
	}
	return reviewJob, nil
}

func scanRuntimePrereqEvent(scanner rowScanner) (RuntimePrereqEvent, error) {
	var event RuntimePrereqEvent
	var sessionID sql.NullInt64
	var detail sql.NullString
	err := scanner.Scan(
		&event.ID,
		&sessionID,
		&event.Requirement,
		&event.Status,
		&detail,
		&event.CreatedAt,
	)
	if err != nil {
		return RuntimePrereqEvent{}, err
	}
	if sessionID.Valid {
		event.SessionID = &sessionID.Int64
	}
	if detail.Valid {
		event.Detail = &detail.String
	}
	return event, nil
}

func isThreadTerminalStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "failed", "stopped", "cancelled":
		return true
	default:
		return false
	}
}

func marshalRawJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	return string(raw)
}
