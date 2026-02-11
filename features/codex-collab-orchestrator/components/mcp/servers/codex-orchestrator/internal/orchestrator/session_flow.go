package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cayde/llm/features/codex-collab-orchestrator/components/mcp/servers/codex-orchestrator/internal/store"
)

func (service *Service) openSession(ctx context.Context, input sessionOpenInput) (map[string]any, error) {
	session, err := service.store.OpenSession(ctx, store.SessionOpenArgs{
		AgentRole:           input.AgentRole,
		Owner:               input.Owner,
		RepoPath:            service.repoPath,
		TerminalFingerprint: input.TerminalFingerprint,
		Intent:              input.Intent,
	})
	if err != nil {
		return nil, err
	}

	mainBranch, err := service.currentGitBranch()
	if err != nil {
		mainBranch = defaultMainBranch
	}
	mainWorktree, err := service.store.CreateOrGetMainWorktree(ctx, service.repoPath, mainBranch)
	if err != nil {
		return nil, err
	}

	intent := strings.TrimSpace(strings.ToLower(input.Intent))
	if intent == "" || intent == "auto" {
		intent = "new_work"
	}

	updatedSession, err := service.store.UpdateSession(ctx, session.ID, store.SessionUpdateArgs{
		MainWorktreeID: &mainWorktree.ID,
		Intent:         pointerToString(intent),
		Status:         pointerToString("opened"),
	})
	if err != nil {
		return nil, err
	}

	if intent == "resume_work" {
		updatedSession, err = service.store.UpdateSession(ctx, updatedSession.ID, store.SessionUpdateArgs{
			Status: pointerToString("awaiting_resume"),
		})
		if err != nil {
			return nil, err
		}
		candidates, candidateErr := service.store.ListResumeCandidates(ctx, service.repoPath, updatedSession.ID, input.HeartbeatTimeoutSeconds)
		if candidateErr != nil {
			return nil, candidateErr
		}
		return map[string]any{
			"session":           updatedSession,
			"main_worktree":     mainWorktree,
			"action_required":   "select_resume_candidate",
			"resume_candidates": candidates,
		}, nil
	}

	alwaysBranch := true
	if input.AlwaysBranch != nil {
		alwaysBranch = *input.AlwaysBranch
	}
	if !alwaysBranch {
		return nil, errors.New("always_branch=false is not supported in root-local mode")
	}

	preferredSlug := deriveWorktreeSlug(input.WorktreeName, input.UserRequest)
	sessionRootWorktree, resolvedSlug, err := service.createSessionRootWorktree(ctx, updatedSession.ID, mainWorktree, preferredSlug)
	if err != nil {
		return nil, err
	}
	viewerSessionName := service.buildViewerTmuxSessionName(sessionRootWorktree.Path)

	updatedSession, err = service.store.UpdateSession(ctx, updatedSession.ID, store.SessionUpdateArgs{
		SessionRootWorktreeID: &sessionRootWorktree.ID,
		Status:                pointerToString("active_new"),
		TmuxSessionName:       &viewerSessionName,
		RuntimeState:          pointerToString("root_local_ready"),
	})
	if err != nil {
		return nil, err
	}

	contextState, err := service.store.BuildSessionContext(ctx, updatedSession.ID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"session_context":     contextState,
		"root_mode":           "caller_cli",
		"worktree_slug":       resolvedSlug,
		"viewer_tmux_session": viewerSessionName,
		"child_attach_hint":   fmt.Sprintf("tmux attach -r -t %s", viewerSessionName),
	}, nil
}

func (service *Service) spawnWorktree(ctx context.Context, input worktreeSpawnInput) (store.Worktree, error) {
	parentWorktree, err := service.store.GetWorktreeByID(ctx, input.ParentWorktreeID)
	if err != nil {
		return store.Worktree{}, err
	}
	if parentWorktree.OwnerSessionID != nil && *parentWorktree.OwnerSessionID != input.SessionID {
		return store.Worktree{}, fmt.Errorf("parent worktree belongs to another session: %d", *parentWorktree.OwnerSessionID)
	}

	baseRef := strings.TrimSpace(input.BaseRef)
	if baseRef == "" {
		baseRef = parentWorktree.Branch
	}
	createOnDisk := true
	if input.CreateOnDisk != nil {
		createOnDisk = *input.CreateOnDisk
	}

	taskID := int64(0)
	if input.TaskID != nil {
		taskID = *input.TaskID
	}
	slug := deriveWorktreeSlug(input.Slug, input.Reason)
	branch := strings.TrimSpace(input.Branch)
	worktreePath := strings.TrimSpace(input.Path)
	if branch == "" {
		branch = fmt.Sprintf("task/%d/%s", input.SessionID, slug)
	}
	if worktreePath == "" {
		worktreePath = filepath.Join(service.repoPath, ".codex-orch", "worktrees", slug)
	}

	if createOnDisk {
		candidateResolved := false
		for attempt := 0; attempt < 64; attempt++ {
			candidateSlug := slugWithSuffix(slug, attempt)
			candidateBranch := branch
			candidatePath := worktreePath
			if strings.TrimSpace(input.Branch) == "" {
				candidateBranch = fmt.Sprintf("task/%d/%s", input.SessionID, candidateSlug)
			}
			if strings.TrimSpace(input.Path) == "" {
				candidatePath = filepath.Join(service.repoPath, ".codex-orch", "worktrees", candidateSlug)
			}
			if service.worktreeCandidateTaken(candidatePath, candidateBranch) {
				continue
			}
			if err := service.runGitWorktreeAdd(candidatePath, candidateBranch, baseRef); err != nil {
				if isLikelyWorktreeConflictError(err) && strings.TrimSpace(input.Branch) == "" && strings.TrimSpace(input.Path) == "" {
					continue
				}
				return store.Worktree{}, err
			}
			branch = candidateBranch
			worktreePath = candidatePath
			candidateResolved = true
			break
		}
		if !candidateResolved {
			return store.Worktree{}, fmt.Errorf("unable to allocate unique worktree for slug=%s", slug)
		}
	}

	status := "planned"
	if createOnDisk {
		status = "active"
	}

	return service.store.CreateWorktreeRecord(ctx, store.WorktreeCreateArgs{
		TaskID:         taskID,
		Path:           worktreePath,
		Branch:         branch,
		Status:         status,
		Kind:           "task_branch",
		ParentWorktree: &input.ParentWorktreeID,
		OwnerSessionID: &input.SessionID,
		MergeState:     "active",
	})
}

func (service *Service) createSessionRootWorktree(ctx context.Context, sessionID int64, mainWorktree store.Worktree, preferredSlug string) (store.Worktree, string, error) {
	slug := deriveWorktreeSlug(preferredSlug, fmt.Sprintf("task-%d", sessionID))
	for attempt := 0; attempt < 64; attempt++ {
		candidateSlug := slugWithSuffix(slug, attempt)
		candidateBranch := fmt.Sprintf("task/%d/%s", sessionID, candidateSlug)
		candidatePath := filepath.Join(service.repoPath, ".codex-orch", "worktrees", candidateSlug)
		if service.worktreeCandidateTaken(candidatePath, candidateBranch) {
			continue
		}
		if err := service.runGitWorktreeAdd(candidatePath, candidateBranch, mainWorktree.Branch); err != nil {
			if isLikelyWorktreeConflictError(err) {
				continue
			}
			return store.Worktree{}, "", err
		}

		record, err := service.store.CreateWorktreeRecord(ctx, store.WorktreeCreateArgs{
			TaskID:         0,
			Path:           candidatePath,
			Branch:         candidateBranch,
			Status:         "active",
			Kind:           "session_root",
			ParentWorktree: &mainWorktree.ID,
			OwnerSessionID: &sessionID,
			MergeState:     "active",
		})
		if err != nil {
			return store.Worktree{}, "", err
		}
		return record, candidateSlug, nil
	}

	return store.Worktree{}, "", fmt.Errorf("unable to allocate unique session worktree for session=%d", sessionID)
}

func (service *Service) worktreeCandidateTaken(worktreePath string, branch string) bool {
	if strings.TrimSpace(worktreePath) != "" {
		if _, err := os.Stat(worktreePath); err == nil {
			return true
		}
	}
	if strings.TrimSpace(branch) == "" {
		return false
	}
	command := exec.Command("git", "-C", service.repoPath, "show-ref", "--verify", "--quiet", fmt.Sprintf("refs/heads/%s", branch))
	if err := command.Run(); err == nil {
		return true
	}
	return false
}

func isLikelyWorktreeConflictError(err error) bool {
	normalized := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(normalized, "already exists") ||
		strings.Contains(normalized, "already checked out") ||
		strings.Contains(normalized, "already used by worktree")
}

func (service *Service) mergeWorktreeToParent(ctx context.Context, input worktreeMergeToParentInput) (map[string]any, error) {
	childWorktree, err := service.store.GetWorktreeByID(ctx, input.WorktreeID)
	if err != nil {
		return nil, err
	}
	if childWorktree.ParentWorktree == nil {
		return nil, fmt.Errorf("worktree has no parent: %d", childWorktree.ID)
	}
	if childWorktree.OwnerSessionID != nil && *childWorktree.OwnerSessionID != input.SessionID {
		return nil, fmt.Errorf("worktree belongs to another session: %d", *childWorktree.OwnerSessionID)
	}

	parentWorktree, err := service.store.GetWorktreeByID(ctx, *childWorktree.ParentWorktree)
	if err != nil {
		return nil, err
	}

	if err := service.runGitMerge(parentWorktree.Path, childWorktree.Branch); err != nil {
		return nil, err
	}

	updatedChild, err := service.store.MarkWorktreeMergedToParent(ctx, childWorktree.ID)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"child_worktree":  updatedChild,
		"parent_worktree": parentWorktree,
		"result":          "merged_to_parent",
	}, nil
}

func (service *Service) currentRef(ctx context.Context, input workCurrentRefInput) (map[string]any, error) {
	currentRef, err := service.store.GetCurrentRef(ctx, input.SessionID, true)
	if err != nil {
		return nil, err
	}
	if currentRef != nil {
		return map[string]any{
			"source":      "current_refs",
			"current_ref": currentRef,
		}, nil
	}

	resumeState, err := service.store.ResumeNextCase(ctx)
	if err != nil {
		return nil, err
	}
	if resumeState.Task == nil {
		return map[string]any{
			"source":      "none",
			"current_ref": nil,
		}, nil
	}

	requiredFilesJSON := marshalStringSlice(input.RequiredFiles)
	var checkpointID *int64
	if resumeState.Checkpoint != nil {
		checkpointID = &resumeState.Checkpoint.ID
	}
	createdRef, err := service.store.UpsertCurrentRef(ctx, store.WorkCurrentRefUpsertArgs{
		SessionID:         input.SessionID,
		NodeType:          "case",
		NodeID:            resumeState.Task.ID,
		CheckpointID:      checkpointID,
		Mode:              input.Mode,
		Status:            "active",
		NextAction:        "resume from latest checkpoint",
		Summary:           resumeState.Task.Title,
		RequiredFilesJSON: requiredFilesJSON,
	})
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"source":      "resume.next",
		"current_ref": createdRef,
		"task":        resumeState.Task,
		"checkpoint":  resumeState.Checkpoint,
	}, nil
}

func (service *Service) currentGitBranch() (string, error) {
	command := exec.Command("git", "-C", service.repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to detect current git branch: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	branch := strings.TrimSpace(string(output))
	if branch == "" {
		return "", fmt.Errorf("empty branch returned from git")
	}
	return branch, nil
}

func (service *Service) runGitMerge(worktreePath string, branch string) error {
	command := exec.Command("git", "-C", worktreePath, "merge", "--no-ff", "--no-edit", branch)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git merge failed: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func pointerToString(value string) *string {
	return &value
}

func pointerToBool(value bool) *bool {
	return &value
}

func marshalStringSlice(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}
