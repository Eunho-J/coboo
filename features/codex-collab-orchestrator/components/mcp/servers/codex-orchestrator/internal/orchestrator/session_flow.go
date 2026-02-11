package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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

	sessionRootBranch := fmt.Sprintf("session/%d", updatedSession.ID)
	sessionRootPath := filepath.Join(service.repoPath, ".codex-orch", "worktrees", fmt.Sprintf("session-%d", updatedSession.ID))

	if err := service.runGitWorktreeAdd(sessionRootPath, sessionRootBranch, mainWorktree.Branch); err != nil {
		return nil, err
	}

	sessionRootWorktree, err := service.store.CreateWorktreeRecord(ctx, store.WorktreeCreateArgs{
		TaskID:         0,
		Path:           sessionRootPath,
		Branch:         sessionRootBranch,
		Status:         "active",
		Kind:           "session_root",
		ParentWorktree: &mainWorktree.ID,
		OwnerSessionID: &updatedSession.ID,
		MergeState:     "active",
	})
	if err != nil {
		return nil, err
	}

	updatedSession, err = service.store.UpdateSession(ctx, updatedSession.ID, store.SessionUpdateArgs{
		SessionRootWorktreeID: &sessionRootWorktree.ID,
		Status:                pointerToString("active_new"),
	})
	if err != nil {
		return nil, err
	}

	contextState, err := service.store.BuildSessionContext(ctx, updatedSession.ID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"session_context": contextState,
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

	branch := strings.TrimSpace(input.Branch)
	if branch == "" {
		branch = fmt.Sprintf("task/%d/%d", input.SessionID, time.Now().Unix())
	}
	worktreePath := strings.TrimSpace(input.Path)
	if worktreePath == "" {
		worktreePath = filepath.Join(service.repoPath, ".codex-orch", "worktrees", sanitizeForPath(branch))
	}

	baseRef := strings.TrimSpace(input.BaseRef)
	if baseRef == "" {
		baseRef = parentWorktree.Branch
	}
	createOnDisk := true
	if input.CreateOnDisk != nil {
		createOnDisk = *input.CreateOnDisk
	}

	if createOnDisk {
		if err := service.runGitWorktreeAdd(worktreePath, branch, baseRef); err != nil {
			return store.Worktree{}, err
		}
	}

	status := "planned"
	if createOnDisk {
		status = "active"
	}
	taskID := int64(0)
	if input.TaskID != nil {
		taskID = *input.TaskID
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
