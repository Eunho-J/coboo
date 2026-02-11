package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestThreadLifecycle(t *testing.T) {
	context := context.Background()
	store := openThreadTestStore(t)
	defer store.Close()

	session, err := store.OpenSession(context, SessionOpenArgs{
		AgentRole: "codex",
		Owner:     "owner-a",
		RepoPath:  "/tmp/repo-a",
		Intent:    "new_work",
	})
	if err != nil {
		t.Fatalf("failed to open session: %v", err)
	}

	rootThread, err := store.CreateThread(context, ThreadCreateArgs{
		SessionID:        session.ID,
		Role:             "session-root",
		Status:           "planned",
		Title:            "session root",
		TaskSpecJSON:     `{"title":"session root","objective":"bootstrap orchestration"}`,
		ScopeTaskIDsJSON: `[1,2]`,
		ScopeCaseIDsJSON: `[11]`,
		ScopeNodeIDsJSON: `[101]`,
	})
	if err != nil {
		t.Fatalf("failed to create thread: %v", err)
	}
	if rootThread.ParentThreadID != nil {
		t.Fatalf("expected root thread parent=nil, got %+v", rootThread.ParentThreadID)
	}

	threads, err := store.ListThreads(context, ThreadFilter{
		SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("failed to list threads: %v", err)
	}
	if len(threads) != 1 {
		t.Fatalf("expected 1 thread, got %d", len(threads))
	}

	runningStatus := "running"
	tmuxSession := "codex-root-1"
	tmuxPane := "%1"
	updatedThread, err := store.UpdateThread(context, rootThread.ID, ThreadUpdateArgs{
		Status:          &runningStatus,
		TmuxSessionName: &tmuxSession,
		TmuxPaneID:      &tmuxPane,
	})
	if err != nil {
		t.Fatalf("failed to update thread: %v", err)
	}
	if updatedThread.Status != "running" {
		t.Fatalf("expected running status, got %s", updatedThread.Status)
	}
	if updatedThread.TmuxPaneID == nil || *updatedThread.TmuxPaneID != tmuxPane {
		t.Fatalf("expected tmux pane %s, got %+v", tmuxPane, updatedThread.TmuxPaneID)
	}
	if updatedThread.TaskSpecJSON == nil || *updatedThread.TaskSpecJSON == "" {
		t.Fatalf("expected persisted task_spec_json, got %+v", updatedThread.TaskSpecJSON)
	}
	if updatedThread.ScopeTaskIDsJSON == nil || *updatedThread.ScopeTaskIDsJSON != `[1,2]` {
		t.Fatalf("expected scope_task_ids_json [1,2], got %+v", updatedThread.ScopeTaskIDsJSON)
	}
}

func TestReviewJobLifecycle(t *testing.T) {
	context := context.Background()
	store := openThreadTestStore(t)
	defer store.Close()

	session, err := store.OpenSession(context, SessionOpenArgs{
		AgentRole: "codex",
		Owner:     "owner-a",
		RepoPath:  "/tmp/repo-a",
		Intent:    "new_work",
	})
	if err != nil {
		t.Fatalf("failed to open session: %v", err)
	}

	featureTask, err := store.CreateTask(context, TaskCreateArgs{
		Level:    "feature",
		Title:    "feature-a",
		Priority: 1,
	})
	if err != nil {
		t.Fatalf("failed to create feature task: %v", err)
	}

	mergeRequest, err := store.CreateMergeRequest(context, MergeRequestArgs{
		FeatureTaskID: featureTask.ID,
	})
	if err != nil {
		t.Fatalf("failed to create merge request: %v", err)
	}

	reviewJob, err := store.CreateReviewJob(context, ReviewJobCreateArgs{
		MergeRequestID: mergeRequest.ID,
		SessionID:      session.ID,
		State:          "requested",
	})
	if err != nil {
		t.Fatalf("failed to create review job: %v", err)
	}
	if reviewJob.State != "requested" {
		t.Fatalf("expected requested state, got %s", reviewJob.State)
	}

	reviewerThread, err := store.CreateThread(context, ThreadCreateArgs{
		SessionID:      session.ID,
		ParentThreadID: nil,
		Role:           "merge-reviewer",
		Status:         "running",
	})
	if err != nil {
		t.Fatalf("failed to create reviewer thread: %v", err)
	}

	runningState := "running"
	reviewJob, err = store.UpdateReviewJob(context, reviewJob.ID, ReviewJobUpdateArgs{
		State:            &runningState,
		ReviewerThreadID: &reviewerThread.ID,
	})
	if err != nil {
		t.Fatalf("failed to update review job: %v", err)
	}
	if reviewJob.ReviewerThreadID == nil || *reviewJob.ReviewerThreadID != reviewerThread.ID {
		t.Fatalf("expected reviewer thread id=%d, got %+v", reviewerThread.ID, reviewJob.ReviewerThreadID)
	}

	latestReviewJob, err := store.GetLatestReviewJobByMergeRequest(context, mergeRequest.ID)
	if err != nil {
		t.Fatalf("failed to load latest review job: %v", err)
	}
	if latestReviewJob == nil || latestReviewJob.ID != reviewJob.ID {
		t.Fatalf("expected latest review job id=%d, got %+v", reviewJob.ID, latestReviewJob)
	}
}

func TestSessionRuntimeFields(t *testing.T) {
	context := context.Background()
	store := openThreadTestStore(t)
	defer store.Close()

	session, err := store.OpenSession(context, SessionOpenArgs{
		AgentRole: "codex",
		Owner:     "owner-a",
		RepoPath:  "/tmp/repo-a",
		Intent:    "new_work",
	})
	if err != nil {
		t.Fatalf("failed to open session: %v", err)
	}

	rootThread, err := store.CreateThread(context, ThreadCreateArgs{
		SessionID: session.ID,
		Role:      "session-root",
	})
	if err != nil {
		t.Fatalf("failed to create root thread: %v", err)
	}

	tmuxSessionName := "codex-root-test"
	runtimeState := "tmux_ready"
	updatedSession, err := store.UpdateSession(context, session.ID, SessionUpdateArgs{
		RootThreadID:    &rootThread.ID,
		TmuxSessionName: &tmuxSessionName,
		RuntimeState:    &runtimeState,
	})
	if err != nil {
		t.Fatalf("failed to update session runtime fields: %v", err)
	}
	if updatedSession.RootThreadID == nil || *updatedSession.RootThreadID != rootThread.ID {
		t.Fatalf("expected root_thread_id=%d, got %+v", rootThread.ID, updatedSession.RootThreadID)
	}
	if updatedSession.TmuxSessionName == nil || *updatedSession.TmuxSessionName != tmuxSessionName {
		t.Fatalf("expected tmux_session_name=%s, got %+v", tmuxSessionName, updatedSession.TmuxSessionName)
	}
	if updatedSession.RuntimeState == nil || *updatedSession.RuntimeState != runtimeState {
		t.Fatalf("expected runtime_state=%s, got %+v", runtimeState, updatedSession.RuntimeState)
	}

	event, err := store.RecordRuntimePrereqEvent(context, RuntimePrereqEventCreateArgs{
		SessionID:   &updatedSession.ID,
		Requirement: "tmux",
		Status:      "ready",
		Detail:      "available",
	})
	if err != nil {
		t.Fatalf("failed to record runtime event: %v", err)
	}
	if event.SessionID == nil || *event.SessionID != updatedSession.ID {
		t.Fatalf("expected event session_id=%d, got %+v", updatedSession.ID, event.SessionID)
	}
}

func TestSessionDelegationFields(t *testing.T) {
	context := context.Background()
	store := openThreadTestStore(t)
	defer store.Close()

	session, err := store.OpenSession(context, SessionOpenArgs{
		AgentRole: "codex",
		Owner:     "owner-a",
		RepoPath:  "/tmp/repo-a",
		Intent:    "new_work",
	})
	if err != nil {
		t.Fatalf("failed to open session: %v", err)
	}

	rootThread, err := store.CreateThread(context, ThreadCreateArgs{
		SessionID: session.ID,
		Role:      "session-root",
		Status:    "running",
	})
	if err != nil {
		t.Fatalf("failed to create root thread: %v", err)
	}

	state := "delegated"
	issuedAt := "2026-02-11T10:00:00Z"
	ackedAt := "2026-02-11T10:01:00Z"
	updatedSession, err := store.UpdateSession(context, session.ID, SessionUpdateArgs{
		RootThreadID:           &rootThread.ID,
		DelegationState:        &state,
		DelegationRootThreadID: &rootThread.ID,
		DelegationIssuedAt:     &issuedAt,
		DelegationAckedAt:      &ackedAt,
	})
	if err != nil {
		t.Fatalf("failed to update delegation fields: %v", err)
	}

	if updatedSession.DelegationState == nil || *updatedSession.DelegationState != state {
		t.Fatalf("expected delegation_state=%s, got %+v", state, updatedSession.DelegationState)
	}
	if updatedSession.DelegationRootThreadID == nil || *updatedSession.DelegationRootThreadID != rootThread.ID {
		t.Fatalf("expected delegation_root_thread_id=%d, got %+v", rootThread.ID, updatedSession.DelegationRootThreadID)
	}
	if updatedSession.DelegationIssuedAt == nil || *updatedSession.DelegationIssuedAt != issuedAt {
		t.Fatalf("expected delegation_issued_at=%s, got %+v", issuedAt, updatedSession.DelegationIssuedAt)
	}
	if updatedSession.DelegationAckedAt == nil || *updatedSession.DelegationAckedAt != ackedAt {
		t.Fatalf("expected delegation_acked_at=%s, got %+v", ackedAt, updatedSession.DelegationAckedAt)
	}
}

func openThreadTestStore(t *testing.T) *Store {
	t.Helper()

	tempDir := t.TempDir()
	store, err := Open(filepath.Join(tempDir, "state.db"))
	if err != nil {
		t.Fatalf("failed to open test store: %v", err)
	}
	return store
}
