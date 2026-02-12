package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	defaultMainMergeLockTTLSeconds = 600
)

const sessionSelectColumns = `id, agent_role, owner, repo_path, terminal_fingerprint, intent, main_worktree_id, session_root_worktree_id, root_thread_id, tmux_session_name, runtime_state, delegation_state, delegation_root_thread_id, delegation_issued_at, delegation_acked_at, started_at, last_seen_at, status`

func (store *Store) OpenSession(ctx context.Context, args SessionOpenArgs) (Session, error) {
	agentRole := strings.TrimSpace(args.AgentRole)
	if agentRole == "" {
		agentRole = "codex"
	}
	owner := strings.TrimSpace(args.Owner)
	if owner == "" {
		owner = "unknown"
	}
	intent := strings.TrimSpace(args.Intent)
	if intent == "" {
		intent = "auto"
	}
	repoPath := strings.TrimSpace(args.RepoPath)
	if repoPath == "" {
		return Session{}, errors.New("repo_path is required")
	}

	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return Session{}, err
	}
	defer transaction.Rollback()

	now := nowTimestamp()
	result, err := transaction.ExecContext(
		ctx,
		`INSERT INTO sessions(agent_role, owner, started_at, last_seen_at, status, repo_path, terminal_fingerprint, intent, root_thread_id, tmux_session_name, runtime_state, delegation_state)
		 VALUES(?, ?, ?, ?, 'opened', ?, ?, ?, NULL, NULL, NULL, 'caller_active')`,
		agentRole,
		owner,
		now,
		now,
		repoPath,
		nullableText(args.TerminalFingerprint),
		intent,
	)
	if err != nil {
		return Session{}, err
	}
	sessionID, err := result.LastInsertId()
	if err != nil {
		return Session{}, err
	}

	if err := store.bumpVersionTx(ctx, transaction); err != nil {
		return Session{}, err
	}

	row := transaction.QueryRowContext(
		ctx,
		`SELECT `+sessionSelectColumns+`
		 FROM sessions WHERE id = ?`,
		sessionID,
	)
	session, err := scanSession(row)
	if err != nil {
		return Session{}, err
	}

	if err := transaction.Commit(); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (store *Store) HeartbeatSession(ctx context.Context, sessionID int64) (Session, error) {
	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return Session{}, err
	}
	defer transaction.Rollback()

	result, err := transaction.ExecContext(
		ctx,
		`UPDATE sessions
		 SET last_seen_at = ?, status = CASE WHEN status = 'opened' THEN 'active_new' ELSE status END
		 WHERE id = ?`,
		nowTimestamp(),
		sessionID,
	)
	if err != nil {
		return Session{}, err
	}
	if changedRows, _ := result.RowsAffected(); changedRows == 0 {
		return Session{}, fmt.Errorf("session not found: %d", sessionID)
	}

	if err := store.bumpVersionTx(ctx, transaction); err != nil {
		return Session{}, err
	}

	row := transaction.QueryRowContext(
		ctx,
		`SELECT `+sessionSelectColumns+`
		 FROM sessions WHERE id = ?`,
		sessionID,
	)
	session, err := scanSession(row)
	if err != nil {
		return Session{}, err
	}

	if err := transaction.Commit(); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (store *Store) CloseSession(ctx context.Context, sessionID int64) (Session, error) {
	return store.UpdateSession(ctx, sessionID, SessionUpdateArgs{
		Status: pointerToString("closed"),
	})
}

func (store *Store) UpdateSession(ctx context.Context, sessionID int64, args SessionUpdateArgs) (Session, error) {
	setClauses := make([]string, 0, 16)
	parameters := make([]any, 0, 24)

	if args.Status != nil {
		setClauses = append(setClauses, "status = ?")
		parameters = append(parameters, *args.Status)
	}
	if args.MainWorktreeID != nil {
		setClauses = append(setClauses, "main_worktree_id = ?")
		parameters = append(parameters, *args.MainWorktreeID)
	}
	if args.SessionRootWorktreeID != nil {
		setClauses = append(setClauses, "session_root_worktree_id = ?")
		parameters = append(parameters, *args.SessionRootWorktreeID)
	}
	if args.RootThreadID != nil {
		setClauses = append(setClauses, "root_thread_id = ?")
		parameters = append(parameters, *args.RootThreadID)
	}
	if args.TmuxSessionName != nil {
		setClauses = append(setClauses, "tmux_session_name = ?")
		parameters = append(parameters, *args.TmuxSessionName)
	}
	if args.RuntimeState != nil {
		setClauses = append(setClauses, "runtime_state = ?")
		parameters = append(parameters, *args.RuntimeState)
	}
	if args.Intent != nil {
		setClauses = append(setClauses, "intent = ?")
		parameters = append(parameters, *args.Intent)
	}
	if args.DelegationState != nil {
		setClauses = append(setClauses, "delegation_state = ?")
		parameters = append(parameters, *args.DelegationState)
	}
	if args.DelegationRootThreadID != nil {
		setClauses = append(setClauses, "delegation_root_thread_id = ?")
		parameters = append(parameters, *args.DelegationRootThreadID)
	}
	if args.DelegationIssuedAt != nil {
		setClauses = append(setClauses, "delegation_issued_at = ?")
		parameters = append(parameters, *args.DelegationIssuedAt)
	}
	if args.DelegationAckedAt != nil {
		setClauses = append(setClauses, "delegation_acked_at = ?")
		parameters = append(parameters, *args.DelegationAckedAt)
	}

	setClauses = append(setClauses, "last_seen_at = ?")
	parameters = append(parameters, nowTimestamp())
	parameters = append(parameters, sessionID)

	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return Session{}, err
	}
	defer transaction.Rollback()

	query := fmt.Sprintf("UPDATE sessions SET %s WHERE id = ?", strings.Join(setClauses, ", "))
	result, err := transaction.ExecContext(ctx, query, parameters...)
	if err != nil {
		return Session{}, err
	}
	if changedRows, _ := result.RowsAffected(); changedRows == 0 {
		return Session{}, fmt.Errorf("session not found: %d", sessionID)
	}

	if err := store.bumpVersionTx(ctx, transaction); err != nil {
		return Session{}, err
	}

	row := transaction.QueryRowContext(
		ctx,
		`SELECT `+sessionSelectColumns+`
		 FROM sessions WHERE id = ?`,
		sessionID,
	)
	session, err := scanSession(row)
	if err != nil {
		return Session{}, err
	}

	if err := transaction.Commit(); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (store *Store) GetSessionByID(ctx context.Context, sessionID int64) (Session, error) {
	row := store.database.QueryRowContext(
		ctx,
		`SELECT `+sessionSelectColumns+`
		 FROM sessions WHERE id = ?`,
		sessionID,
	)
	return scanSession(row)
}

func (store *Store) ListActiveSessions(ctx context.Context) ([]Session, error) {
	rows, err := store.database.QueryContext(ctx,
		`SELECT `+sessionSelectColumns+`
		   FROM sessions
		  WHERE status != 'closed'
		  ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sessions := make([]Session, 0)
	for rows.Next() {
		s, scanErr := scanSession(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

func (store *Store) GetPendingDelegationSession(ctx context.Context) (*Session, error) {
	row := store.database.QueryRowContext(
		ctx,
		`SELECT `+sessionSelectColumns+`
		 FROM sessions
		 WHERE LOWER(COALESCE(delegation_state, '')) = 'delegated'
		   AND (delegation_acked_at IS NULL OR TRIM(delegation_acked_at) = '')
		   AND status != 'closed'
		 ORDER BY id DESC
		 LIMIT 1`,
	)
	session, err := scanSession(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &session, nil
}

func (store *Store) CreateOrGetMainWorktree(ctx context.Context, repoPath string, branch string) (Worktree, error) {
	row := store.database.QueryRowContext(
		ctx,
		`SELECT id, task_id, path, branch, status, kind, parent_worktree_id, owner_session_id, merge_state, created_at, merged_at
		 FROM worktrees
		 WHERE kind = 'main' AND path = ?
		 ORDER BY id DESC
		 LIMIT 1`,
		repoPath,
	)
	worktree, err := scanWorktree(row)
	if err == nil {
		return worktree, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Worktree{}, err
	}

	return store.CreateWorktreeRecord(ctx, WorktreeCreateArgs{
		TaskID:     0,
		Path:       repoPath,
		Branch:     branch,
		Status:     "active",
		Kind:       "main",
		MergeState: "attached",
	})
}

func (store *Store) CreateSessionHandoff(ctx context.Context, fromSessionID int64, toSessionID int64, state string) error {
	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer transaction.Rollback()

	_, err = transaction.ExecContext(
		ctx,
		`INSERT INTO session_handoffs(from_session_id, to_session_id, state, created_at, completed_at)
		 VALUES(?, ?, ?, ?, NULL)`,
		fromSessionID,
		toSessionID,
		state,
		nowTimestamp(),
	)
	if err != nil {
		return err
	}

	if err := store.bumpVersionTx(ctx, transaction); err != nil {
		return err
	}
	return transaction.Commit()
}

func (store *Store) CompleteSessionHandoff(ctx context.Context, fromSessionID int64, toSessionID int64) error {
	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer transaction.Rollback()

	_, err = transaction.ExecContext(
		ctx,
		`UPDATE session_handoffs
		 SET state = 'completed', completed_at = ?
		 WHERE from_session_id = ? AND to_session_id = ? AND completed_at IS NULL`,
		nowTimestamp(),
		fromSessionID,
		toSessionID,
	)
	if err != nil {
		return err
	}

	if err := store.bumpVersionTx(ctx, transaction); err != nil {
		return err
	}
	return transaction.Commit()
}

func (store *Store) ListResumeCandidates(ctx context.Context, repoPath string, requesterSessionID int64, heartbeatTimeoutSeconds int) ([]ResumeCandidate, error) {
	if heartbeatTimeoutSeconds <= 0 {
		heartbeatTimeoutSeconds = 60
	}
	cutoff := time.Now().UTC().Add(-time.Duration(heartbeatTimeoutSeconds) * time.Second).Format(time.RFC3339Nano)

	rows, err := store.database.QueryContext(
		ctx,
		`SELECT `+sessionSelectColumns+`
		 FROM sessions
		 WHERE repo_path = ?
		   AND id != ?
		   AND status IN ('active_new', 'active_resume', 'handoff_attached')
		   AND last_seen_at < ?
		 ORDER BY last_seen_at ASC`,
		repoPath,
		requesterSessionID,
		cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	suspendedSessions := make([]Session, 0)
	for rows.Next() {
		session, scanErr := scanSession(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		suspendedSessions = append(suspendedSessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	candidates := make([]ResumeCandidate, 0, len(suspendedSessions))
	for _, session := range suspendedSessions {
		currentRef, refErr := store.GetCurrentRef(ctx, session.ID, true)
		if refErr != nil {
			return nil, refErr
		}
		if currentRef == nil || currentRef.Status != "active" {
			continue
		}

		var sessionRoot *Worktree
		if session.SessionRootWorktreeID != nil {
			worktree, rootErr := store.GetWorktreeByID(ctx, *session.SessionRootWorktreeID)
			if rootErr != nil {
				return nil, rootErr
			}
			sessionRoot = &worktree
		}

		candidates = append(candidates, ResumeCandidate{
			Session:     session,
			CurrentRef:  currentRef,
			SessionRoot: sessionRoot,
		})
	}

	return candidates, nil
}

func (store *Store) GetWorktreeByID(ctx context.Context, worktreeID int64) (Worktree, error) {
	row := store.database.QueryRowContext(
		ctx,
		`SELECT id, task_id, path, branch, status, kind, parent_worktree_id, owner_session_id, merge_state, created_at, merged_at
		 FROM worktrees WHERE id = ?`,
		worktreeID,
	)
	return scanWorktree(row)
}

func (store *Store) GetCurrentRef(ctx context.Context, sessionID int64, activeOnly bool) (*CurrentRef, error) {
	query := `SELECT id, session_id, node_type, node_id, checkpoint_id, mode, status, next_action, summary, required_files_json, acked_at, version, created_at, updated_at
		FROM current_refs
		WHERE session_id = ?`
	if activeOnly {
		query += " AND status = 'active'"
	}
	query += " ORDER BY updated_at DESC, id DESC LIMIT 1"

	row := store.database.QueryRowContext(ctx, query, sessionID)
	currentRef, err := scanCurrentRef(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &currentRef, nil
}

func (store *Store) UpsertCurrentRef(ctx context.Context, args WorkCurrentRefUpsertArgs) (CurrentRef, error) {
	if args.SessionID <= 0 {
		return CurrentRef{}, errors.New("session_id is required")
	}
	if strings.TrimSpace(args.NodeType) == "" {
		return CurrentRef{}, errors.New("node_type is required")
	}
	if args.NodeID <= 0 {
		return CurrentRef{}, errors.New("node_id is required")
	}
	mode := strings.TrimSpace(args.Mode)
	if mode == "" {
		mode = "compact"
	}
	status := strings.TrimSpace(args.Status)
	if status == "" {
		status = "active"
	}

	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return CurrentRef{}, err
	}
	defer transaction.Rollback()

	now := nowTimestamp()
	row := transaction.QueryRowContext(
		ctx,
		`SELECT id, version
		 FROM current_refs
		 WHERE session_id = ? AND status = 'active'
		 ORDER BY updated_at DESC, id DESC
		 LIMIT 1`,
		args.SessionID,
	)
	var refID int64
	var version int64
	queryErr := row.Scan(&refID, &version)
	switch {
	case queryErr == nil:
		_, err = transaction.ExecContext(
			ctx,
			`UPDATE current_refs
			 SET node_type = ?, node_id = ?, checkpoint_id = ?, mode = ?, status = ?, next_action = ?, summary = ?, required_files_json = ?, version = ?, updated_at = ?, acked_at = NULL
			 WHERE id = ?`,
			args.NodeType,
			args.NodeID,
			args.CheckpointID,
			mode,
			status,
			nullableText(args.NextAction),
			nullableText(args.Summary),
			nullableText(args.RequiredFilesJSON),
			version+1,
			now,
			refID,
		)
		if err != nil {
			return CurrentRef{}, err
		}
	case errors.Is(queryErr, sql.ErrNoRows):
		result, insertErr := transaction.ExecContext(
			ctx,
			`INSERT INTO current_refs(session_id, node_type, node_id, checkpoint_id, mode, status, next_action, summary, required_files_json, acked_at, version, created_at, updated_at)
			 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, 1, ?, ?)`,
			args.SessionID,
			args.NodeType,
			args.NodeID,
			args.CheckpointID,
			mode,
			status,
			nullableText(args.NextAction),
			nullableText(args.Summary),
			nullableText(args.RequiredFilesJSON),
			now,
			now,
		)
		if insertErr != nil {
			return CurrentRef{}, insertErr
		}
		refID, err = result.LastInsertId()
		if err != nil {
			return CurrentRef{}, err
		}
	default:
		return CurrentRef{}, queryErr
	}

	if err := store.bumpVersionTx(ctx, transaction); err != nil {
		return CurrentRef{}, err
	}

	refRow := transaction.QueryRowContext(
		ctx,
		`SELECT id, session_id, node_type, node_id, checkpoint_id, mode, status, next_action, summary, required_files_json, acked_at, version, created_at, updated_at
		 FROM current_refs
		 WHERE id = ?`,
		refID,
	)
	currentRef, err := scanCurrentRef(refRow)
	if err != nil {
		return CurrentRef{}, err
	}

	if err := transaction.Commit(); err != nil {
		return CurrentRef{}, err
	}
	return currentRef, nil
}

func (store *Store) AckCurrentRef(ctx context.Context, sessionID int64, refID int64) (CurrentRef, error) {
	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return CurrentRef{}, err
	}
	defer transaction.Rollback()

	result, err := transaction.ExecContext(
		ctx,
		`UPDATE current_refs
		 SET acked_at = ?, updated_at = ?, version = version + 1
		 WHERE id = ? AND session_id = ?`,
		nowTimestamp(),
		nowTimestamp(),
		refID,
		sessionID,
	)
	if err != nil {
		return CurrentRef{}, err
	}
	if changedRows, _ := result.RowsAffected(); changedRows == 0 {
		return CurrentRef{}, fmt.Errorf("current_ref not found: id=%d session=%d", refID, sessionID)
	}

	if err := store.bumpVersionTx(ctx, transaction); err != nil {
		return CurrentRef{}, err
	}

	row := transaction.QueryRowContext(
		ctx,
		`SELECT id, session_id, node_type, node_id, checkpoint_id, mode, status, next_action, summary, required_files_json, acked_at, version, created_at, updated_at
		 FROM current_refs
		 WHERE id = ?`,
		refID,
	)
	currentRef, err := scanCurrentRef(row)
	if err != nil {
		return CurrentRef{}, err
	}

	if err := transaction.Commit(); err != nil {
		return CurrentRef{}, err
	}
	return currentRef, nil
}

func (store *Store) AttachResumeCandidate(ctx context.Context, requesterSessionID int64, targetSessionID int64) (SessionContext, error) {
	targetSession, err := store.GetSessionByID(ctx, targetSessionID)
	if err != nil {
		return SessionContext{}, err
	}
	if targetSession.SessionRootWorktreeID == nil {
		return SessionContext{}, fmt.Errorf("target session has no session-root worktree: %d", targetSessionID)
	}

	requesterSession, err := store.GetSessionByID(ctx, requesterSessionID)
	if err != nil {
		return SessionContext{}, err
	}

	updateArgs := SessionUpdateArgs{
		Status:                pointerToString("active_resume"),
		MainWorktreeID:        targetSession.MainWorktreeID,
		SessionRootWorktreeID: targetSession.SessionRootWorktreeID,
		Intent:                pointerToString("resume_work"),
	}
	requesterSession, err = store.UpdateSession(ctx, requesterSession.ID, updateArgs)
	if err != nil {
		return SessionContext{}, err
	}

	_, err = store.UpdateSession(ctx, targetSessionID, SessionUpdateArgs{
		Status: pointerToString("handoff_attached"),
	})
	if err != nil {
		return SessionContext{}, err
	}

	if err := store.CreateSessionHandoff(ctx, targetSessionID, requesterSessionID, "attached"); err != nil {
		return SessionContext{}, err
	}

	targetRef, err := store.GetCurrentRef(ctx, targetSessionID, true)
	if err != nil {
		return SessionContext{}, err
	}
	var requesterRef *CurrentRef
	if targetRef != nil {
		upsertedRef, upsertErr := store.UpsertCurrentRef(ctx, WorkCurrentRefUpsertArgs{
			SessionID:         requesterSessionID,
			NodeType:          targetRef.NodeType,
			NodeID:            targetRef.NodeID,
			CheckpointID:      targetRef.CheckpointID,
			Mode:              "resume",
			Status:            "active",
			NextAction:        dereferenceOrEmpty(targetRef.NextAction),
			Summary:           dereferenceOrEmpty(targetRef.Summary),
			RequiredFilesJSON: dereferenceOrEmpty(targetRef.RequiredFilesJSON),
		})
		if upsertErr != nil {
			return SessionContext{}, upsertErr
		}
		requesterRef = &upsertedRef
	}

	var sessionRoot *Worktree
	if requesterSession.SessionRootWorktreeID != nil {
		worktree, rootErr := store.GetWorktreeByID(ctx, *requesterSession.SessionRootWorktreeID)
		if rootErr != nil {
			return SessionContext{}, rootErr
		}
		sessionRoot = &worktree
	}
	var mainWorktree *Worktree
	if requesterSession.MainWorktreeID != nil {
		worktree, mainErr := store.GetWorktreeByID(ctx, *requesterSession.MainWorktreeID)
		if mainErr != nil {
			return SessionContext{}, mainErr
		}
		mainWorktree = &worktree
	}

	return SessionContext{
		Session:      requesterSession,
		MainWorktree: mainWorktree,
		SessionRoot:  sessionRoot,
		CurrentRef:   requesterRef,
	}, nil
}

func (store *Store) EnqueueMainMergeRequest(ctx context.Context, args MainMergeRequestArgs) (MainMergeQueueItem, error) {
	if args.SessionID <= 0 {
		return MainMergeQueueItem{}, errors.New("session_id is required")
	}
	if args.FromWorktreeID <= 0 {
		return MainMergeQueueItem{}, errors.New("from_worktree_id is required")
	}
	targetBranch := strings.TrimSpace(args.TargetBranch)
	if targetBranch == "" {
		targetBranch = "main"
	}

	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return MainMergeQueueItem{}, err
	}
	defer transaction.Rollback()

	var unmergedChildren int64
	if err := transaction.QueryRowContext(
		ctx,
		`SELECT COUNT(*)
		 FROM worktrees
		 WHERE parent_worktree_id = ?
		   AND kind = 'task_branch'
		   AND COALESCE(merge_state, '') != 'merged_to_parent'`,
		args.FromWorktreeID,
	).Scan(&unmergedChildren); err != nil {
		return MainMergeQueueItem{}, err
	}
	if unmergedChildren > 0 {
		return MainMergeQueueItem{}, fmt.Errorf("session-root has %d unmerged child worktrees", unmergedChildren)
	}

	now := nowTimestamp()
	result, err := transaction.ExecContext(
		ctx,
		`INSERT INTO merge_main_queue(session_id, from_worktree_id, target_branch, state, started_at, completed_at, error_message, created_at, updated_at)
		 VALUES(?, ?, ?, 'queued', NULL, NULL, NULL, ?, ?)`,
		args.SessionID,
		args.FromWorktreeID,
		targetBranch,
		now,
		now,
	)
	if err != nil {
		return MainMergeQueueItem{}, err
	}
	requestID, err := result.LastInsertId()
	if err != nil {
		return MainMergeQueueItem{}, err
	}

	if err := store.bumpVersionTx(ctx, transaction); err != nil {
		return MainMergeQueueItem{}, err
	}

	row := transaction.QueryRowContext(
		ctx,
		`SELECT id, session_id, from_worktree_id, target_branch, state, started_at, completed_at, error_message, created_at, updated_at
		 FROM merge_main_queue
		 WHERE id = ?`,
		requestID,
	)
	queueItem, err := scanMainMergeQueueItem(row)
	if err != nil {
		return MainMergeQueueItem{}, err
	}

	if err := transaction.Commit(); err != nil {
		return MainMergeQueueItem{}, err
	}
	return queueItem, nil
}

func (store *Store) GetMainMergeRequest(ctx context.Context, requestID int64) (MainMergeQueueItem, error) {
	row := store.database.QueryRowContext(
		ctx,
		`SELECT id, session_id, from_worktree_id, target_branch, state, started_at, completed_at, error_message, created_at, updated_at
		 FROM merge_main_queue
		 WHERE id = ?`,
		requestID,
	)
	return scanMainMergeQueueItem(row)
}

func (store *Store) NextMainMergeRequest(ctx context.Context) (*MainMergeQueueItem, error) {
	row := store.database.QueryRowContext(
		ctx,
		`SELECT id, session_id, from_worktree_id, target_branch, state, started_at, completed_at, error_message, created_at, updated_at
		 FROM merge_main_queue
		 WHERE state = 'queued'
		 ORDER BY id ASC
		 LIMIT 1`,
	)
	item, err := scanMainMergeQueueItem(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (store *Store) AcquireMainMergeLock(ctx context.Context, sessionID int64, ttlSeconds int) (MainMergeLock, error) {
	if sessionID <= 0 {
		return MainMergeLock{}, errors.New("session_id is required")
	}
	if ttlSeconds <= 0 {
		ttlSeconds = defaultMainMergeLockTTLSeconds
	}

	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return MainMergeLock{}, err
	}
	defer transaction.Rollback()

	row := transaction.QueryRowContext(
		ctx,
		`SELECT id, holder_session_id, lease_until, state, updated_at
		 FROM merge_main_lock
		 WHERE id = 1`,
	)
	lock, err := scanMainMergeLock(row)
	if err != nil {
		return MainMergeLock{}, err
	}

	now := time.Now().UTC()
	locked := strings.EqualFold(lock.State, "locked")
	if locked && lock.LeaseUntil != nil {
		leaseTime, parseErr := time.Parse(time.RFC3339Nano, *lock.LeaseUntil)
		if parseErr == nil && leaseTime.After(now) {
			if lock.HolderSessionID == nil || *lock.HolderSessionID != sessionID {
				return MainMergeLock{}, fmt.Errorf("main merge lock held by session %d until %s", dereferenceInt64(lock.HolderSessionID), *lock.LeaseUntil)
			}
		}
	}

	leaseUntil := now.Add(time.Duration(ttlSeconds) * time.Second).Format(time.RFC3339Nano)
	_, err = transaction.ExecContext(
		ctx,
		`UPDATE merge_main_lock
		 SET holder_session_id = ?, lease_until = ?, state = 'locked', updated_at = ?
		 WHERE id = 1`,
		sessionID,
		leaseUntil,
		nowTimestamp(),
	)
	if err != nil {
		return MainMergeLock{}, err
	}

	if err := store.bumpVersionTx(ctx, transaction); err != nil {
		return MainMergeLock{}, err
	}

	row = transaction.QueryRowContext(
		ctx,
		`SELECT id, holder_session_id, lease_until, state, updated_at
		 FROM merge_main_lock
		 WHERE id = 1`,
	)
	lock, err = scanMainMergeLock(row)
	if err != nil {
		return MainMergeLock{}, err
	}
	if err := transaction.Commit(); err != nil {
		return MainMergeLock{}, err
	}
	return lock, nil
}

func (store *Store) ReleaseMainMergeLock(ctx context.Context, sessionID int64) (MainMergeLock, error) {
	if sessionID <= 0 {
		return MainMergeLock{}, errors.New("session_id is required")
	}

	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return MainMergeLock{}, err
	}
	defer transaction.Rollback()

	result, err := transaction.ExecContext(
		ctx,
		`UPDATE merge_main_lock
		 SET holder_session_id = NULL, lease_until = NULL, state = 'unlocked', updated_at = ?
		 WHERE id = 1
		   AND (holder_session_id = ? OR holder_session_id IS NULL)`,
		nowTimestamp(),
		sessionID,
	)
	if err != nil {
		return MainMergeLock{}, err
	}
	if changedRows, _ := result.RowsAffected(); changedRows == 0 {
		return MainMergeLock{}, errors.New("main merge lock is held by another session")
	}

	if err := store.bumpVersionTx(ctx, transaction); err != nil {
		return MainMergeLock{}, err
	}

	row := transaction.QueryRowContext(
		ctx,
		`SELECT id, holder_session_id, lease_until, state, updated_at
		 FROM merge_main_lock
		 WHERE id = 1`,
	)
	lock, err := scanMainMergeLock(row)
	if err != nil {
		return MainMergeLock{}, err
	}
	if err := transaction.Commit(); err != nil {
		return MainMergeLock{}, err
	}
	return lock, nil
}

func (store *Store) MarkWorktreeMergedToParent(ctx context.Context, worktreeID int64) (Worktree, error) {
	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return Worktree{}, err
	}
	defer transaction.Rollback()

	result, err := transaction.ExecContext(
		ctx,
		`UPDATE worktrees
		 SET merge_state = 'merged_to_parent',
		     status = 'closed',
		     merged_at = ?
		 WHERE id = ?`,
		nowTimestamp(),
		worktreeID,
	)
	if err != nil {
		return Worktree{}, err
	}
	if changedRows, _ := result.RowsAffected(); changedRows == 0 {
		return Worktree{}, fmt.Errorf("worktree not found: %d", worktreeID)
	}

	if err := store.bumpVersionTx(ctx, transaction); err != nil {
		return Worktree{}, err
	}

	row := transaction.QueryRowContext(
		ctx,
		`SELECT id, task_id, path, branch, status, kind, parent_worktree_id, owner_session_id, merge_state, created_at, merged_at
		 FROM worktrees WHERE id = ?`,
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

func (store *Store) BuildSessionContext(ctx context.Context, sessionID int64) (SessionContext, error) {
	session, err := store.GetSessionByID(ctx, sessionID)
	if err != nil {
		return SessionContext{}, err
	}

	var mainWorktree *Worktree
	if session.MainWorktreeID != nil {
		worktree, mainErr := store.GetWorktreeByID(ctx, *session.MainWorktreeID)
		if mainErr != nil {
			return SessionContext{}, mainErr
		}
		mainWorktree = &worktree
	}

	var sessionRoot *Worktree
	if session.SessionRootWorktreeID != nil {
		worktree, rootErr := store.GetWorktreeByID(ctx, *session.SessionRootWorktreeID)
		if rootErr != nil {
			return SessionContext{}, rootErr
		}
		sessionRoot = &worktree
	}

	currentRef, err := store.GetCurrentRef(ctx, sessionID, true)
	if err != nil {
		return SessionContext{}, err
	}

	return SessionContext{
		Session:      session,
		MainWorktree: mainWorktree,
		SessionRoot:  sessionRoot,
		CurrentRef:   currentRef,
	}, nil
}

func scanSession(scanner rowScanner) (Session, error) {
	var session Session
	var repoPath sql.NullString
	var terminalFingerprint sql.NullString
	var intent sql.NullString
	var mainWorktreeID sql.NullInt64
	var sessionRootWorktreeID sql.NullInt64
	var rootThreadID sql.NullInt64
	var tmuxSessionName sql.NullString
	var runtimeState sql.NullString
	var delegationState sql.NullString
	var delegationRootThreadID sql.NullInt64
	var delegationIssuedAt sql.NullString
	var delegationAckedAt sql.NullString
	err := scanner.Scan(
		&session.ID,
		&session.AgentRole,
		&session.Owner,
		&repoPath,
		&terminalFingerprint,
		&intent,
		&mainWorktreeID,
		&sessionRootWorktreeID,
		&rootThreadID,
		&tmuxSessionName,
		&runtimeState,
		&delegationState,
		&delegationRootThreadID,
		&delegationIssuedAt,
		&delegationAckedAt,
		&session.StartedAt,
		&session.LastSeenAt,
		&session.Status,
	)
	if err != nil {
		return Session{}, err
	}

	if repoPath.Valid {
		session.RepoPath = &repoPath.String
	}
	if terminalFingerprint.Valid {
		session.TerminalFingerprint = &terminalFingerprint.String
	}
	if intent.Valid {
		session.Intent = &intent.String
	}
	if mainWorktreeID.Valid {
		session.MainWorktreeID = &mainWorktreeID.Int64
	}
	if sessionRootWorktreeID.Valid {
		session.SessionRootWorktreeID = &sessionRootWorktreeID.Int64
	}
	if rootThreadID.Valid {
		session.RootThreadID = &rootThreadID.Int64
	}
	if tmuxSessionName.Valid {
		session.TmuxSessionName = &tmuxSessionName.String
	}
	if runtimeState.Valid {
		session.RuntimeState = &runtimeState.String
	}
	if delegationState.Valid {
		session.DelegationState = &delegationState.String
	}
	if delegationRootThreadID.Valid {
		session.DelegationRootThreadID = &delegationRootThreadID.Int64
	}
	if delegationIssuedAt.Valid {
		session.DelegationIssuedAt = &delegationIssuedAt.String
	}
	if delegationAckedAt.Valid {
		session.DelegationAckedAt = &delegationAckedAt.String
	}
	return session, nil
}

func scanCurrentRef(scanner rowScanner) (CurrentRef, error) {
	var currentRef CurrentRef
	var checkpointID sql.NullInt64
	var nextAction sql.NullString
	var summary sql.NullString
	var requiredFilesJSON sql.NullString
	var ackedAt sql.NullString
	err := scanner.Scan(
		&currentRef.ID,
		&currentRef.SessionID,
		&currentRef.NodeType,
		&currentRef.NodeID,
		&checkpointID,
		&currentRef.Mode,
		&currentRef.Status,
		&nextAction,
		&summary,
		&requiredFilesJSON,
		&ackedAt,
		&currentRef.Version,
		&currentRef.CreatedAt,
		&currentRef.UpdatedAt,
	)
	if err != nil {
		return CurrentRef{}, err
	}
	if checkpointID.Valid {
		currentRef.CheckpointID = &checkpointID.Int64
	}
	if nextAction.Valid {
		currentRef.NextAction = &nextAction.String
	}
	if summary.Valid {
		currentRef.Summary = &summary.String
	}
	if requiredFilesJSON.Valid {
		currentRef.RequiredFilesJSON = &requiredFilesJSON.String
	}
	if ackedAt.Valid {
		currentRef.AckedAt = &ackedAt.String
	}
	return currentRef, nil
}

func scanMainMergeQueueItem(scanner rowScanner) (MainMergeQueueItem, error) {
	var item MainMergeQueueItem
	var startedAt sql.NullString
	var completedAt sql.NullString
	var errorMessage sql.NullString
	err := scanner.Scan(
		&item.ID,
		&item.SessionID,
		&item.FromWorktreeID,
		&item.TargetBranch,
		&item.State,
		&startedAt,
		&completedAt,
		&errorMessage,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return MainMergeQueueItem{}, err
	}
	if startedAt.Valid {
		item.StartedAt = &startedAt.String
	}
	if completedAt.Valid {
		item.CompletedAt = &completedAt.String
	}
	if errorMessage.Valid {
		item.ErrorMessage = &errorMessage.String
	}
	return item, nil
}

func scanMainMergeLock(scanner rowScanner) (MainMergeLock, error) {
	var lock MainMergeLock
	var holderSessionID sql.NullInt64
	var leaseUntil sql.NullString
	err := scanner.Scan(
		&lock.ID,
		&holderSessionID,
		&leaseUntil,
		&lock.State,
		&lock.UpdatedAt,
	)
	if err != nil {
		return MainMergeLock{}, err
	}
	if holderSessionID.Valid {
		lock.HolderSessionID = &holderSessionID.Int64
	}
	if leaseUntil.Valid {
		lock.LeaseUntil = &leaseUntil.String
	}
	return lock, nil
}

func pointerToString(value string) *string {
	return &value
}

func dereferenceOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func dereferenceInt64(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
