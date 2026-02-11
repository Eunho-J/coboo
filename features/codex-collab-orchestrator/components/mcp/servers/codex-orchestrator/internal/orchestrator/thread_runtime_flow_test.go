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
