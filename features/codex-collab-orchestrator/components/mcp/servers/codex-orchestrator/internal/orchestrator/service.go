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

const (
	docMirrorManagerRole = "doc-mirror-manager"
	defaultMirrorPath    = ".codex-orch/mirror/status.md"
	defaultMainBranch    = "main"
)

type Service struct {
	repoPath string
	store    *store.Store
}

func NewService(repoPath string) (*Service, error) {
	absoluteRepoPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve repo path: %w", err)
	}

	databasePath := filepath.Join(absoluteRepoPath, ".codex-orch", "state.db")
	stateStore, err := store.Open(databasePath)
	if err != nil {
		return nil, err
	}

	return &Service{
		repoPath: absoluteRepoPath,
		store:    stateStore,
	}, nil
}

func (service *Service) Close() error {
	return service.store.Close()
}

func (service *Service) Handle(ctx context.Context, method string, rawParams json.RawMessage) (any, error) {
	switch method {
	case "workspace.init":
		return map[string]any{
			"repo_path": service.repoPath,
			"db_path":   service.store.DBPath(),
		}, nil
	case "session.open":
		var input sessionOpenInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.openSession(ctx, input)
	case "session.heartbeat":
		var input sessionHeartbeatInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.store.HeartbeatSession(ctx, input.SessionID)
	case "session.close":
		var input sessionCloseInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.store.CloseSession(ctx, input.SessionID)
	case "session.context":
		var input sessionContextInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.store.BuildSessionContext(ctx, input.SessionID)
	case "runtime.tmux.ensure":
		var input runtimeTmuxEnsureInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.ensureTmux(ctx, input)
	case "runtime.bundle.info":
		var input runtimeBundleInfoInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.runtimeBundleInfo(ctx, input)
	case "orchestration.delegate":
		var input orchestrationDelegateInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.delegateOrchestration(ctx, input)
	case "task.create":
		var input taskCreateInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.store.CreateTask(ctx, store.TaskCreateArgs{
			Level:           input.Level,
			Title:           input.Title,
			ParentID:        input.ParentID,
			Priority:        input.Priority,
			AssigneeSession: input.AssigneeSession,
		})
	case "task.list":
		var input taskListInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.store.ListTasks(ctx, store.TaskFilter{
			Level:    input.Level,
			Status:   input.Status,
			ParentID: input.ParentID,
		})
	case "task.get":
		var input taskGetInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.store.GetTaskByID(ctx, input.TaskID)
	case "graph.node.create":
		var input graphNodeCreateInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.store.CreateGraphNode(ctx, store.GraphNodeCreateArgs{
			NodeType:          input.NodeType,
			Facet:             input.Facet,
			Title:             input.Title,
			Status:            input.Status,
			Priority:          input.Priority,
			ParentID:          input.ParentID,
			WorktreeID:        input.WorktreeID,
			OwnerSessionID:    input.OwnerSessionID,
			Summary:           input.Summary,
			RiskLevel:         input.RiskLevel,
			TokenEstimate:     input.TokenEstimate,
			AffectedFilesJSON: marshalStringSlice(input.AffectedFiles),
			ApprovalState:     input.ApprovalState,
		})
	case "graph.node.list":
		var input graphNodeListInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.store.ListGraphNodes(ctx, store.GraphNodeFilter{
			NodeType: input.NodeType,
			Facet:    input.Facet,
			Status:   input.Status,
			ParentID: input.ParentID,
		})
	case "graph.edge.create":
		var input graphEdgeCreateInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.store.CreateGraphEdge(ctx, store.GraphEdgeCreateArgs{
			FromNodeID: input.FromNodeID,
			ToNodeID:   input.ToNodeID,
			EdgeType:   input.EdgeType,
		})
	case "graph.checklist.upsert":
		var input graphChecklistUpsertInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.store.UpsertNodeChecklistItem(ctx, store.NodeChecklistUpsertArgs{
			NodeID:   input.NodeID,
			ItemText: input.ItemText,
			Status:   input.Status,
			OrderNo:  input.OrderNo,
			Facet:    input.Facet,
		})
	case "graph.snapshot.create":
		var input graphSnapshotCreateInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.store.CreateNodeSnapshot(ctx, store.NodeSnapshotCreateArgs{
			NodeID:            input.NodeID,
			SnapshotType:      input.SnapshotType,
			Summary:           input.Summary,
			AffectedFilesJSON: marshalStringSlice(input.AffectedFiles),
			NextAction:        input.NextAction,
		})
	case "plan.bootstrap":
		var input planBootstrapInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.planBootstrap(ctx, input)
	case "plan.slice.generate":
		var input planSliceGenerateInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.planSliceGenerate(ctx, input)
	case "plan.slice.replan":
		var input planSliceReplanInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.planSliceReplan(ctx, input)
	case "plan.rollup.preview":
		var input planRollupPreviewInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.store.RollupPreview(ctx, input.ParentNodeID)
	case "plan.rollup.submit":
		var input planRollupSubmitInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.planRollupSubmit(ctx, input)
	case "plan.rollup.approve":
		var input planRollupApproveInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.store.UpdateGraphNodeApprovalState(ctx, input.NodeID, "approved", "done")
	case "plan.rollup.reject":
		var input planRollupRejectInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.store.UpdateGraphNodeApprovalState(ctx, input.NodeID, "rejected", "blocked")
	case "scheduler.decide_worktree":
		var input worktreeDecisionInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return decideWorktree(input), nil
	case "worktree.create":
		var input worktreeCreateInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.createWorktree(ctx, input)
	case "worktree.list":
		return service.store.ListWorktrees(ctx)
	case "worktree.spawn":
		var input worktreeSpawnInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.spawnWorktree(ctx, input)
	case "worktree.merge_to_parent":
		var input worktreeMergeToParentInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.mergeWorktreeToParent(ctx, input)
	case "thread.root.ensure":
		var input threadRootEnsureInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.ensureRootThread(ctx, input)
	case "thread.root.handoff_ack":
		var input threadRootHandoffAckInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.ackRootHandoff(ctx, input)
	case "thread.child.spawn":
		var input threadChildSpawnInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		if err := service.requireDelegationAck(ctx, input.SessionID, "thread.child.spawn"); err != nil {
			return nil, err
		}
		return service.spawnChildThread(ctx, input)
	case "thread.child.list":
		var input threadChildListInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.listChildThreads(ctx, input)
	case "thread.child.interrupt":
		var input threadChildSignalInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.interruptChildThread(ctx, input)
	case "thread.child.stop":
		var input threadChildStopInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.stopChildThread(ctx, input)
	case "thread.attach_info":
		var input threadAttachInfoInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.threadAttachInfo(ctx, input)
	case "lock.acquire":
		var input lockAcquireInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.store.AcquireLock(ctx, store.LockAcquireArgs{
			ScopeType:    input.ScopeType,
			ScopePath:    input.ScopePath,
			OwnerSession: input.OwnerSession,
			TTLSeconds:   input.TTLSeconds,
		})
	case "lock.heartbeat":
		var input lockHeartbeatInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.store.HeartbeatLock(ctx, input.LockID, input.TTLSeconds)
	case "lock.release":
		var input lockReleaseInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.store.ReleaseLock(ctx, input.LockID)
	case "case.begin":
		var input caseBeginInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		if err := service.requireDelegationAck(ctx, input.SessionID, "case.begin"); err != nil {
			return nil, err
		}
		caseTask, err := service.store.BeginCase(ctx, store.CaseBeginArgs{
			TaskID:        input.CaseID,
			InputContract: input.InputContract,
			Fixtures:      input.Fixtures,
		})
		if err != nil {
			return nil, err
		}
		if input.SessionID > 0 {
			requiredFilesJSON := marshalStringSlice(input.RequiredFiles)
			_, _ = service.store.UpsertCurrentRef(ctx, store.WorkCurrentRefUpsertArgs{
				SessionID:         input.SessionID,
				NodeType:          "case",
				NodeID:            input.CaseID,
				Mode:              "compact",
				Status:            "active",
				NextAction:        "run first pending step",
				Summary:           "case.begin recorded",
				RequiredFilesJSON: requiredFilesJSON,
			})
		}
		return caseTask, nil
	case "step.check":
		var input stepCheckInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		if err := service.requireDelegationAck(ctx, input.SessionID, "step.check"); err != nil {
			return nil, err
		}
		stepResult, err := service.store.AddStepCheck(ctx, store.StepCheckArgs{
			TaskID:    input.CaseID,
			StepTitle: input.StepTitle,
			Result:    input.Result,
			Artifacts: input.Artifacts,
		})
		if err != nil {
			return nil, err
		}
		if input.SessionID > 0 {
			requiredFilesJSON := marshalStringSlice(input.RequiredFiles)
			_, _ = service.store.UpsertCurrentRef(ctx, store.WorkCurrentRefUpsertArgs{
				SessionID:         input.SessionID,
				NodeType:          "case",
				NodeID:            input.CaseID,
				Mode:              "compact",
				Status:            "active",
				NextAction:        "continue next step",
				Summary:           fmt.Sprintf("last step checked: %s", input.StepTitle),
				RequiredFilesJSON: requiredFilesJSON,
			})
		}
		return stepResult, nil
	case "case.complete":
		var input caseCompleteInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		if err := service.requireDelegationAck(ctx, input.SessionID, "case.complete"); err != nil {
			return nil, err
		}
		completedCase, err := service.store.CompleteCase(ctx, store.CaseCompleteArgs{
			TaskID:     input.CaseID,
			Summary:    input.Summary,
			NextAction: input.NextAction,
		})
		if err != nil {
			return nil, err
		}
		if input.SessionID > 0 {
			requiredFilesJSON := marshalStringSlice(input.RequiredFiles)
			_, _ = service.store.UpsertCurrentRef(ctx, store.WorkCurrentRefUpsertArgs{
				SessionID:         input.SessionID,
				NodeType:          "case",
				NodeID:            input.CaseID,
				Mode:              "compact",
				Status:            "completed",
				NextAction:        input.NextAction,
				Summary:           input.Summary,
				RequiredFilesJSON: requiredFilesJSON,
			})
		}
		return completedCase, nil
	case "resume.next":
		return service.store.ResumeNextCase(ctx)
	case "resume.candidates.list":
		var input resumeCandidatesListInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.store.ListResumeCandidates(ctx, service.repoPath, input.RequesterSessionID, input.HeartbeatTimeoutSeconds)
	case "resume.candidates.attach":
		var input resumeCandidatesAttachInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.store.AttachResumeCandidate(ctx, input.RequesterSessionID, input.TargetSessionID)
	case "work.current_ref":
		var input workCurrentRefInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		if err := service.requireDelegationAck(ctx, input.SessionID, "work.current_ref"); err != nil {
			return nil, err
		}
		return service.currentRef(ctx, input)
	case "work.current_ref.ack":
		var input workCurrentRefAckInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		if err := service.requireDelegationAck(ctx, input.SessionID, "work.current_ref.ack"); err != nil {
			return nil, err
		}
		return service.store.AckCurrentRef(ctx, input.SessionID, input.RefID)
	case "merge.request":
		var input mergeRequestInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.store.CreateMergeRequest(ctx, store.MergeRequestArgs{
			FeatureTaskID:   input.FeatureTaskID,
			ReviewerSession: input.ReviewerSession,
			NotesJSON:       input.NotesJSON,
		})
	case "merge.review_context":
		var input mergeReviewContextInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.mergeReviewContext(ctx, input.MergeRequestID)
	case "merge.review.request_auto":
		var input mergeReviewRequestAutoInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		if err := service.requireDelegationAck(ctx, input.SessionID, "merge.review.request_auto"); err != nil {
			return nil, err
		}
		return service.requestAutoMergeReview(ctx, input)
	case "merge.review.thread_status":
		var input mergeReviewThreadStatusInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.mergeReviewThreadStatus(ctx, input)
	case "merge.main.request":
		var input mergeMainRequestInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		if err := service.requireDelegationAck(ctx, input.SessionID, "merge.main.request"); err != nil {
			return nil, err
		}
		mainMergeRequest, err := service.store.EnqueueMainMergeRequest(ctx, store.MainMergeRequestArgs{
			SessionID:      input.SessionID,
			FromWorktreeID: input.FromWorktreeID,
			TargetBranch:   input.TargetBranch,
		})
		if err != nil {
			return nil, err
		}

		response := map[string]any{
			"main_merge_request": mainMergeRequest,
		}

		autoReview := false
		if input.MergeRequestID != nil {
			autoReview = true
		}
		if input.AutoReview != nil {
			autoReview = *input.AutoReview
		}
		if autoReview {
			if input.MergeRequestID == nil || *input.MergeRequestID <= 0 {
				return nil, errors.New("merge_request_id is required when auto_review=true")
			}
			autoReviewResponse, autoReviewErr := service.requestAutoMergeReview(ctx, mergeReviewRequestAutoInput{
				SessionID:      input.SessionID,
				MergeRequestID: *input.MergeRequestID,
				ReviewerRole:   input.ReviewerRole,
				AgentGuidePath: input.AgentGuidePath,
				EnsureTmux:     pointerToBool(true),
				AutoInstall:    pointerToBool(true),
			})
			if autoReviewErr != nil {
				response["review_dispatch_error"] = autoReviewErr.Error()
			} else {
				response["review_dispatch"] = autoReviewResponse
			}
		}
		return response, nil
	case "merge.main.next":
		return service.store.NextMainMergeRequest(ctx)
	case "merge.main.status":
		var input mergeMainStatusInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.store.GetMainMergeRequest(ctx, input.RequestID)
	case "merge.main.acquire_lock":
		var input mergeMainAcquireLockInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		if err := service.requireDelegationAck(ctx, input.SessionID, "merge.main.acquire_lock"); err != nil {
			return nil, err
		}
		return service.store.AcquireMainMergeLock(ctx, input.SessionID, input.TTLSeconds)
	case "merge.main.release_lock":
		var input mergeMainReleaseLockInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		if err := service.requireDelegationAck(ctx, input.SessionID, "merge.main.release_lock"); err != nil {
			return nil, err
		}
		return service.store.ReleaseMainMergeLock(ctx, input.SessionID)
	case "mirror.status":
		return service.store.GetMirrorStatus(ctx)
	case "mirror.refresh":
		var input mirrorRefreshInput
		if err := decodeParams(rawParams, &input); err != nil {
			return nil, err
		}
		return service.refreshMirror(ctx, input)
	default:
		return nil, fmt.Errorf("unsupported method: %s", method)
	}
}

func (service *Service) createWorktree(ctx context.Context, input worktreeCreateInput) (store.Worktree, error) {
	worktreePath := input.Path
	if strings.TrimSpace(worktreePath) == "" {
		branchSlug := sanitizeForPath(input.Branch)
		worktreePath = filepath.Join(service.repoPath, ".codex-orch", "worktrees", branchSlug)
	}

	if input.CreateOnDisk {
		if err := service.runGitWorktreeAdd(worktreePath, input.Branch, input.BaseRef); err != nil {
			return store.Worktree{}, err
		}
	}

	status := "planned"
	if input.CreateOnDisk {
		status = "active"
	}

	return service.store.CreateWorktreeRecord(ctx, store.WorktreeCreateArgs{
		TaskID: input.TaskID,
		Path:   worktreePath,
		Branch: input.Branch,
		Status: status,
	})
}

func (service *Service) runGitWorktreeAdd(worktreePath string, branch string, baseRef string) error {
	if strings.TrimSpace(branch) == "" {
		return errors.New("branch is required when create_on_disk=true")
	}

	if err := os.MkdirAll(filepath.Dir(worktreePath), 0o755); err != nil {
		return fmt.Errorf("failed to create worktree parent directory: %w", err)
	}

	args := []string{"-C", service.repoPath, "worktree", "add", "-b", branch, worktreePath}
	if strings.TrimSpace(baseRef) != "" {
		args = append(args, baseRef)
	} else {
		args = append(args, "HEAD")
	}

	command := exec.Command("git", args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add failed: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (service *Service) mergeReviewContext(ctx context.Context, mergeRequestID int64) (map[string]any, error) {
	mergeRequest, err := service.store.GetMergeRequest(ctx, mergeRequestID)
	if err != nil {
		return nil, err
	}

	featureTask, err := service.store.GetTaskByID(ctx, mergeRequest.FeatureTaskID)
	if err != nil {
		return nil, err
	}

	childTasks, err := service.store.ListTasks(ctx, store.TaskFilter{
		ParentID: &featureTask.ID,
	})
	if err != nil {
		return nil, err
	}

	checkpoints := make(map[int64]*store.Checkpoint)
	for _, childTask := range childTasks {
		if strings.EqualFold(childTask.Level, "case") {
			latestCheckpoint, checkpointError := service.store.GetLatestCheckpoint(ctx, childTask.ID)
			if checkpointError != nil {
				return nil, checkpointError
			}
			checkpoints[childTask.ID] = latestCheckpoint
		}
	}

	return map[string]any{
		"merge_request": mergeRequest,
		"feature":       featureTask,
		"children":      childTasks,
		"checkpoints":   checkpoints,
	}, nil
}

func (service *Service) refreshMirror(ctx context.Context, input mirrorRefreshInput) (map[string]any, error) {
	if strings.TrimSpace(input.RequesterRole) != docMirrorManagerRole {
		return nil, fmt.Errorf("mirror.refresh is restricted to role=%s", docMirrorManagerRole)
	}

	status, err := service.store.GetMirrorStatus(ctx)
	if err != nil {
		return nil, err
	}

	targetPath := strings.TrimSpace(input.TargetPath)
	if targetPath == "" {
		if strings.TrimSpace(status.MDPath) != "" {
			targetPath = status.MDPath
		} else {
			targetPath = filepath.Join(service.repoPath, defaultMirrorPath)
		}
	}

	taskStatusCounts, err := service.store.GetTaskStatusCounts(ctx)
	if err != nil {
		return nil, err
	}

	activeLocks, err := service.store.ListActiveLocks(ctx)
	if err != nil {
		return nil, err
	}

	if err := writeMirrorMarkdown(targetPath, status.DBVersion, taskStatusCounts, activeLocks); err != nil {
		return nil, err
	}

	updatedStatus, err := service.store.MarkMirrorRefreshed(ctx, targetPath)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"outdated_before_refresh": status.Outdated,
		"mirror_status":           updatedStatus,
		"path":                    targetPath,
	}, nil
}

func writeMirrorMarkdown(targetPath string, dbVersion int64, taskStatusCounts map[string]int64, activeLocks []store.Lock) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("failed to create mirror directory: %w", err)
	}

	builder := strings.Builder{}
	builder.WriteString("# Codex Orchestrator State Mirror\n\n")
	builder.WriteString(fmt.Sprintf("- DB version: `%d`\n", dbVersion))
	builder.WriteString(fmt.Sprintf("- Active locks: `%d`\n\n", len(activeLocks)))

	builder.WriteString("## Task status counts\n\n")
	if len(taskStatusCounts) == 0 {
		builder.WriteString("- (none)\n")
	} else {
		for status, count := range taskStatusCounts {
			builder.WriteString(fmt.Sprintf("- %s: %d\n", status, count))
		}
	}

	builder.WriteString("\n## Active locks\n\n")
	if len(activeLocks) == 0 {
		builder.WriteString("- (none)\n")
	} else {
		for _, lock := range activeLocks {
			builder.WriteString(fmt.Sprintf("- #%d `%s:%s` owner=%s lease_until=%s\n", lock.ID, lock.ScopeType, lock.ScopePath, lock.OwnerSession, lock.LeaseUntil))
		}
	}

	if err := os.WriteFile(targetPath, []byte(builder.String()), 0o644); err != nil {
		return fmt.Errorf("failed to write mirror file: %w", err)
	}
	return nil
}

func sanitizeForPath(value string) string {
	trimmedValue := strings.TrimSpace(value)
	if trimmedValue == "" {
		return "worktree"
	}
	normalized := strings.ReplaceAll(trimmedValue, "/", "-")
	normalized = strings.ReplaceAll(normalized, " ", "-")
	return normalized
}

func decodeParams(rawParams json.RawMessage, destination any) error {
	if len(rawParams) == 0 {
		rawParams = []byte("{}")
	}
	if err := json.Unmarshal(rawParams, destination); err != nil {
		return fmt.Errorf("invalid params: %w", err)
	}
	return nil
}

type taskCreateInput struct {
	Level           string `json:"level"`
	Title           string `json:"title"`
	ParentID        *int64 `json:"parent_id"`
	Priority        int    `json:"priority"`
	AssigneeSession string `json:"assignee_session"`
}

type sessionOpenInput struct {
	AgentRole               string `json:"agent_role"`
	Owner                   string `json:"owner"`
	TerminalFingerprint     string `json:"terminal_fingerprint"`
	Intent                  string `json:"intent"`
	HeartbeatTimeoutSeconds int    `json:"heartbeat_timeout_seconds"`
}

type sessionHeartbeatInput struct {
	SessionID int64 `json:"session_id"`
}

type sessionCloseInput struct {
	SessionID int64 `json:"session_id"`
}

type sessionContextInput struct {
	SessionID int64 `json:"session_id"`
}

type taskListInput struct {
	Level    string `json:"level"`
	Status   string `json:"status"`
	ParentID *int64 `json:"parent_id"`
}

type taskGetInput struct {
	TaskID int64 `json:"task_id"`
}

type worktreeDecisionInput struct {
	ChangedFiles     int `json:"changed_files"`
	EstimateMinutes  int `json:"estimate_minutes"`
	Risk             int `json:"risk"`
	ParallelWorkers  int `json:"parallel_workers"`
	ConflictingPaths int `json:"conflicting_paths"`
}

type worktreeDecisionResult struct {
	Mode    string   `json:"mode"`
	Score   int      `json:"score"`
	Reasons []string `json:"reasons"`
}

func decideWorktree(input worktreeDecisionInput) worktreeDecisionResult {
	score := 0
	reasons := make([]string, 0, 5)

	if input.ChangedFiles > 0 {
		score += input.ChangedFiles
		reasons = append(reasons, fmt.Sprintf("changed_files=%d", input.ChangedFiles))
	}
	if input.EstimateMinutes > 0 {
		estimateScore := (input.EstimateMinutes + 14) / 15
		score += estimateScore
		reasons = append(reasons, fmt.Sprintf("estimate_minutes=%d(+%d)", input.EstimateMinutes, estimateScore))
	}
	if input.Risk > 0 {
		riskScore := input.Risk * 3
		score += riskScore
		reasons = append(reasons, fmt.Sprintf("risk=%d(+%d)", input.Risk, riskScore))
	}
	if input.ParallelWorkers > 0 {
		parallelScore := input.ParallelWorkers * 2
		score += parallelScore
		reasons = append(reasons, fmt.Sprintf("parallel_workers=%d(+%d)", input.ParallelWorkers, parallelScore))
	}
	if input.ConflictingPaths > 0 {
		conflictScore := input.ConflictingPaths * 2
		score += conflictScore
		reasons = append(reasons, fmt.Sprintf("conflicting_paths=%d(+%d)", input.ConflictingPaths, conflictScore))
	}

	mode := "shared"
	if score >= 12 {
		mode = "worktree"
	}

	return worktreeDecisionResult{
		Mode:    mode,
		Score:   score,
		Reasons: reasons,
	}
}

type worktreeCreateInput struct {
	TaskID       int64  `json:"task_id"`
	Branch       string `json:"branch"`
	Path         string `json:"path"`
	BaseRef      string `json:"base_ref"`
	CreateOnDisk bool   `json:"create_on_disk"`
}

type worktreeSpawnInput struct {
	SessionID        int64  `json:"session_id"`
	ParentWorktreeID int64  `json:"parent_worktree_id"`
	TaskID           *int64 `json:"task_id"`
	Reason           string `json:"reason"`
	Branch           string `json:"branch"`
	Path             string `json:"path"`
	BaseRef          string `json:"base_ref"`
	CreateOnDisk     *bool  `json:"create_on_disk"`
}

type worktreeMergeToParentInput struct {
	SessionID  int64 `json:"session_id"`
	WorktreeID int64 `json:"worktree_id"`
}

type lockAcquireInput struct {
	ScopeType    string `json:"scope_type"`
	ScopePath    string `json:"scope_path"`
	OwnerSession string `json:"owner_session"`
	TTLSeconds   int    `json:"ttl_seconds"`
}

type lockHeartbeatInput struct {
	LockID     int64 `json:"lock_id"`
	TTLSeconds int   `json:"ttl_seconds"`
}

type lockReleaseInput struct {
	LockID int64 `json:"lock_id"`
}

type caseBeginInput struct {
	CaseID        int64           `json:"case_id"`
	SessionID     int64           `json:"session_id"`
	InputContract json.RawMessage `json:"input_contract"`
	Fixtures      []string        `json:"fixtures"`
	RequiredFiles []string        `json:"required_files"`
}

type stepCheckInput struct {
	CaseID        int64    `json:"case_id"`
	SessionID     int64    `json:"session_id"`
	StepTitle     string   `json:"step_title"`
	Result        string   `json:"result"`
	Artifacts     []string `json:"artifacts"`
	RequiredFiles []string `json:"required_files"`
}

type caseCompleteInput struct {
	CaseID        int64    `json:"case_id"`
	SessionID     int64    `json:"session_id"`
	Summary       string   `json:"summary"`
	NextAction    string   `json:"next_action"`
	RequiredFiles []string `json:"required_files"`
}

type resumeCandidatesListInput struct {
	RequesterSessionID      int64 `json:"requester_session_id"`
	HeartbeatTimeoutSeconds int   `json:"heartbeat_timeout_seconds"`
}

type resumeCandidatesAttachInput struct {
	RequesterSessionID int64 `json:"requester_session_id"`
	TargetSessionID    int64 `json:"target_session_id"`
}

type workCurrentRefInput struct {
	SessionID     int64    `json:"session_id"`
	Mode          string   `json:"mode"`
	RequiredFiles []string `json:"required_files"`
}

type workCurrentRefAckInput struct {
	SessionID int64 `json:"session_id"`
	RefID     int64 `json:"ref_id"`
}

type mergeRequestInput struct {
	FeatureTaskID   int64           `json:"feature_task_id"`
	ReviewerSession string          `json:"reviewer_session"`
	NotesJSON       json.RawMessage `json:"notes_json"`
}

type mergeReviewContextInput struct {
	MergeRequestID int64 `json:"merge_request_id"`
}

type mergeMainRequestInput struct {
	SessionID      int64  `json:"session_id"`
	FromWorktreeID int64  `json:"from_worktree_id"`
	TargetBranch   string `json:"target_branch"`
	MergeRequestID *int64 `json:"merge_request_id"`
	AutoReview     *bool  `json:"auto_review"`
	ReviewerRole   string `json:"reviewer_role"`
	AgentGuidePath string `json:"agent_guide_path"`
}

type mergeMainStatusInput struct {
	RequestID int64 `json:"request_id"`
}

type mergeMainAcquireLockInput struct {
	SessionID  int64 `json:"session_id"`
	TTLSeconds int   `json:"ttl_seconds"`
}

type mergeMainReleaseLockInput struct {
	SessionID int64 `json:"session_id"`
}

type mirrorRefreshInput struct {
	RequesterRole string `json:"requester_role"`
	TargetPath    string `json:"target_path"`
}

type runtimeTmuxEnsureInput struct {
	SessionID   *int64 `json:"session_id"`
	AutoInstall *bool  `json:"auto_install"`
}

type runtimeBundleInfoInput struct{}

type orchestrationDelegateInput struct {
	SessionID             int64           `json:"session_id"`
	Title                 string          `json:"title"`
	Objective             string          `json:"objective"`
	UserRequest           string          `json:"user_request"`
	AgentGuidePath        string          `json:"agent_guide_path"`
	InitialPrompt         string          `json:"initial_prompt"`
	CodexCommand          string          `json:"codex_command"`
	EnsureTmux            *bool           `json:"ensure_tmux"`
	AutoInstall           *bool           `json:"auto_install"`
	TmuxSessionName       string          `json:"tmux_session_name"`
	TmuxWindowName        string          `json:"tmux_window_name"`
	ChildSessionName      string          `json:"child_session_name"`
	MaxConcurrentChildren *int            `json:"max_concurrent_children"`
	TaskSpec              json.RawMessage `json:"task_spec"`
	ScopeTaskIDs          []int64         `json:"scope_task_ids"`
	ScopeCaseIDs          []int64         `json:"scope_case_ids"`
	ScopeNodeIDs          []int64         `json:"scope_node_ids"`
}

type threadRootEnsureInput struct {
	SessionID             int64           `json:"session_id"`
	Role                  string          `json:"role"`
	Title                 string          `json:"title"`
	Objective             string          `json:"objective"`
	EnsureTmux            *bool           `json:"ensure_tmux"`
	AutoInstall           *bool           `json:"auto_install"`
	AgentGuidePath        string          `json:"agent_guide_path"`
	TmuxSessionName       string          `json:"tmux_session_name"`
	TmuxWindowName        string          `json:"tmux_window_name"`
	InitialPrompt         string          `json:"initial_prompt"`
	LaunchCommand         string          `json:"launch_command"`
	CodexCommand          string          `json:"codex_command"`
	LaunchCodex           *bool           `json:"launch_codex"`
	ForceLaunch           *bool           `json:"force_launch"`
	ChildSessionName      string          `json:"child_session_name"`
	MaxConcurrentChildren *int            `json:"max_concurrent_children"`
	TaskSpec              json.RawMessage `json:"task_spec"`
	ScopeTaskIDs          []int64         `json:"scope_task_ids"`
	ScopeCaseIDs          []int64         `json:"scope_case_ids"`
	ScopeNodeIDs          []int64         `json:"scope_node_ids"`
}

type threadRootHandoffAckInput struct {
	SessionID int64  `json:"session_id"`
	ThreadID  int64  `json:"thread_id"`
	State     string `json:"state"`
}

type threadChildSpawnInput struct {
	SessionID             int64           `json:"session_id"`
	ParentThreadID        *int64          `json:"parent_thread_id"`
	WorktreeID            *int64          `json:"worktree_id"`
	Role                  string          `json:"role"`
	Title                 string          `json:"title"`
	Objective             string          `json:"objective"`
	AgentGuidePath        string          `json:"agent_guide_path"`
	AgentOverride         json.RawMessage `json:"agent_override"`
	LaunchCommand         string          `json:"launch_command"`
	SplitDirection        string          `json:"split_direction"`
	EnsureTmux            *bool           `json:"ensure_tmux"`
	AutoInstall           *bool           `json:"auto_install"`
	TmuxSessionName       string          `json:"tmux_session_name"`
	TmuxWindowName        string          `json:"tmux_window_name"`
	InitialPrompt         string          `json:"initial_prompt"`
	CodexCommand          string          `json:"codex_command"`
	LaunchCodex           *bool           `json:"launch_codex"`
	MaxConcurrentChildren *int            `json:"max_concurrent_children"`
	TaskSpec              json.RawMessage `json:"task_spec"`
	ScopeTaskIDs          []int64         `json:"scope_task_ids"`
	ScopeCaseIDs          []int64         `json:"scope_case_ids"`
	ScopeNodeIDs          []int64         `json:"scope_node_ids"`
}

type threadChildListInput struct {
	SessionID      int64  `json:"session_id"`
	ParentThreadID *int64 `json:"parent_thread_id"`
	Status         string `json:"status"`
	Role           string `json:"role"`
}

type threadChildSignalInput struct {
	ThreadID int64 `json:"thread_id"`
}

type threadChildStopInput struct {
	ThreadID      int64 `json:"thread_id"`
	TerminatePane *bool `json:"terminate_pane"`
}

type threadAttachInfoInput struct {
	SessionID int64  `json:"session_id"`
	ThreadID  *int64 `json:"thread_id"`
}

type mergeReviewRequestAutoInput struct {
	SessionID      int64           `json:"session_id"`
	MergeRequestID int64           `json:"merge_request_id"`
	ReviewerRole   string          `json:"reviewer_role"`
	AgentGuidePath string          `json:"agent_guide_path"`
	AgentOverride  json.RawMessage `json:"agent_override"`
	EnsureTmux     *bool           `json:"ensure_tmux"`
	AutoInstall    *bool           `json:"auto_install"`
}

type mergeReviewThreadStatusInput struct {
	ReviewJobID    *int64 `json:"review_job_id"`
	MergeRequestID *int64 `json:"merge_request_id"`
}
