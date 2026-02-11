package orchestrator

import (
	"strings"
	"testing"

	"github.com/cayde/llm/features/codex-collab-orchestrator/components/mcp/servers/codex-orchestrator/internal/store"
)

func TestResolveChildTmuxSessionName(t *testing.T) {
	service := &Service{}

	defaultName := service.resolveChildTmuxSessionName("", 42)
	if defaultName != "codex-child-42" {
		t.Fatalf("expected default child session name codex-child-42, got %s", defaultName)
	}

	overrideName := service.resolveChildTmuxSessionName("custom-child", 42)
	if overrideName != "custom-child" {
		t.Fatalf("expected override child session name custom-child, got %s", overrideName)
	}
}

func TestResolveRootTmuxSessionName(t *testing.T) {
	service := &Service{}

	sessionName := "codex-root-existing"
	session := store.Session{
		TmuxSessionName: &sessionName,
	}
	resolved := service.resolveRootTmuxSessionName("", session, 7)
	if resolved != sessionName {
		t.Fatalf("expected existing session name %s, got %s", sessionName, resolved)
	}

	fallback := service.resolveRootTmuxSessionName("", store.Session{}, 7)
	if fallback != "codex-root-7" {
		t.Fatalf("expected fallback root session name codex-root-7, got %s", fallback)
	}
}

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

func TestDefaultRootPromptIncludesHandoff(t *testing.T) {
	service := &Service{repoPath: t.TempDir()}
	session := store.Session{ID: 77}
	thread := store.Thread{
		ID:               12,
		Role:             "session-root",
		TaskSpecJSON:     pointerToString(`{"title":"root","objective":"delegate work"}`),
		ScopeTaskIDsJSON: pointerToString(`[1,2]`),
	}
	prompt := service.defaultRootPrompt(session, thread, threadRootEnsureInput{
		Objective: "delegate work",
		Title:     "root",
	}, "codex-child-12")

	if !strings.Contains(prompt, "thread.root.handoff_ack") {
		t.Fatalf("expected handoff ack method in root prompt, got %s", prompt)
	}
	if !strings.Contains(prompt, "\"task_ids\"") {
		t.Fatalf("expected scope payload in root prompt, got %s", prompt)
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
