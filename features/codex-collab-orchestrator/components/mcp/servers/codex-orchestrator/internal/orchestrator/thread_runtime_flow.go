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
	defaultTmuxEnsureTimeout = 120 * time.Second
	defaultAgentGuidePath    = ".codex/agents/codex-collab-orchestrator/merge-reviewer.md"
	defaultRootWindowName    = "root"
	defaultChildWindowName   = "children"
	defaultCodexCommand      = "codex --no-alt-screen"
	defaultMaxChildThreads   = 6
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

func (service *Service) ensureRootThread(ctx context.Context, input threadRootEnsureInput) (map[string]any, error) {
	session, rootThread, tmuxResult, err := service.ensureRootThreadInternal(ctx, input)
	if err != nil {
		return nil, err
	}
	attachInfo, err := service.buildAttachInfo(ctx, session, &rootThread)
	if err != nil {
		return nil, err
	}
	childSessionName := service.resolveChildTmuxSessionName(input.ChildSessionName, rootThread.ID)

	return map[string]any{
		"session":            session,
		"root_thread":        rootThread,
		"tmux":               tmuxResult,
		"attach_info":        attachInfo,
		"child_tmux_session": childSessionName,
		"child_attach_hint":  fmt.Sprintf("tmux attach-session -t %s", childSessionName),
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

func (service *Service) interruptChildThread(ctx context.Context, input threadChildSignalInput) (map[string]any, error) {
	if input.ThreadID <= 0 {
		return nil, errors.New("thread_id is required")
	}
	thread, err := service.store.GetThreadByID(ctx, input.ThreadID)
	if err != nil {
		return nil, err
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
		agentGuidePath = defaultAgentGuidePath
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
	})
	if spawnErr != nil {
		failedState := "failed"
		_, _ = service.store.UpdateReviewJob(ctx, reviewJob.ID, store.ReviewJobUpdateArgs{
			State: &failedState,
		})
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

func (service *Service) ensureRootThreadInternal(ctx context.Context, input threadRootEnsureInput) (store.Session, store.Thread, map[string]any, error) {
	if input.SessionID <= 0 {
		return store.Session{}, store.Thread{}, nil, errors.New("session_id is required")
	}
	session, err := service.store.GetSessionByID(ctx, input.SessionID)
	if err != nil {
		return store.Session{}, store.Thread{}, nil, err
	}

	var rootThread *store.Thread
	if session.RootThreadID != nil {
		thread, threadErr := service.store.GetThreadByID(ctx, *session.RootThreadID)
		if threadErr == nil {
			rootThread = &thread
		}
	}
	if rootThread == nil {
		thread, threadErr := service.store.GetSessionRootThread(ctx, session.ID)
		if threadErr != nil {
			return store.Session{}, store.Thread{}, nil, threadErr
		}
		rootThread = thread
	}

	role := strings.TrimSpace(input.Role)
	if role == "" {
		role = "session-root"
	}

	if rootThread == nil {
		createdThread, createErr := service.store.CreateThread(ctx, store.ThreadCreateArgs{
			SessionID:      session.ID,
			ParentThreadID: nil,
			Role:           role,
			Status:         "planned",
			Title:          input.Title,
			Objective:      input.Objective,
			AgentGuidePath: input.AgentGuidePath,
		})
		if createErr != nil {
			return store.Session{}, store.Thread{}, nil, createErr
		}
		rootThread = &createdThread
	}

	tmuxSessionName := service.resolveRootTmuxSessionName(input.TmuxSessionName, session, rootThread.ID)
	rootWindowName := normalizeWindowName(input.TmuxWindowName, defaultRootWindowName)

	session, err = service.store.UpdateSession(ctx, session.ID, store.SessionUpdateArgs{
		RootThreadID:    &rootThread.ID,
		TmuxSessionName: &tmuxSessionName,
		RuntimeState:    pointerToString("thread_root_ready"),
	})
	if err != nil {
		return store.Session{}, store.Thread{}, nil, err
	}

	rootThreadUpdated, err := service.store.UpdateThread(ctx, rootThread.ID, store.ThreadUpdateArgs{
		TmuxSessionName: &tmuxSessionName,
		TmuxWindowName:  &rootWindowName,
	})
	if err != nil {
		return store.Session{}, store.Thread{}, nil, err
	}

	ensureTmux := boolValueOrDefault(input.EnsureTmux, true)
	if !ensureTmux {
		return session, rootThreadUpdated, map[string]any{"status": "skipped"}, nil
	}

	tmuxResult, err := service.ensureTmux(ctx, runtimeTmuxEnsureInput{
		SessionID:   &session.ID,
		AutoInstall: input.AutoInstall,
	})
	if err != nil {
		return store.Session{}, store.Thread{}, nil, err
	}

	if isTmuxReady(tmuxResult) {
		workdir, pathErr := service.resolveSessionRootPath(ctx, session)
		if pathErr != nil {
			return store.Session{}, store.Thread{}, nil, pathErr
		}
		rootPaneID, sessionCreated, sessionErr := service.ensureTmuxSession(ctx, tmuxSessionName, workdir, rootWindowName)
		if sessionErr != nil {
			return store.Session{}, store.Thread{}, nil, sessionErr
		}
		rootPaneID, sessionErr = service.normalizeRootSessionLayout(ctx, tmuxSessionName, rootWindowName)
		if sessionErr != nil {
			return store.Session{}, store.Thread{}, nil, sessionErr
		}
		runningStatus := "running"
		rootThreadUpdated, err = service.store.UpdateThread(ctx, rootThreadUpdated.ID, store.ThreadUpdateArgs{
			Status:          &runningStatus,
			TmuxSessionName: &tmuxSessionName,
			TmuxWindowName:  &rootWindowName,
			TmuxPaneID:      &rootPaneID,
		})
		if err != nil {
			return store.Session{}, store.Thread{}, nil, err
		}

		launchRootCodex := boolValueOrDefault(input.LaunchCodex, true)
		forceLaunch := boolValueOrDefault(input.ForceLaunch, false)
		shouldLaunch := launchRootCodex && (sessionCreated || forceLaunch || strings.TrimSpace(valueOrEmpty(rootThreadUpdated.LaunchCommand)) == "")
		if shouldLaunch {
			launchCommand := strings.TrimSpace(input.LaunchCommand)
			if launchCommand == "" {
				initialPrompt := strings.TrimSpace(input.InitialPrompt)
				if initialPrompt == "" {
					initialPrompt = defaultRootPrompt(session.ID, rootThreadUpdated, input)
				}
				launchCommand = service.defaultCodexLaunchCommand(workdir, input.CodexCommand, input.AgentGuidePath, initialPrompt)
			}
			if _, launchErr := service.runCommand(ctx, "tmux", "send-keys", "-t", rootPaneID, launchCommand, "C-m"); launchErr != nil {
				return store.Session{}, store.Thread{}, nil, launchErr
			}
			rootThreadUpdated, err = service.store.UpdateThread(ctx, rootThreadUpdated.ID, store.ThreadUpdateArgs{
				LaunchCommand: &launchCommand,
			})
			if err != nil {
				return store.Session{}, store.Thread{}, nil, err
			}
			tmuxResult["root_launch"] = "started"
		}
	}

	return session, rootThreadUpdated, tmuxResult, nil
}

func (service *Service) spawnChildThreadInternal(ctx context.Context, input threadChildSpawnInput) (store.Thread, map[string]any, map[string]any, error) {
	if input.SessionID <= 0 {
		return store.Thread{}, nil, nil, errors.New("session_id is required")
	}

	session, rootThread, tmuxResult, err := service.ensureRootThreadInternal(ctx, threadRootEnsureInput{
		SessionID:      input.SessionID,
		EnsureTmux:     input.EnsureTmux,
		AutoInstall:    input.AutoInstall,
		AgentGuidePath: input.AgentGuidePath,
		LaunchCodex:    pointerToBool(false),
	})
	if err != nil {
		return store.Thread{}, nil, nil, err
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
	agentOverride := normalizeAgentOverride(input.AgentOverride)
	createdThread, err := service.store.CreateThread(ctx, store.ThreadCreateArgs{
		SessionID:      input.SessionID,
		ParentThreadID: &parentThreadID,
		WorktreeID:     input.WorktreeID,
		Role:           role,
		Status:         "planned",
		Title:          input.Title,
		Objective:      input.Objective,
		AgentGuidePath: input.AgentGuidePath,
		AgentOverride:  agentOverride,
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

	childSessionName := service.resolveChildTmuxSessionName(input.TmuxSessionName, rootThread.ID)
	childWindowName := normalizeWindowName(input.TmuxWindowName, defaultChildWindowName)
	maxConcurrentChildren := intValueOrDefault(input.MaxConcurrentChildren, defaultMaxChildThreads)
	if maxConcurrentChildren <= 0 {
		maxConcurrentChildren = defaultMaxChildThreads
	}

	workdir, err := service.resolveThreadWorkdir(ctx, session, input.WorktreeID)
	if err != nil {
		return store.Thread{}, nil, nil, err
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
			initialPrompt = defaultChildPrompt(input.SessionID, rootThread.ID, createdThread, input)
		}
		launchCommand = service.defaultCodexLaunchCommand(workdir, input.CodexCommand, input.AgentGuidePath, initialPrompt)
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
	switchCommand := ""
	if sessionName != "" {
		attachCommand = fmt.Sprintf("tmux attach-session -t %s", sessionName)
		switchCommand = fmt.Sprintf("tmux switch-client -t %s", sessionName)
	}

	return map[string]any{
		"available":      sessionName != "",
		"tmux_session":   sessionName,
		"tmux_pane_id":   paneID,
		"attach_command": attachCommand,
		"switch_command": switchCommand,
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

func (service *Service) normalizeRootSessionLayout(ctx context.Context, sessionName string, windowName string) (string, error) {
	targetWindow := fmt.Sprintf("%s:0", sessionName)
	windowOutput, err := service.runCommand(ctx, "tmux", "list-windows", "-t", sessionName, "-F", "#{window_index}")
	if err != nil {
		return "", err
	}
	windowLines := strings.Split(strings.TrimSpace(windowOutput), "\n")
	for _, windowLine := range windowLines {
		windowIndex := strings.TrimSpace(windowLine)
		if windowIndex == "" || windowIndex == "0" {
			continue
		}
		_, _ = service.runCommand(ctx, "tmux", "kill-window", "-t", fmt.Sprintf("%s:%s", sessionName, windowIndex))
	}
	if strings.TrimSpace(windowName) != "" {
		if _, err := service.runCommand(ctx, "tmux", "rename-window", "-t", targetWindow, windowName); err != nil {
			return "", err
		}
	}

	panesOutput, err := service.runCommand(ctx, "tmux", "list-panes", "-t", targetWindow, "-F", "#{pane_id}")
	if err != nil {
		return "", err
	}
	paneLines := strings.Split(strings.TrimSpace(panesOutput), "\n")
	if len(paneLines) == 0 || strings.TrimSpace(paneLines[0]) == "" {
		return "", fmt.Errorf("tmux window has no panes: %s", targetWindow)
	}
	rootPaneID := strings.TrimSpace(paneLines[0])
	for _, paneLine := range paneLines[1:] {
		paneID := strings.TrimSpace(paneLine)
		if paneID == "" {
			continue
		}
		_, _ = service.runCommand(ctx, "tmux", "kill-pane", "-t", paneID)
	}
	return rootPaneID, nil
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

func (service *Service) resolveRootTmuxSessionName(override string, session store.Session, rootThreadID int64) string {
	sessionName := strings.TrimSpace(override)
	if sessionName != "" {
		return sessionName
	}
	if session.TmuxSessionName != nil && strings.TrimSpace(*session.TmuxSessionName) != "" {
		return strings.TrimSpace(*session.TmuxSessionName)
	}
	return fmt.Sprintf("codex-root-%d", rootThreadID)
}

func (service *Service) resolveChildTmuxSessionName(override string, rootThreadID int64) string {
	sessionName := strings.TrimSpace(override)
	if sessionName != "" {
		return sessionName
	}
	return fmt.Sprintf("codex-child-%d", rootThreadID)
}

func normalizeWindowName(windowName string, fallback string) string {
	trimmedWindowName := strings.TrimSpace(windowName)
	if trimmedWindowName == "" {
		return fallback
	}
	return trimmedWindowName
}

func defaultRootPrompt(sessionID int64, rootThread store.Thread, input threadRootEnsureInput) string {
	objective := strings.TrimSpace(input.Objective)
	if objective == "" {
		objective = strings.TrimSpace(valueOrEmpty(rootThread.Objective))
	}
	if objective == "" {
		objective = "orchestrate the requested work from this root thread"
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = strings.TrimSpace(valueOrEmpty(rootThread.Title))
	}
	if title == "" {
		title = "root orchestration"
	}

	return fmt.Sprintf(
		"You are the root orchestrator thread for codex-orchestrator session %d.\nTask: %s\nObjective: %s\n\nOperate from this root tmux session. Create child threads for implementation, keep state updated through MCP methods, and wait for direct user instructions when blocked.",
		sessionID,
		title,
		objective,
	)
}

func defaultChildPrompt(sessionID int64, rootThreadID int64, childThread store.Thread, input threadChildSpawnInput) string {
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
	return fmt.Sprintf(
		"You are child thread %d under root thread %d for session %d.\nTask: %s\n\nWork only on this assignment, update progress via orchestrator state/checkpoints, and pause for user or root-thread follow-up when blocked.",
		childThread.ID,
		rootThreadID,
		sessionID,
		objective,
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

func normalizeAgentOverride(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	return trimmed
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
