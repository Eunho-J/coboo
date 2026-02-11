package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cayde/llm/features/codex-collab-orchestrator/components/mcp/servers/codex-orchestrator/internal/store"
)

const featureBundleName = "codex-collab-orchestrator"

func (service *Service) runtimeBundleInfo(_ context.Context, _ runtimeBundleInfoInput) (map[string]any, error) {
	agentsRoot := filepath.Join(service.repoPath, ".codex", "agents", featureBundleName, "codex")
	skillsRoot := filepath.Join(service.repoPath, ".agents", "skills", "codex-work-orchestrator")
	mcpRoot := filepath.Join(service.repoPath, ".codex", "mcp", "features", featureBundleName)
	sourceRoot := filepath.Join(service.repoPath, "features", featureBundleName)

	roleTemplates := map[string]string{
		"session-root":       filepath.Join(agentsRoot, "root-orchestrator.md"),
		"worker":             filepath.Join(agentsRoot, "main-worker.md"),
		"merge-reviewer":     filepath.Join(agentsRoot, "merge-reviewer.md"),
		"doc-mirror-manager": filepath.Join(agentsRoot, "doc-mirror-manager.md"),
		"plan-architect":     filepath.Join(agentsRoot, "plan-architect.md"),
	}

	return map[string]any{
		"feature":     featureBundleName,
		"repo_path":   service.repoPath,
		"source_root": sourceRoot,
		"agents_root": agentsRoot,
		"skills_root": skillsRoot,
		"mcp_root":    mcpRoot,
		"exists": map[string]any{
			"source_root": pathExists(sourceRoot),
			"agents_root": pathExists(agentsRoot),
			"skills_root": pathExists(skillsRoot),
			"mcp_root":    pathExists(mcpRoot),
		},
		"role_templates": roleTemplates,
		"sync_verify": map[string]any{
			"command": fmt.Sprintf("./scripts/verify-feature-sync.sh %s %s", featureBundleName, service.repoPath),
		},
	}, nil
}

func (service *Service) delegateOrchestration(ctx context.Context, input orchestrationDelegateInput) (map[string]any, error) {
	if input.SessionID <= 0 {
		return nil, errors.New("session_id is required")
	}

	session, err := service.store.GetSessionByID(ctx, input.SessionID)
	if err != nil {
		return nil, err
	}

	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = strings.TrimSpace(input.UserRequest)
	}
	if title == "" {
		title = "root orchestration"
	}

	objective := strings.TrimSpace(input.Objective)
	if objective == "" {
		objective = strings.TrimSpace(input.UserRequest)
	}
	if objective == "" {
		objective = strings.TrimSpace(title)
	}

	taskSpec := normalizeRawJSON(input.TaskSpec)
	if taskSpec == "" {
		extra := map[string]any{}
		if strings.TrimSpace(input.UserRequest) != "" {
			extra["user_request"] = strings.TrimSpace(input.UserRequest)
		}
		extra["delegated_from"] = "caller_cli"
		taskSpec = service.defaultTaskSpecJSON("session-root", title, objective, extra)
	}

	rootEnsureInput := threadRootEnsureInput{
		SessionID:             input.SessionID,
		Role:                  "session-root",
		Title:                 title,
		Objective:             objective,
		EnsureTmux:            input.EnsureTmux,
		AutoInstall:           input.AutoInstall,
		AgentGuidePath:        input.AgentGuidePath,
		TmuxSessionName:       input.TmuxSessionName,
		TmuxWindowName:        input.TmuxWindowName,
		InitialPrompt:         input.InitialPrompt,
		CodexCommand:          input.CodexCommand,
		LaunchCodex:           pointerToBool(true),
		ForceLaunch:           pointerToBool(true),
		ChildSessionName:      input.ChildSessionName,
		MaxConcurrentChildren: input.MaxConcurrentChildren,
		TaskSpec:              json.RawMessage(taskSpec),
		ScopeTaskIDs:          input.ScopeTaskIDs,
		ScopeCaseIDs:          input.ScopeCaseIDs,
		ScopeNodeIDs:          input.ScopeNodeIDs,
	}

	updatedSession, rootThread, tmuxResult, err := service.ensureRootThreadInternal(ctx, rootEnsureInput)
	if err != nil {
		return nil, err
	}

	delegatedState := "delegated"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	emptyAck := ""
	updatedSession, err = service.store.UpdateSession(ctx, updatedSession.ID, store.SessionUpdateArgs{
		DelegationState:        &delegatedState,
		DelegationRootThreadID: &rootThread.ID,
		DelegationIssuedAt:     &now,
		DelegationAckedAt:      &emptyAck,
		RuntimeState:           pointerToString("delegated_to_root"),
	})
	if err != nil {
		return nil, err
	}

	attachInfo, err := service.buildAttachInfo(ctx, updatedSession, &rootThread)
	if err != nil {
		return nil, err
	}
	childSessionName := service.resolveChildTmuxSessionName(input.ChildSessionName, rootThread.ID)

	return map[string]any{
		"session":             updatedSession,
		"root_thread":         rootThread,
		"tmux":                tmuxResult,
		"attach_info":         attachInfo,
		"child_tmux_session":  childSessionName,
		"child_attach_hint":   fmt.Sprintf("tmux attach-session -t %s", childSessionName),
		"caller_action":       "return_to_idle",
		"handoff_ack_method":  "thread.root.handoff_ack",
		"delegation_contract": "caller_cli_bootstrap_only",
		"session_origin": map[string]any{
			"session_id": session.ID,
			"status":     session.Status,
		},
	}, nil
}

func (service *Service) ackRootHandoff(ctx context.Context, input threadRootHandoffAckInput) (map[string]any, error) {
	if input.SessionID <= 0 {
		return nil, errors.New("session_id is required")
	}
	if input.ThreadID <= 0 {
		return nil, errors.New("thread_id is required")
	}

	session, err := service.store.GetSessionByID(ctx, input.SessionID)
	if err != nil {
		return nil, err
	}
	thread, err := service.store.GetThreadByID(ctx, input.ThreadID)
	if err != nil {
		return nil, err
	}
	if thread.SessionID != input.SessionID {
		return nil, fmt.Errorf("thread belongs to another session: %d", thread.SessionID)
	}

	if session.RootThreadID != nil && *session.RootThreadID != thread.ID {
		return nil, fmt.Errorf("thread_id=%d is not the session root thread", thread.ID)
	}

	ackState := strings.TrimSpace(input.State)
	if ackState == "" {
		ackState = "acknowledged"
	}
	ackTime := time.Now().UTC().Format(time.RFC3339Nano)

	updatedSession, err := service.store.UpdateSession(ctx, input.SessionID, store.SessionUpdateArgs{
		DelegationState:        &ackState,
		DelegationRootThreadID: &thread.ID,
		DelegationAckedAt:      &ackTime,
		RuntimeState:           pointerToString("root_active"),
	})
	if err != nil {
		return nil, err
	}

	attachInfo, err := service.buildAttachInfo(ctx, updatedSession, &thread)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"session":     updatedSession,
		"root_thread": thread,
		"attach_info": attachInfo,
		"result":      "handoff_acknowledged",
		"ack_state":   ackState,
		"acked_at":    ackTime,
	}, nil
}

func pathExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	if _, err := os.Stat(path); err != nil {
		return false
	}
	return true
}

func (service *Service) requireDelegationAck(ctx context.Context, sessionID int64, method string) error {
	if sessionID <= 0 {
		return nil
	}
	session, err := service.store.GetSessionByID(ctx, sessionID)
	if err != nil {
		return err
	}
	state := strings.ToLower(strings.TrimSpace(valueOrEmpty(session.DelegationState)))
	if state != "delegated" {
		return nil
	}
	if strings.TrimSpace(valueOrEmpty(session.DelegationAckedAt)) != "" {
		return nil
	}
	return fmt.Errorf("session %d is delegated to root thread %d; %s is blocked until thread.root.handoff_ack", sessionID, int64OrZero(session.DelegationRootThreadID), method)
}

func int64OrZero(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
