package orchestrator

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
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

func TestDeriveWorktreeSlugLimitsToTwoWords(t *testing.T) {
	slug := deriveWorktreeSlug("", "Improve the gallery experience for artist profile pages")
	if strings.Count(slug, "-") > 1 {
		t.Fatalf("expected 1-2 words slug, got %s", slug)
	}
	if slug == "" {
		t.Fatalf("expected non-empty slug")
	}
}

func TestBuildViewerTmuxSessionName(t *testing.T) {
	repoPath := t.TempDir()
	service, err := NewService(repoPath)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	defer service.Close()

	sessionName := service.buildViewerTmuxSessionName("/tmp/worktrees/gallery-refresh")
	if !strings.Contains(sessionName, "-") {
		t.Fatalf("expected repository-worktree format, got %s", sessionName)
	}
	if strings.Contains(sessionName, " ") {
		t.Fatalf("expected normalized tmux session name, got %s", sessionName)
	}
}

func TestThreadChildDirectiveDecode(t *testing.T) {
	payload := []byte(`{"thread_id":12,"directive":"continue with new constraint","mode":"interrupt_patch"}`)
	var input threadChildDirectiveInput
	if err := json.Unmarshal(payload, &input); err != nil {
		t.Fatalf("expected directive payload to decode: %v", err)
	}
	if input.ThreadID != 12 {
		t.Fatalf("expected thread_id=12, got %d", input.ThreadID)
	}
	if input.Mode != "interrupt_patch" {
		t.Fatalf("expected mode interrupt_patch, got %s", input.Mode)
	}
}
