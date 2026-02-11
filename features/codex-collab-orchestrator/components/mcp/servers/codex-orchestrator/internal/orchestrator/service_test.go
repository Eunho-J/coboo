package orchestrator

import "testing"

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
