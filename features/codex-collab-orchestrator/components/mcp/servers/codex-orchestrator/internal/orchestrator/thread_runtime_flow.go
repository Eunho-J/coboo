package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/cayde/llm/features/codex-collab-orchestrator/components/mcp/servers/codex-orchestrator/internal/store"
)

const (
	defaultTmuxEnsureTimeout  = 120 * time.Second
	defaultRootAgentGuidePath = ".codex/agents/codex-collab-orchestrator/codex/root-orchestrator.md"
	defaultWorkerGuidePath    = ".codex/agents/codex-collab-orchestrator/codex/main-worker.md"
	defaultMergeReviewerPath  = ".codex/agents/codex-collab-orchestrator/codex/merge-reviewer.md"
	defaultDocMirrorPath      = ".codex/agents/codex-collab-orchestrator/codex/doc-mirror-manager.md"
	defaultPlanArchitectPath  = ".codex/agents/codex-collab-orchestrator/codex/plan-architect.md"
	defaultChildWindowName    = "children"
	defaultCodexCommand       = "codex --no-alt-screen"
	defaultAgentsRunnerScript = "features/codex-collab-orchestrator/components/mcp/servers/codex-orchestrator/scripts/agents_codex_runner.py"
	defaultPythonCommand      = "python3"
	defaultRunnerKind         = "agents_sdk_codex_mcp"
	defaultInteractionMode    = "view_only"
	defaultMaxChildThreads    = 6
)

func (service *Service) ensureTmux(ctx context.Context, input runtimeTmuxEnsureInput) (map[string]any, error) {
	autoInstall := boolValueOrDefault(input.AutoInstall, true)

	sessionID := int64(0)
	if input.SessionID != nil {
		sessionID = *input.SessionID
	}

	recordEvent := func(status string, detail string) *store.RuntimePrereqEvent {
		args := store.RuntimePrereqEventCreateArgs{
			Requirement: "tmux",
			Status:      status,
			Detail:      detail,
		}
		if sessionID > 0 {
			args.SessionID = input.SessionID
		}
		event, err := service.store.RecordRuntimePrereqEvent(ctx, args)
		if err != nil {
			return nil
		}
		return &event
	}

	if tmuxPath, err := exec.LookPath("tmux"); err == nil {
		if input.SessionID != nil {
			_, _ = service.store.UpdateSession(ctx, *input.SessionID, store.SessionUpdateArgs{
				RuntimeState: pointerToString("tmux_ready"),
			})
		}
		event := recordEvent("ready", "tmux already available")
		return map[string]any{
			"status":    "ready",
			"tmux_path": tmuxPath,
			"event":     event,
		}, nil
	}

	if !autoInstall {
		manual := service.manualTmuxInstallInstructions()
		if input.SessionID != nil {
			_, _ = service.store.UpdateSession(ctx, *input.SessionID, store.SessionUpdateArgs{
				RuntimeState: pointerToString("tmux_manual_required"),
			})
		}
		event := recordEvent("manual_required", "auto install disabled")
		return map[string]any{
			"status":              "manual_required",
			"manual_instructions": manual,
			"event":               event,
		}, nil
	}

	installResult := service.tryInstallTmux(ctx)
	if installResult.installed {
		if input.SessionID != nil {
			_, _ = service.store.UpdateSession(ctx, *input.SessionID, store.SessionUpdateArgs{
				RuntimeState: pointerToString("tmux_ready"),
			})
		}
		event := recordEvent("installed", installResult.message)
		return map[string]any{
			"status":              "installed",
			"tmux_path":           installResult.tmuxPath,
			"attempts":            installResult.attempts,
			"manual_instructions": service.manualTmuxInstallInstructions(),
			"event":               event,
		}, nil
	}

	if input.SessionID != nil {
		_, _ = service.store.UpdateSession(ctx, *input.SessionID, store.SessionUpdateArgs{
			RuntimeState: pointerToString("tmux_manual_required"),
		})
	}
	event := recordEvent("manual_required", installResult.message)
	return map[string]any{
		"status":              "manual_required",
		"message":             installResult.message,
		"attempts":            installResult.attempts,
		"manual_instructions": service.manualTmuxInstallInstructions(),
		"event":               event,
	}, nil
}

func (service *Service) listChildThreads(ctx context.Context, input threadChildListInput) (map[string]any, error) {
	if input.SessionID <= 0 {
		return nil, errors.New("session_id is required")
	}
	threads, err := service.store.ListThreads(ctx, store.ThreadFilter{
		SessionID:      input.SessionID,
		ParentThreadID: input.ParentThreadID,
		Status:         input.Status,
		Role:           input.Role,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"threads": threads,
	}, nil
}

func (service *Service) spawnChildThread(ctx context.Context, input threadChildSpawnInput) (map[string]any, error) {
	thread, attachInfo, tmuxResult, err := service.spawnChildThreadInternal(ctx, input)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"thread":      thread,
		"attach_info": attachInfo,
		"tmux":        tmuxResult,
	}, nil
}

func (service *Service) directiveChildThread(ctx context.Context, input threadChildDirectiveInput) (map[string]any, error) {
	if input.ThreadID <= 0 {
		return nil, errors.New("thread_id is required")
	}
	directive := strings.TrimSpace(input.Directive)
	if directive == "" {
		return nil, errors.New("directive is required")
	}
	mode := strings.ToLower(strings.TrimSpace(input.Mode))
	if mode == "" {
		mode = "interrupt_patch"
	}

	thread, err := service.store.GetThreadByID(ctx, input.ThreadID)
	if err != nil {
		return nil, err
	}
	if thread.ParentThreadID == nil {
		return nil, fmt.Errorf("thread is not a child thread: %d", thread.ID)
	}
	if thread.TmuxPaneID == nil || strings.TrimSpace(*thread.TmuxPaneID) == "" {
		return nil, fmt.Errorf("thread has no tmux pane bound: %d", thread.ID)
	}
	paneID := strings.TrimSpace(*thread.TmuxPaneID)

	sendDirective := func() error {
		if _, err := service.runCommand(ctx, "tmux", "send-keys", "-t", paneID, directive, "C-m"); err != nil {
			return err
		}
		return nil
	}

	switch mode {
	case "queue":
		if err := sendDirective(); err != nil {
			return nil, err
		}
	case "restart":
		_, _ = service.stopChildThread(ctx, threadChildStopInput{
			ThreadID:      thread.ID,
			TerminatePane: pointerToBool(true),
		})
		respawnedThread, attachInfo, tmuxResult, spawnErr := service.spawnChildThreadInternal(ctx, threadChildSpawnInput{
			SessionID:      thread.SessionID,
			ParentThreadID: thread.ParentThreadID,
			WorktreeID:     thread.WorktreeID,
			Role:           thread.Role,
			Title:          strings.TrimSpace(valueOrEmpty(thread.Title)),
			Objective:      strings.TrimSpace(valueOrEmpty(thread.Objective)),
			AgentGuidePath: strings.TrimSpace(valueOrEmpty(thread.AgentGuidePath)),
			InitialPrompt:  directive,
			LaunchCodex:    pointerToBool(true),
		})
		if spawnErr != nil {
			return nil, spawnErr
		}
		return map[string]any{
			"result":      "respawned_with_directive",
			"mode":        mode,
			"thread":      respawnedThread,
			"attach_info": attachInfo,
			"tmux":        tmuxResult,
		}, nil
	default:
		if _, err := service.runCommand(ctx, "tmux", "send-keys", "-t", paneID, "C-c"); err != nil {
			return nil, err
		}
		if err := sendDirective(); err != nil {
			return nil, err
		}
		runningStatus := "running"
		_, _ = service.store.UpdateThread(ctx, thread.ID, store.ThreadUpdateArgs{Status: &runningStatus})
		mode = "interrupt_patch"
	}

	updatedThread, err := service.store.GetThreadByID(ctx, thread.ID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"result":    "directive_sent",
		"mode":      mode,
		"thread":    updatedThread,
		"directive": directive,
	}, nil
}

func (service *Service) interruptChildThread(ctx context.Context, input threadChildSignalInput) (map[string]any, error) {
	if input.ThreadID <= 0 {
		return nil, errors.New("thread_id is required")
	}
	thread, err := service.store.GetThreadByID(ctx, input.ThreadID)
	if err != nil {
		return nil, err
	}
	if thread.ParentThreadID == nil {
		return nil, fmt.Errorf("thread is not a child thread: %d", thread.ID)
	}
	if thread.TmuxPaneID == nil || strings.TrimSpace(*thread.TmuxPaneID) == "" {
		return nil, fmt.Errorf("thread has no tmux pane bound: %d", thread.ID)
	}

	if _, err := service.runCommand(ctx, "tmux", "send-keys", "-t", *thread.TmuxPaneID, "C-c"); err != nil {
		return nil, err
	}
	interruptedStatus := "interrupted"
	updatedThread, err := service.store.UpdateThread(ctx, thread.ID, store.ThreadUpdateArgs{
		Status: &interruptedStatus,
	})
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"thread": updatedThread,
		"result": "interrupt_sent",
	}, nil
}

func (service *Service) stopChildThread(ctx context.Context, input threadChildStopInput) (map[string]any, error) {
	if input.ThreadID <= 0 {
		return nil, errors.New("thread_id is required")
	}
	thread, err := service.store.GetThreadByID(ctx, input.ThreadID)
	if err != nil {
		return nil, err
	}
	if thread.ParentThreadID == nil {
		return nil, fmt.Errorf("thread is not a child thread: %d", thread.ID)
	}
	if thread.TmuxPaneID == nil || strings.TrimSpace(*thread.TmuxPaneID) == "" {
		return nil, fmt.Errorf("thread has no tmux pane bound: %d", thread.ID)
	}

	_, _ = service.runCommand(ctx, "tmux", "send-keys", "-t", *thread.TmuxPaneID, "exit", "C-m")
	updateArgs := store.ThreadUpdateArgs{}
	if boolValueOrDefault(input.TerminatePane, false) {
		_, _ = service.runCommand(ctx, "tmux", "kill-pane", "-t", *thread.TmuxPaneID)
		updateArgs.TmuxPaneID = pointerToString("")
		updateArgs.TmuxWindowName = pointerToString("")
	}

	stoppedStatus := "stopped"
	updateArgs.Status = &stoppedStatus
	updatedThread, err := service.store.UpdateThread(ctx, thread.ID, updateArgs)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"thread": updatedThread,
		"result": "stopped",
	}, nil
}

func (service *Service) threadAttachInfo(ctx context.Context, input threadAttachInfoInput) (map[string]any, error) {
	if input.SessionID <= 0 {
		return nil, errors.New("session_id is required")
	}
	session, err := service.store.GetSessionByID(ctx, input.SessionID)
	if err != nil {
		return nil, err
	}

	var thread *store.Thread
	if input.ThreadID != nil {
		loadedThread, loadErr := service.store.GetThreadByID(ctx, *input.ThreadID)
		if loadErr != nil {
			return nil, loadErr
		}
		thread = &loadedThread
	} else if session.RootThreadID != nil {
		loadedThread, loadErr := service.store.GetThreadByID(ctx, *session.RootThreadID)
		if loadErr == nil {
			thread = &loadedThread
		}
	}

	return service.buildAttachInfo(ctx, session, thread)
}

func (service *Service) requestAutoMergeReview(ctx context.Context, input mergeReviewRequestAutoInput) (map[string]any, error) {
	if input.SessionID <= 0 {
		return nil, errors.New("session_id is required")
	}
	if input.MergeRequestID <= 0 {
		return nil, errors.New("merge_request_id is required")
	}
	if _, err := service.store.GetMergeRequest(ctx, input.MergeRequestID); err != nil {
		return nil, err
	}
	mainMergeLock, err := service.store.AcquireMainMergeLock(ctx, input.SessionID, 0)
	if err != nil {
		return nil, err
	}

	reviewJob, err := service.store.CreateReviewJob(ctx, store.ReviewJobCreateArgs{
		MergeRequestID: input.MergeRequestID,
		SessionID:      input.SessionID,
		State:          "requested",
		NotesJSON:      input.AgentOverride,
	})
	if err != nil {
		return nil, err
	}

	title := fmt.Sprintf("merge-review:%d", input.MergeRequestID)
	objective := fmt.Sprintf("review merge request %d and report conflict risk", input.MergeRequestID)
	role := strings.TrimSpace(input.ReviewerRole)
	if role == "" {
		role = "merge-reviewer"
	}
	agentGuidePath := strings.TrimSpace(input.AgentGuidePath)
	if agentGuidePath == "" {
		agentGuidePath = defaultMergeReviewerPath
	}

	thread, attachInfo, tmuxResult, spawnErr := service.spawnChildThreadInternal(ctx, threadChildSpawnInput{
		SessionID:      input.SessionID,
		Role:           role,
		Title:          title,
		Objective:      objective,
		AgentGuidePath: agentGuidePath,
		AgentOverride:  input.AgentOverride,
		EnsureTmux:     input.EnsureTmux,
		AutoInstall:    input.AutoInstall,
		RunnerKind:     input.RunnerKind,
	})
	if spawnErr != nil {
		failedState := "failed"
		_, _ = service.store.UpdateReviewJob(ctx, reviewJob.ID, store.ReviewJobUpdateArgs{
			State: &failedState,
		})
		_, _ = service.store.ReleaseMainMergeLock(ctx, input.SessionID)
		return nil, spawnErr
	}

	runningState := "running"
	reviewJob, err = service.store.UpdateReviewJob(ctx, reviewJob.ID, store.ReviewJobUpdateArgs{
		State:            &runningState,
		ReviewerThreadID: &thread.ID,
	})
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"review_job":   reviewJob,
		"thread":       thread,
		"attach_info":  attachInfo,
		"tmux":         tmuxResult,
		"main_lock":    mainMergeLock,
		"merge_review": map[string]any{"merge_request_id": input.MergeRequestID},
	}, nil
}

func (service *Service) mergeReviewThreadStatus(ctx context.Context, input mergeReviewThreadStatusInput) (map[string]any, error) {
	var reviewJob *store.ReviewJob
	switch {
	case input.ReviewJobID != nil && *input.ReviewJobID > 0:
		job, err := service.store.GetReviewJobByID(ctx, *input.ReviewJobID)
		if err != nil {
			return nil, err
		}
		reviewJob = &job
	case input.MergeRequestID != nil && *input.MergeRequestID > 0:
		job, err := service.store.GetLatestReviewJobByMergeRequest(ctx, *input.MergeRequestID)
		if err != nil {
			return nil, err
		}
		if job == nil {
			return map[string]any{
				"review_job": nil,
				"thread":     nil,
			}, nil
		}
		reviewJob = job
	default:
		return nil, errors.New("review_job_id or merge_request_id is required")
	}

	response := map[string]any{
		"review_job": reviewJob,
	}
	if reviewJob.ReviewerThreadID != nil {
		thread, err := service.store.GetThreadByID(ctx, *reviewJob.ReviewerThreadID)
		if err == nil {
			response["thread"] = thread
			session, sessionErr := service.store.GetSessionByID(ctx, thread.SessionID)
			if sessionErr == nil {
				attachInfo, attachErr := service.buildAttachInfo(ctx, session, &thread)
				if attachErr == nil {
					response["attach_info"] = attachInfo
				}
			}
		}
	}
	return response, nil
}

func (service *Service) spawnChildThreadInternal(ctx context.Context, input threadChildSpawnInput) (store.Thread, map[string]any, map[string]any, error) {
	if input.SessionID <= 0 {
		return store.Thread{}, nil, nil, errors.New("session_id is required")
	}

	session, err := service.store.GetSessionByID(ctx, input.SessionID)
	if err != nil {
		return store.Thread{}, nil, nil, err
	}
	session, rootThread, err := service.ensureRootThreadRecord(ctx, session)
	if err != nil {
		return store.Thread{}, nil, nil, err
	}

	tmuxResult := map[string]any{"status": "skipped"}
	ensureTmux := boolValueOrDefault(input.EnsureTmux, true)
	if ensureTmux {
		tmuxResult, err = service.ensureTmux(ctx, runtimeTmuxEnsureInput{
			SessionID:   &session.ID,
			AutoInstall: input.AutoInstall,
		})
		if err != nil {
			return store.Thread{}, nil, nil, err
		}
	}

	parentThreadID := rootThread.ID
	if input.ParentThreadID != nil && *input.ParentThreadID > 0 {
		parentThread, parentErr := service.store.GetThreadByID(ctx, *input.ParentThreadID)
		if parentErr != nil {
			return store.Thread{}, nil, nil, parentErr
		}
		if parentThread.SessionID != input.SessionID {
			return store.Thread{}, nil, nil, fmt.Errorf("parent thread belongs to another session: %d", parentThread.SessionID)
		}
		parentThreadID = parentThread.ID
	}

	role := strings.TrimSpace(input.Role)
	if role == "" {
		role = "worker"
	}
	runnerKind := strings.TrimSpace(input.RunnerKind)
	if runnerKind == "" {
		runnerKind = defaultRunnerKind
	}
	interactionMode := strings.TrimSpace(input.InteractionMode)
	if interactionMode == "" {
		interactionMode = defaultInteractionMode
	}

	resolvedGuidePath := service.resolveAgentGuidePathForRole(role, input.AgentGuidePath)
	agentOverride := normalizeAgentOverride(input.AgentOverride)
	taskSpecJSON := normalizeRawJSON(input.TaskSpec)
	if taskSpecJSON == "" {
		taskSpecJSON = service.defaultTaskSpecJSON(role, input.Title, input.Objective, map[string]any{
			"runner_kind":      runnerKind,
			"interaction_mode": interactionMode,
		})
	}
	scopeTaskIDsJSON := marshalInt64Slice(input.ScopeTaskIDs)
	scopeCaseIDsJSON := marshalInt64Slice(input.ScopeCaseIDs)
	scopeNodeIDsJSON := marshalInt64Slice(input.ScopeNodeIDs)
	createdThread, err := service.store.CreateThread(ctx, store.ThreadCreateArgs{
		SessionID:        input.SessionID,
		ParentThreadID:   &parentThreadID,
		WorktreeID:       input.WorktreeID,
		Role:             role,
		Status:           "planned",
		Title:            input.Title,
		Objective:        input.Objective,
		AgentGuidePath:   resolvedGuidePath,
		AgentOverride:    agentOverride,
		TaskSpecJSON:     taskSpecJSON,
		ScopeTaskIDsJSON: scopeTaskIDsJSON,
		ScopeCaseIDsJSON: scopeCaseIDsJSON,
		ScopeNodeIDsJSON: scopeNodeIDsJSON,
	})
	if err != nil {
		return store.Thread{}, nil, nil, err
	}

	attachInfo, attachErr := service.buildAttachInfo(ctx, session, &createdThread)
	if attachErr != nil {
		return store.Thread{}, nil, nil, attachErr
	}

	if !isTmuxReady(tmuxResult) {
		return createdThread, attachInfo, tmuxResult, nil
	}

	workdir, err := service.resolveThreadWorkdir(ctx, session, input.WorktreeID)
	if err != nil {
		return store.Thread{}, nil, nil, err
	}
	childSessionName := strings.TrimSpace(input.TmuxSessionName)
	if childSessionName == "" {
		childSessionName = strings.TrimSpace(valueOrEmpty(session.TmuxSessionName))
	}
	if childSessionName == "" {
		sessionRootPath, pathErr := service.resolveSessionRootPath(ctx, session)
		if pathErr == nil && strings.TrimSpace(sessionRootPath) != "" {
			childSessionName = service.buildViewerTmuxSessionName(sessionRootPath)
		} else {
			childSessionName = service.buildViewerTmuxSessionName(workdir)
		}
		updatedSession, updateErr := service.store.UpdateSession(ctx, session.ID, store.SessionUpdateArgs{
			TmuxSessionName: &childSessionName,
		})
		if updateErr == nil {
			session = updatedSession
		}
	}
	childWindowName := normalizeWindowName(input.TmuxWindowName, defaultChildWindowName)
	maxConcurrentChildren := intValueOrDefault(input.MaxConcurrentChildren, defaultMaxChildThreads)
	if maxConcurrentChildren <= 0 {
		maxConcurrentChildren = defaultMaxChildThreads
	}
	if _, _, err := service.ensureTmuxSession(ctx, childSessionName, workdir, childWindowName); err != nil {
		return store.Thread{}, nil, nil, err
	}
	if err := service.ensureChildPaneCapacity(ctx, input.SessionID, parentThreadID, childSessionName, maxConcurrentChildren); err != nil {
		return store.Thread{}, nil, nil, err
	}

	paneID, err := service.createTmuxPane(ctx, fmt.Sprintf("%s:0", childSessionName), workdir, input.SplitDirection)
	if err != nil {
		return store.Thread{}, nil, nil, err
	}

	launchCodex := boolValueOrDefault(input.LaunchCodex, true)
	launchCommand := strings.TrimSpace(input.LaunchCommand)
	if launchCommand == "" && launchCodex {
		initialPrompt := strings.TrimSpace(input.InitialPrompt)
		if initialPrompt == "" {
			initialPrompt = service.defaultChildPrompt(input.SessionID, rootThread.ID, createdThread, input)
		}
		if strings.TrimSpace(input.CodexCommand) != "" {
			launchCommand = service.defaultCodexLaunchCommand(workdir, input.CodexCommand, resolvedGuidePath, initialPrompt)
		} else {
			launchCommand = service.defaultAgentsRunnerLaunchCommand(workdir, input.SessionID, createdThread, role, initialPrompt)
		}
	}
	if strings.TrimSpace(launchCommand) != "" {
		if _, err := service.runCommand(ctx, "tmux", "send-keys", "-t", paneID, launchCommand, "C-m"); err != nil {
			return store.Thread{}, nil, nil, err
		}
	}

	threadStatus := "planned"
	if strings.TrimSpace(launchCommand) != "" {
		threadStatus = "running"
	}
	updatedThread, err := service.store.UpdateThread(ctx, createdThread.ID, store.ThreadUpdateArgs{
		Status:          &threadStatus,
		TmuxSessionName: &childSessionName,
		TmuxWindowName:  &childWindowName,
		TmuxPaneID:      &paneID,
		LaunchCommand:   &launchCommand,
	})
	if err != nil {
		return store.Thread{}, nil, nil, err
	}

	attachInfo, err = service.buildAttachInfo(ctx, session, &updatedThread)
	if err != nil {
		return store.Thread{}, nil, nil, err
	}
	return updatedThread, attachInfo, tmuxResult, nil
}

func (service *Service) ensureRootThreadRecord(ctx context.Context, session store.Session) (store.Session, store.Thread, error) {
	var rootThread *store.Thread
	if session.RootThreadID != nil {
		thread, err := service.store.GetThreadByID(ctx, *session.RootThreadID)
		if err == nil {
			rootThread = &thread
		}
	}
	if rootThread == nil {
		thread, err := service.store.GetSessionRootThread(ctx, session.ID)
		if err != nil {
			return store.Session{}, store.Thread{}, err
		}
		rootThread = thread
	}

	if rootThread == nil {
		taskSpecJSON := service.defaultTaskSpecJSON("session-root", "root-local orchestration", "orchestrate from caller CLI", map[string]any{
			"root_mode": "caller_cli",
		})
		createdThread, err := service.store.CreateThread(ctx, store.ThreadCreateArgs{
			SessionID:      session.ID,
			Role:           "session-root",
			Status:         "running",
			Title:          "root-local orchestration",
			Objective:      "manage planning and delegation from caller CLI",
			TaskSpecJSON:   taskSpecJSON,
			AgentGuidePath: defaultRootAgentGuidePath,
		})
		if err != nil {
			return store.Session{}, store.Thread{}, err
		}
		rootThread = &createdThread
	}

	updatedSession, err := service.store.UpdateSession(ctx, session.ID, store.SessionUpdateArgs{
		RootThreadID: &rootThread.ID,
		RuntimeState: pointerToString("root_local_active"),
	})
	if err != nil {
		return store.Session{}, store.Thread{}, err
	}
	return updatedSession, *rootThread, nil
}

func (service *Service) buildAttachInfo(_ context.Context, session store.Session, thread *store.Thread) (map[string]any, error) {
	sessionName := ""
	if thread != nil && thread.TmuxSessionName != nil && strings.TrimSpace(*thread.TmuxSessionName) != "" {
		sessionName = strings.TrimSpace(*thread.TmuxSessionName)
	}
	if sessionName == "" && session.TmuxSessionName != nil {
		sessionName = strings.TrimSpace(*session.TmuxSessionName)
	}

	paneID := ""
	if thread != nil && thread.TmuxPaneID != nil {
		paneID = strings.TrimSpace(*thread.TmuxPaneID)
	}
	attachCommand := ""
	attachReadonlyCommand := ""
	switchCommand := ""
	readOnly := false
	if thread != nil && thread.ParentThreadID != nil {
		readOnly = true
	}
	if sessionName != "" {
		attachCommand = fmt.Sprintf("tmux attach-session -t %s", sessionName)
		if readOnly {
			attachReadonlyCommand = fmt.Sprintf("tmux attach -r -t %s", sessionName)
			attachCommand = attachReadonlyCommand
		}
		switchCommand = fmt.Sprintf("tmux switch-client -t %s", sessionName)
	}

	return map[string]any{
		"available":                sessionName != "",
		"tmux_session":             sessionName,
		"tmux_pane_id":             paneID,
		"read_only":                readOnly,
		"attach_command":           attachCommand,
		"attach_readonly_command":  attachReadonlyCommand,
		"switch_command":           switchCommand,
	}, nil
}

func (service *Service) resolveSessionRootPath(ctx context.Context, session store.Session) (string, error) {
	if session.SessionRootWorktreeID != nil {
		sessionRoot, err := service.store.GetWorktreeByID(ctx, *session.SessionRootWorktreeID)
		if err != nil {
			return "", err
		}
		return sessionRoot.Path, nil
	}
	return service.repoPath, nil
}

func (service *Service) resolveThreadWorkdir(ctx context.Context, session store.Session, worktreeID *int64) (string, error) {
	if worktreeID != nil && *worktreeID > 0 {
		worktree, err := service.store.GetWorktreeByID(ctx, *worktreeID)
		if err != nil {
			return "", err
		}
		return worktree.Path, nil
	}
	return service.resolveSessionRootPath(ctx, session)
}

func (service *Service) ensureTmuxSession(ctx context.Context, sessionName string, workdir string, windowName string) (string, bool, error) {
	if strings.TrimSpace(sessionName) == "" {
		return "", false, errors.New("tmux session name is required")
	}
	if strings.TrimSpace(workdir) == "" {
		workdir = service.repoPath
	}

	created := false
	if _, err := service.runCommand(ctx, "tmux", "has-session", "-t", sessionName); err != nil {
		args := []string{"new-session", "-d", "-s", sessionName}
		if strings.TrimSpace(windowName) != "" {
			args = append(args, "-n", windowName)
		}
		args = append(args, "-c", workdir)
		if _, createErr := service.runCommand(ctx, "tmux", args...); createErr != nil {
			return "", false, createErr
		}
		created = true
	}

	targetWindow := fmt.Sprintf("%s:0", sessionName)
	if strings.TrimSpace(windowName) != "" {
		if _, err := service.runCommand(ctx, "tmux", "rename-window", "-t", targetWindow, windowName); err != nil {
			return "", false, err
		}
	}

	panesOutput, err := service.runCommand(ctx, "tmux", "list-panes", "-t", targetWindow, "-F", "#{pane_id}")
	if err != nil {
		return "", false, err
	}
	lines := strings.Split(strings.TrimSpace(panesOutput), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return "", false, fmt.Errorf("tmux session has no panes: %s", sessionName)
	}
	return strings.TrimSpace(lines[0]), created, nil
}

func (service *Service) createTmuxPane(ctx context.Context, target string, workdir string, splitDirection string) (string, error) {
	directionFlag := "-v"
	if strings.EqualFold(strings.TrimSpace(splitDirection), "horizontal") {
		directionFlag = "-h"
	}

	output, err := service.runCommand(
		ctx,
		"tmux",
		"split-window",
		directionFlag,
		"-t", target,
		"-c", workdir,
		"-P",
		"-F", "#{pane_id}",
	)
	if err != nil {
		return "", err
	}
	paneID := strings.TrimSpace(output)
	if paneID == "" {
		return "", errors.New("failed to capture tmux pane id")
	}
	return paneID, nil
}

func (service *Service) defaultCodexLaunchCommand(workdir string, codexCommand string, agentGuidePath string, initialPrompt string) string {
	quotedDir := shellQuote(workdir)
	command := strings.TrimSpace(codexCommand)
	if command == "" {
		command = defaultCodexCommand
	}
	prompt := strings.TrimSpace(initialPrompt)
	guidePath := strings.TrimSpace(normalizePathForThread(agentGuidePath))

	baseCommand := fmt.Sprintf("cd %s", quotedDir)
	if guidePath != "" {
		baseCommand = fmt.Sprintf("%s && echo \"[codex-orchestrator] agent guide: %s\"", baseCommand, shellQuote(guidePath))
	}
	if prompt == "" {
		return fmt.Sprintf("%s && %s", baseCommand, command)
	}
	return fmt.Sprintf("%s && %s %s", baseCommand, command, shellQuote(prompt))
}

func (service *Service) defaultAgentsRunnerLaunchCommand(workdir string, sessionID int64, childThread store.Thread, role string, initialPrompt string) string {
	scriptPath := filepath.Join(service.repoPath, defaultAgentsRunnerScript)
	baseCommand := fmt.Sprintf(
		"%s %s --mode child --session-id %d --thread-id %d --role %s",
		defaultPythonCommand,
		shellQuote(scriptPath),
		sessionID,
		childThread.ID,
		shellQuote(role),
	)
	if strings.TrimSpace(initialPrompt) != "" {
		baseCommand = fmt.Sprintf("%s --initial-prompt %s", baseCommand, shellQuote(initialPrompt))
	}
	return fmt.Sprintf("cd %s && %s", shellQuote(workdir), baseCommand)
}

func (service *Service) ensureChildPaneCapacity(ctx context.Context, sessionID int64, parentThreadID int64, childSessionName string, maxConcurrentChildren int) error {
	filterParentThreadID := parentThreadID
	childThreads, err := service.store.ListThreads(ctx, store.ThreadFilter{
		SessionID:      sessionID,
		ParentThreadID: &filterParentThreadID,
	})
	if err != nil {
		return err
	}

	occupied := 0
	recyclable := make([]store.Thread, 0)
	for _, childThread := range childThreads {
		if childThread.TmuxPaneID == nil || strings.TrimSpace(*childThread.TmuxPaneID) == "" {
			continue
		}
		if childThread.TmuxSessionName == nil || strings.TrimSpace(*childThread.TmuxSessionName) != childSessionName {
			continue
		}
		paneID := strings.TrimSpace(*childThread.TmuxPaneID)
		exists, existsErr := service.tmuxPaneExists(ctx, paneID)
		if existsErr != nil {
			return existsErr
		}
		if !exists {
			_, _ = service.store.UpdateThread(ctx, childThread.ID, store.ThreadUpdateArgs{
				TmuxPaneID:     pointerToString(""),
				TmuxWindowName: pointerToString(""),
			})
			continue
		}
		occupied++
		if isChildThreadReusable(childThread.Status) {
			recyclable = append(recyclable, childThread)
		}
	}

	if occupied < maxConcurrentChildren {
		return nil
	}

	for _, candidate := range recyclable {
		paneID := strings.TrimSpace(valueOrEmpty(candidate.TmuxPaneID))
		if paneID == "" {
			continue
		}
		_, _ = service.runCommand(ctx, "tmux", "kill-pane", "-t", paneID)
		stoppedStatus := "stopped"
		_, _ = service.store.UpdateThread(ctx, candidate.ID, store.ThreadUpdateArgs{
			Status:         &stoppedStatus,
			TmuxPaneID:     pointerToString(""),
			TmuxWindowName: pointerToString(""),
		})
		occupied--
		if occupied < maxConcurrentChildren {
			return nil
		}
	}

	return fmt.Errorf("child thread limit reached: session_id=%d max=%d", sessionID, maxConcurrentChildren)
}

func (service *Service) tmuxPaneExists(ctx context.Context, paneID string) (bool, error) {
	if strings.TrimSpace(paneID) == "" {
		return false, nil
	}
	if _, err := service.runCommand(ctx, "tmux", "display-message", "-p", "-t", paneID, "#{pane_id}"); err != nil {
		errLower := strings.ToLower(err.Error())
		if strings.Contains(errLower, "can't find pane") || strings.Contains(errLower, "can't find window") || strings.Contains(errLower, "can't find session") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func normalizeWindowName(windowName string, fallback string) string {
	trimmedWindowName := strings.TrimSpace(windowName)
	if trimmedWindowName == "" {
		return fallback
	}
	return trimmedWindowName
}

func (service *Service) resolveAgentGuidePathForRole(role string, override string) string {
	resolved := strings.TrimSpace(override)
	if resolved != "" {
		return resolved
	}
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "session-root", "root", "orchestrator":
		return defaultRootAgentGuidePath
	case "merge-reviewer":
		return defaultMergeReviewerPath
	case "doc-mirror-manager":
		return defaultDocMirrorPath
	case "plan-architect":
		return defaultPlanArchitectPath
	default:
		return defaultWorkerGuidePath
	}
}

func (service *Service) readAgentTemplate(path string) string {
	normalized := normalizePathForThread(path)
	if strings.TrimSpace(normalized) == "" {
		return ""
	}
	if !filepath.IsAbs(normalized) {
		normalized = filepath.Join(service.repoPath, normalized)
	}
	content, err := os.ReadFile(normalized)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(content))
}

func (service *Service) defaultTaskSpecJSON(role string, title string, objective string, extra map[string]any) string {
	spec := map[string]any{
		"thread_role": strings.TrimSpace(role),
	}
	if strings.TrimSpace(title) != "" {
		spec["title"] = strings.TrimSpace(title)
	}
	if strings.TrimSpace(objective) != "" {
		spec["objective"] = strings.TrimSpace(objective)
	}
	for key, value := range extra {
		spec[key] = value
	}
	encoded, err := json.Marshal(spec)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func marshalInt64Slice(values []int64) string {
	if len(values) == 0 {
		return "[]"
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

func decodeJSONForPrompt(raw string) any {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return map[string]any{}
	}
	var decoded any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return trimmed
	}
	return decoded
}

func decodeInt64JSON(raw string) []int64 {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return []int64{}
	}
	values := make([]int64, 0)
	if err := json.Unmarshal([]byte(trimmed), &values); err != nil {
		return []int64{}
	}
	return values
}

func prettyJSON(value any) string {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func (service *Service) defaultChildPrompt(sessionID int64, rootThreadID int64, childThread store.Thread, input threadChildSpawnInput) string {
	objective := strings.TrimSpace(input.Objective)
	if objective == "" {
		objective = strings.TrimSpace(valueOrEmpty(childThread.Objective))
	}
	if objective == "" {
		objective = strings.TrimSpace(input.Title)
	}
	if objective == "" {
		objective = "execute the assigned case and report progress"
	}
	role := strings.TrimSpace(childThread.Role)
	if role == "" {
		role = "worker"
	}
	taskSpecJSON := strings.TrimSpace(valueOrEmpty(childThread.TaskSpecJSON))
	if taskSpecJSON == "" {
		taskSpecJSON = service.defaultTaskSpecJSON(role, strings.TrimSpace(valueOrEmpty(childThread.Title)), objective, nil)
	}
	templateText := service.readAgentTemplate(service.resolveAgentGuidePathForRole(role, input.AgentGuidePath))
	if templateText == "" {
		templateText = "# Child Worker\n- Execute the assigned scope and report back."
	}
	contextPayload := map[string]any{
		"thread": map[string]any{
			"role":           role,
			"session_id":     sessionID,
			"root_thread_id": rootThreadID,
			"thread_id":      childThread.ID,
			"title":          strings.TrimSpace(valueOrEmpty(childThread.Title)),
			"objective":      objective,
		},
		"scope": map[string]any{
			"task_ids": decodeInt64JSON(valueOrEmpty(childThread.ScopeTaskIDsJSON)),
			"case_ids": decodeInt64JSON(valueOrEmpty(childThread.ScopeCaseIDsJSON)),
			"node_ids": decodeInt64JSON(valueOrEmpty(childThread.ScopeNodeIDsJSON)),
		},
		"task_spec": decodeJSONForPrompt(taskSpecJSON),
	}

	return fmt.Sprintf(
		`%s

# Runtime Assignment
~~~json
%s
~~~

Execution rules:
1. Work only on this thread assignment and scope.
2. Read/update orchestrator state only for your scoped IDs and your own progress.
3. Report blockers and completion status back to the root thread.
4. Pause for user/root follow-up instead of expanding scope autonomously.`,
		templateText,
		prettyJSON(contextPayload),
	)
}

func (service *Service) runCommand(ctx context.Context, name string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, name, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %s failed: %w (%s)", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

type tmuxInstallResult struct {
	installed bool
	message   string
	tmuxPath  string
	attempts  []map[string]any
}

func (service *Service) tryInstallTmux(ctx context.Context) tmuxInstallResult {
	result := tmuxInstallResult{
		installed: false,
		message:   "no installer succeeded",
		attempts:  make([]map[string]any, 0),
	}

	installers := [][]string{
		{"brew", "install", "tmux"},
	}
	sudoPrefix := service.sudoPrefix()
	if hasBinary("apt-get") {
		installers = append(installers, append(append([]string{}, sudoPrefix...), "apt-get", "update"))
		installers = append(installers, append(append([]string{}, sudoPrefix...), "apt-get", "install", "-y", "tmux"))
	}
	if hasBinary("dnf") {
		installers = append(installers, append(append([]string{}, sudoPrefix...), "dnf", "install", "-y", "tmux"))
	}
	if hasBinary("yum") {
		installers = append(installers, append(append([]string{}, sudoPrefix...), "yum", "install", "-y", "tmux"))
	}
	if hasBinary("pacman") {
		installers = append(installers, append(append([]string{}, sudoPrefix...), "pacman", "-Sy", "--noconfirm", "tmux"))
	}

	for _, installCommand := range installers {
		if len(installCommand) == 0 {
			continue
		}
		binary := installCommand[0]
		if !hasBinary(binary) {
			continue
		}

		commandCtx, cancel := context.WithTimeout(ctx, defaultTmuxEnsureTimeout)
		cmd := exec.CommandContext(commandCtx, installCommand[0], installCommand[1:]...)
		output, err := cmd.CombinedOutput()
		cancel()

		attempt := map[string]any{
			"command": strings.Join(installCommand, " "),
			"output":  strings.TrimSpace(string(output)),
		}
		if err != nil {
			attempt["error"] = err.Error()
		}
		result.attempts = append(result.attempts, attempt)

		if err != nil {
			continue
		}
		if tmuxPath, lookupErr := exec.LookPath("tmux"); lookupErr == nil {
			result.installed = true
			result.tmuxPath = tmuxPath
			result.message = "tmux installed successfully"
			return result
		}
	}

	result.message = "automatic installation failed; manual installation required"
	return result
}

func (service *Service) sudoPrefix() []string {
	if runtime.GOOS != "linux" {
		return nil
	}
	if os.Geteuid() == 0 {
		return nil
	}
	if hasBinary("sudo") {
		return []string{"sudo", "-n"}
	}
	return nil
}

func (service *Service) manualTmuxInstallInstructions() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"brew install tmux",
		}
	case "linux":
		return []string{
			"sudo apt-get update && sudo apt-get install -y tmux",
			"sudo dnf install -y tmux",
			"sudo yum install -y tmux",
			"sudo pacman -Sy --noconfirm tmux",
		}
	default:
		return []string{
			"install tmux from your OS package manager",
		}
	}
}

func hasBinary(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func isTmuxReady(status map[string]any) bool {
	rawStatus, ok := status["status"]
	if !ok {
		return false
	}
	normalizedStatus := strings.TrimSpace(strings.ToLower(fmt.Sprint(rawStatus)))
	return normalizedStatus == "ready" || normalizedStatus == "installed"
}

func boolValueOrDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func intValueOrDefault(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}

func isChildThreadReusable(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "failed", "stopped", "cancelled":
		return true
	default:
		return false
	}
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func normalizeRawJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	return trimmed
}

func normalizeAgentOverride(raw json.RawMessage) string {
	return normalizeRawJSON(raw)
}

func shellQuote(value string) string {
	if strings.TrimSpace(value) == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func normalizePathForThread(path string) string {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return ""
	}
	if filepath.IsAbs(trimmedPath) {
		return trimmedPath
	}
	return filepath.Clean(trimmedPath)
}
