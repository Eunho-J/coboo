package orchestrator

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/cayde/llm/features/codex-collab-orchestrator/components/mcp/servers/codex-orchestrator/internal/store"
)

func TestDecideWorktreeSharedMode(t *testing.T) {
	result := decideWorktree(worktreeDecisionInput{
		ChangedFiles:     2,
		EstimateMinutes:  15,
		Risk:             1,
		ParallelWorkers:  1,
		ConflictingPaths: 1,
	})

	if result.Mode != "shared" {
		t.Fatalf("expected shared mode, got %s", result.Mode)
	}
	if result.Score >= 12 {
		t.Fatalf("expected score < 12 for shared mode, got %d", result.Score)
	}
}

func TestDecideWorktreeWorktreeMode(t *testing.T) {
	result := decideWorktree(worktreeDecisionInput{
		ChangedFiles:     6,
		EstimateMinutes:  60,
		Risk:             2,
		ParallelWorkers:  2,
		ConflictingPaths: 2,
	})

	if result.Mode != "worktree" {
		t.Fatalf("expected worktree mode, got %s", result.Mode)
	}
	if result.Score < 12 {
		t.Fatalf("expected score >= 12 for worktree mode, got %d", result.Score)
	}
}

func TestRuntimeBundleInfo(t *testing.T) {
	repoPath := t.TempDir()
	service, err := NewService(repoPath)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	defer service.Close()

	result, err := service.runtimeBundleInfo(context.Background(), runtimeBundleInfoInput{})
	if err != nil {
		t.Fatalf("failed to get runtime bundle info: %v", err)
	}
	if result["feature"] != featureBundleName {
		t.Fatalf("expected feature=%s, got %+v", featureBundleName, result["feature"])
	}
}

func TestAckRootHandoff(t *testing.T) {
	repoPath := t.TempDir()
	service, err := NewService(repoPath)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	defer service.Close()

	ctx := context.Background()
	session, err := service.store.OpenSession(ctx, store.SessionOpenArgs{
		AgentRole: "codex",
		Owner:     "tester",
		RepoPath:  filepath.ToSlash(repoPath),
		Intent:    "new_work",
	})
	if err != nil {
		t.Fatalf("failed to open session: %v", err)
	}
	rootThread, err := service.store.CreateThread(ctx, store.ThreadCreateArgs{
		SessionID: session.ID,
		Role:      "session-root",
		Status:    "running",
	})
	if err != nil {
		t.Fatalf("failed to create root thread: %v", err)
	}
	_, err = service.store.UpdateSession(ctx, session.ID, store.SessionUpdateArgs{
		RootThreadID: &rootThread.ID,
	})
	if err != nil {
		t.Fatalf("failed to update root thread binding: %v", err)
	}

	response, err := service.ackRootHandoff(ctx, threadRootHandoffAckInput{
		SessionID: session.ID,
		ThreadID:  rootThread.ID,
	})
	if err != nil {
		t.Fatalf("failed to ack root handoff: %v", err)
	}
	if response["result"] != "handoff_acknowledged" {
		t.Fatalf("expected handoff_acknowledged result, got %+v", response["result"])
	}
}
