package orchestrator

import (
	"strings"
	"testing"

	"github.com/cayde/llm/features/codex-collab-orchestrator/components/mcp/servers/codex-orchestrator/internal/store"
)

func TestResolveAgentGuidePathForRole(t *testing.T) {
	service := &Service{}

	if path := service.resolveAgentGuidePathForRole("session-root", ""); path != defaultRootAgentGuidePath {
		t.Fatalf("expected default root guide path, got %s", path)
	}
	if path := service.resolveAgentGuidePathForRole("merge-reviewer", ""); path != defaultMergeReviewerPath {
		t.Fatalf("expected default merge reviewer guide path, got %s", path)
	}
	if path := service.resolveAgentGuidePathForRole("worker", "/tmp/custom.md"); path != "/tmp/custom.md" {
		t.Fatalf("expected override guide path, got %s", path)
	}
}

func TestDefaultCodexLaunchCommand(t *testing.T) {
	service := &Service{}
	command := service.defaultCodexLaunchCommand("/tmp/work", "", ".codex/agents/root.md", "orchestrate this task")

	if !strings.Contains(command, "cd '/tmp/work'") {
		t.Fatalf("expected workdir change in launch command, got %s", command)
	}
	if !strings.Contains(command, "codex --no-alt-screen") {
		t.Fatalf("expected default codex command in launch command, got %s", command)
	}
	if !strings.Contains(command, "'orchestrate this task'") {
		t.Fatalf("expected quoted initial prompt in launch command, got %s", command)
	}
}

func TestDefaultChildPromptIncludesScope(t *testing.T) {
	service := &Service{repoPath: t.TempDir()}
	thread := store.Thread{
		ID:               21,
		Role:             "worker",
		ScopeCaseIDsJSON: pointerToString(`[301]`),
		TaskSpecJSON:     pointerToString(`{"title":"case-301"}`),
	}
	prompt := service.defaultChildPrompt(9, 3, thread, threadChildSpawnInput{
		Objective: "implement case-301",
	})
	if !strings.Contains(prompt, "\"case_ids\"") {
		t.Fatalf("expected case scope in child prompt, got %s", prompt)
	}
	if !strings.Contains(prompt, "Work only on this thread assignment") {
		t.Fatalf("expected scoped execution rule in child prompt, got %s", prompt)
	}
}

func TestIsChildThreadReusable(t *testing.T) {
	if !isChildThreadReusable("completed") {
		t.Fatalf("expected completed to be reusable")
	}
	if !isChildThreadReusable("failed") {
		t.Fatalf("expected failed to be reusable")
	}
	if isChildThreadReusable("running") {
		t.Fatalf("expected running to not be reusable")
	}
}
