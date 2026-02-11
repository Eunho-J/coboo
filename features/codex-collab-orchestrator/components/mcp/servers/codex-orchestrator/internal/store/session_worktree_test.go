package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestMainMergeLockAcquireRelease(t *testing.T) {
	context := context.Background()
	store := openTestStore(t)
	defer store.Close()

	firstLock, err := store.AcquireMainMergeLock(context, 1, 30)
	if err != nil {
		t.Fatalf("failed to acquire first lock: %v", err)
	}
	if firstLock.HolderSessionID == nil || *firstLock.HolderSessionID != 1 {
		t.Fatalf("expected holder_session_id=1, got %+v", firstLock.HolderSessionID)
	}

	_, err = store.AcquireMainMergeLock(context, 2, 30)
	if err == nil {
		t.Fatalf("expected lock acquisition conflict for second session")
	}

	releasedLock, err := store.ReleaseMainMergeLock(context, 1)
	if err != nil {
		t.Fatalf("failed to release lock: %v", err)
	}
	if releasedLock.State != "unlocked" {
		t.Fatalf("expected unlocked state, got %s", releasedLock.State)
	}
}

func TestResumeCandidatesFromSuspendedSession(t *testing.T) {
	context := context.Background()
	store := openTestStore(t)
	defer store.Close()

	suspendedSession, err := store.OpenSession(context, SessionOpenArgs{
		AgentRole:           "codex",
		Owner:               "owner-a",
		RepoPath:            "/tmp/repo-a",
		TerminalFingerprint: "tty-a",
		Intent:              "new_work",
	})
	if err != nil {
		t.Fatalf("failed to open suspended session: %v", err)
	}
	requesterSession, err := store.OpenSession(context, SessionOpenArgs{
		AgentRole:           "codex",
		Owner:               "owner-b",
		RepoPath:            "/tmp/repo-a",
		TerminalFingerprint: "tty-b",
		Intent:              "resume_work",
	})
	if err != nil {
		t.Fatalf("failed to open requester session: %v", err)
	}

	mainWorktree, err := store.CreateOrGetMainWorktree(context, "/tmp/repo-a", "main")
	if err != nil {
		t.Fatalf("failed to create main worktree: %v", err)
	}
	sessionRoot, err := store.CreateWorktreeRecord(context, WorktreeCreateArgs{
		TaskID:         0,
		Path:           filepath.ToSlash("/tmp/repo-a/.codex-orch/worktrees/session-a"),
		Branch:         "session/a",
		Status:         "active",
		Kind:           "session_root",
		ParentWorktree: &mainWorktree.ID,
		OwnerSessionID: &suspendedSession.ID,
		MergeState:     "active",
	})
	if err != nil {
		t.Fatalf("failed to create session root: %v", err)
	}
	_, err = store.UpdateSession(context, suspendedSession.ID, SessionUpdateArgs{
		MainWorktreeID:        &mainWorktree.ID,
		SessionRootWorktreeID: &sessionRoot.ID,
		Status:                pointerToString("active_new"),
	})
	if err != nil {
		t.Fatalf("failed to update suspended session: %v", err)
	}

	_, err = store.UpsertCurrentRef(context, WorkCurrentRefUpsertArgs{
		SessionID:         suspendedSession.ID,
		NodeType:          "case",
		NodeID:            42,
		Mode:              "resume",
		Status:            "active",
		NextAction:        "continue",
		Summary:           "resume this case",
		RequiredFilesJSON: `["api/users.go"]`,
	})
	if err != nil {
		t.Fatalf("failed to upsert current ref: %v", err)
	}

	oldTimestamp := time.Now().UTC().Add(-3 * time.Minute).Format(time.RFC3339Nano)
	_, err = store.database.ExecContext(context, "UPDATE sessions SET last_seen_at = ? WHERE id = ?", oldTimestamp, suspendedSession.ID)
	if err != nil {
		t.Fatalf("failed to backdate suspended session heartbeat: %v", err)
	}

	candidates, err := store.ListResumeCandidates(context, "/tmp/repo-a", requesterSession.ID, 30)
	if err != nil {
		t.Fatalf("failed to list resume candidates: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].Session.ID != suspendedSession.ID {
		t.Fatalf("expected candidate session id %d, got %d", suspendedSession.ID, candidates[0].Session.ID)
	}
	if candidates[0].CurrentRef == nil || candidates[0].CurrentRef.NodeID != 42 {
		t.Fatalf("unexpected candidate current ref: %+v", candidates[0].CurrentRef)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()

	tempDir := t.TempDir()
	store, err := Open(filepath.Join(tempDir, "state.db"))
	if err != nil {
		t.Fatalf("failed to open test store: %v", err)
	}
	return store
}
