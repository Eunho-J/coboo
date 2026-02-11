package store

import "encoding/json"

type Task struct {
	ID              int64   `json:"id"`
	Level           string  `json:"level"`
	ParentID        *int64  `json:"parent_id,omitempty"`
	Title           string  `json:"title"`
	Status          string  `json:"status"`
	Priority        int     `json:"priority"`
	AssigneeSession *string `json:"assignee_session,omitempty"`
	InputContract   *string `json:"input_contract,omitempty"`
	Fixtures        *string `json:"fixtures,omitempty"`
	NextAction      *string `json:"next_action,omitempty"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

type Step struct {
	ID         int64  `json:"id"`
	TaskID     int64  `json:"task_id"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	Evidence   string `json:"evidence"`
	OrderNo    int64  `json:"order_no"`
	RecordedAt string `json:"recorded_at"`
}

type Checkpoint struct {
	ID         int64  `json:"id"`
	TaskID     int64  `json:"task_id"`
	StepTitle  string `json:"step_title"`
	Snapshot   string `json:"snapshot"`
	RecordedAt string `json:"recorded_at"`
}

type Lock struct {
	ID           int64  `json:"id"`
	ScopeType    string `json:"scope_type"`
	ScopePath    string `json:"scope_path"`
	OwnerSession string `json:"owner_session"`
	LeaseUntil   string `json:"lease_until"`
	HeartbeatAt  string `json:"heartbeat_at"`
	State        string `json:"state"`
}

type Worktree struct {
	ID             int64   `json:"id"`
	TaskID         int64   `json:"task_id"`
	Path           string  `json:"path"`
	Branch         string  `json:"branch"`
	Status         string  `json:"status"`
	Kind           *string `json:"kind,omitempty"`
	ParentWorktree *int64  `json:"parent_worktree_id,omitempty"`
	OwnerSessionID *int64  `json:"owner_session_id,omitempty"`
	MergeState     *string `json:"merge_state,omitempty"`
	CreatedAt      string  `json:"created_at"`
	MergedAt       string  `json:"merged_at,omitempty"`
}

type MergeRequest struct {
	ID              int64   `json:"id"`
	FeatureTaskID   int64   `json:"feature_task_id"`
	Status          string  `json:"status"`
	ReviewerSession *string `json:"reviewer_session,omitempty"`
	NotesJSON       *string `json:"notes_json,omitempty"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

type MirrorStatus struct {
	DBVersion int64  `json:"db_version"`
	MDVersion int64  `json:"md_version"`
	MDPath    string `json:"md_path"`
	Outdated  bool   `json:"outdated"`
	UpdatedAt string `json:"updated_at"`
}

type ResumeState struct {
	Task       *Task       `json:"task,omitempty"`
	Checkpoint *Checkpoint `json:"checkpoint,omitempty"`
}

type Session struct {
	ID                     int64   `json:"id"`
	AgentRole              string  `json:"agent_role"`
	Owner                  string  `json:"owner"`
	RepoPath               *string `json:"repo_path,omitempty"`
	TerminalFingerprint    *string `json:"terminal_fingerprint,omitempty"`
	Intent                 *string `json:"intent,omitempty"`
	MainWorktreeID         *int64  `json:"main_worktree_id,omitempty"`
	SessionRootWorktreeID  *int64  `json:"session_root_worktree_id,omitempty"`
	RootThreadID           *int64  `json:"root_thread_id,omitempty"`
	TmuxSessionName        *string `json:"tmux_session_name,omitempty"`
	RuntimeState           *string `json:"runtime_state,omitempty"`
	DelegationState        *string `json:"delegation_state,omitempty"`
	DelegationRootThreadID *int64  `json:"delegation_root_thread_id,omitempty"`
	DelegationIssuedAt     *string `json:"delegation_issued_at,omitempty"`
	DelegationAckedAt      *string `json:"delegation_acked_at,omitempty"`
	StartedAt              string  `json:"started_at"`
	LastSeenAt             string  `json:"last_seen_at"`
	Status                 string  `json:"status"`
}

type ResumeCandidate struct {
	Session     Session     `json:"session"`
	CurrentRef  *CurrentRef `json:"current_ref,omitempty"`
	SessionRoot *Worktree   `json:"session_root_worktree,omitempty"`
}

type SessionContext struct {
	Session      Session     `json:"session"`
	MainWorktree *Worktree   `json:"main_worktree,omitempty"`
	SessionRoot  *Worktree   `json:"session_root_worktree,omitempty"`
	CurrentRef   *CurrentRef `json:"current_ref,omitempty"`
}

type CurrentRef struct {
	ID                int64   `json:"id"`
	SessionID         int64   `json:"session_id"`
	NodeType          string  `json:"node_type"`
	NodeID            int64   `json:"node_id"`
	CheckpointID      *int64  `json:"checkpoint_id,omitempty"`
	Mode              string  `json:"mode"`
	Status            string  `json:"status"`
	NextAction        *string `json:"next_action,omitempty"`
	Summary           *string `json:"summary,omitempty"`
	RequiredFilesJSON *string `json:"required_files_json,omitempty"`
	AckedAt           *string `json:"acked_at,omitempty"`
	Version           int64   `json:"version"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
}

type MainMergeQueueItem struct {
	ID             int64   `json:"id"`
	SessionID      int64   `json:"session_id"`
	FromWorktreeID int64   `json:"from_worktree_id"`
	TargetBranch   string  `json:"target_branch"`
	State          string  `json:"state"`
	StartedAt      *string `json:"started_at,omitempty"`
	CompletedAt    *string `json:"completed_at,omitempty"`
	ErrorMessage   *string `json:"error_message,omitempty"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
}

type MainMergeLock struct {
	ID              int64   `json:"id"`
	HolderSessionID *int64  `json:"holder_session_id,omitempty"`
	LeaseUntil      *string `json:"lease_until,omitempty"`
	State           string  `json:"state"`
	UpdatedAt       string  `json:"updated_at"`
}

type Thread struct {
	ID               int64   `json:"id"`
	SessionID        int64   `json:"session_id"`
	ParentThreadID   *int64  `json:"parent_thread_id,omitempty"`
	Role             string  `json:"role"`
	Status           string  `json:"status"`
	Title            *string `json:"title,omitempty"`
	Objective        *string `json:"objective,omitempty"`
	WorktreeID       *int64  `json:"worktree_id,omitempty"`
	AgentGuidePath   *string `json:"agent_guide_path,omitempty"`
	AgentOverride    *string `json:"agent_override,omitempty"`
	TaskSpecJSON     *string `json:"task_spec_json,omitempty"`
	ScopeTaskIDsJSON *string `json:"scope_task_ids_json,omitempty"`
	ScopeCaseIDsJSON *string `json:"scope_case_ids_json,omitempty"`
	ScopeNodeIDsJSON *string `json:"scope_node_ids_json,omitempty"`
	TmuxSessionName  *string `json:"tmux_session_name,omitempty"`
	TmuxWindowName   *string `json:"tmux_window_name,omitempty"`
	TmuxPaneID       *string `json:"tmux_pane_id,omitempty"`
	LaunchCommand    *string `json:"launch_command,omitempty"`
	CreatedAt        string  `json:"created_at"`
	StartedAt        *string `json:"started_at,omitempty"`
	CompletedAt      *string `json:"completed_at,omitempty"`
	UpdatedAt        string  `json:"updated_at"`
}

type ReviewJob struct {
	ID               int64   `json:"id"`
	MergeRequestID   int64   `json:"merge_request_id"`
	SessionID        int64   `json:"session_id"`
	ReviewerThreadID *int64  `json:"reviewer_thread_id,omitempty"`
	State            string  `json:"state"`
	NotesJSON        *string `json:"notes_json,omitempty"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
	CompletedAt      *string `json:"completed_at,omitempty"`
}

type RuntimePrereqEvent struct {
	ID          int64   `json:"id"`
	SessionID   *int64  `json:"session_id,omitempty"`
	Requirement string  `json:"requirement"`
	Status      string  `json:"status"`
	Detail      *string `json:"detail,omitempty"`
	CreatedAt   string  `json:"created_at"`
}

type GraphNode struct {
	ID                int64   `json:"id"`
	NodeType          string  `json:"node_type"`
	Facet             string  `json:"facet"`
	Title             string  `json:"title"`
	Status            string  `json:"status"`
	Priority          int     `json:"priority"`
	ParentID          *int64  `json:"parent_id,omitempty"`
	WorktreeID        *int64  `json:"worktree_id,omitempty"`
	OwnerSessionID    *int64  `json:"owner_session_id,omitempty"`
	Summary           *string `json:"summary,omitempty"`
	RiskLevel         *int    `json:"risk_level,omitempty"`
	TokenEstimate     *int    `json:"token_estimate,omitempty"`
	AffectedFilesJSON *string `json:"affected_files_json,omitempty"`
	ApprovalState     string  `json:"approval_state"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
}

type GraphEdge struct {
	ID         int64  `json:"id"`
	FromNodeID int64  `json:"from_node_id"`
	ToNodeID   int64  `json:"to_node_id"`
	EdgeType   string `json:"edge_type"`
	CreatedAt  string `json:"created_at"`
}

type NodeChecklistItem struct {
	ID        int64  `json:"id"`
	NodeID    int64  `json:"node_id"`
	ItemText  string `json:"item_text"`
	Status    string `json:"status"`
	OrderNo   int64  `json:"order_no"`
	Facet     string `json:"facet"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type NodeSnapshot struct {
	ID                int64   `json:"id"`
	NodeID            int64   `json:"node_id"`
	SnapshotType      string  `json:"snapshot_type"`
	Summary           *string `json:"summary,omitempty"`
	AffectedFilesJSON *string `json:"affected_files_json,omitempty"`
	NextAction        *string `json:"next_action,omitempty"`
	CreatedAt         string  `json:"created_at"`
}

type PlanningRule struct {
	MaxTokenPerSlice   int    `json:"max_token_per_slice"`
	MaxFilesPerSlice   int    `json:"max_files_per_slice"`
	ReplanTriggersJSON string `json:"replan_triggers_json"`
	ApprovalPolicy     string `json:"approval_policy"`
	UpdatedAt          string `json:"updated_at"`
}

type TaskCreateArgs struct {
	Level           string
	Title           string
	ParentID        *int64
	Priority        int
	AssigneeSession string
}

type TaskFilter struct {
	Level    string
	Status   string
	ParentID *int64
}

type CaseBeginArgs struct {
	TaskID        int64
	InputContract json.RawMessage
	Fixtures      []string
}

type StepCheckArgs struct {
	TaskID    int64
	StepTitle string
	Result    string
	Artifacts []string
}

type CaseCompleteArgs struct {
	TaskID     int64
	Summary    string
	NextAction string
}

type LockAcquireArgs struct {
	ScopeType    string
	ScopePath    string
	OwnerSession string
	TTLSeconds   int
}

type WorktreeCreateArgs struct {
	TaskID         int64
	Path           string
	Branch         string
	Status         string
	Kind           string
	ParentWorktree *int64
	OwnerSessionID *int64
	MergeState     string
}

type MergeRequestArgs struct {
	FeatureTaskID   int64
	ReviewerSession string
	NotesJSON       json.RawMessage
}

type SessionOpenArgs struct {
	AgentRole           string
	Owner               string
	RepoPath            string
	TerminalFingerprint string
	Intent              string
}

type SessionUpdateArgs struct {
	Status                 *string
	MainWorktreeID         *int64
	SessionRootWorktreeID  *int64
	RootThreadID           *int64
	TmuxSessionName        *string
	RuntimeState           *string
	Intent                 *string
	DelegationState        *string
	DelegationRootThreadID *int64
	DelegationIssuedAt     *string
	DelegationAckedAt      *string
}

type WorkCurrentRefUpsertArgs struct {
	SessionID         int64
	NodeType          string
	NodeID            int64
	CheckpointID      *int64
	Mode              string
	Status            string
	NextAction        string
	Summary           string
	RequiredFilesJSON string
}

type MainMergeRequestArgs struct {
	SessionID      int64
	FromWorktreeID int64
	TargetBranch   string
}

type ThreadCreateArgs struct {
	SessionID        int64
	ParentThreadID   *int64
	Role             string
	Status           string
	Title            string
	Objective        string
	WorktreeID       *int64
	AgentGuidePath   string
	AgentOverride    string
	TaskSpecJSON     string
	ScopeTaskIDsJSON string
	ScopeCaseIDsJSON string
	ScopeNodeIDsJSON string
}

type ThreadFilter struct {
	SessionID      int64
	ParentThreadID *int64
	Status         string
	Role           string
}

type ThreadUpdateArgs struct {
	Status           *string
	TmuxSessionName  *string
	TmuxWindowName   *string
	TmuxPaneID       *string
	LaunchCommand    *string
	TaskSpecJSON     *string
	ScopeTaskIDsJSON *string
	ScopeCaseIDsJSON *string
	ScopeNodeIDsJSON *string
}

type ReviewJobCreateArgs struct {
	MergeRequestID   int64
	SessionID        int64
	ReviewerThreadID *int64
	State            string
	NotesJSON        json.RawMessage
}

type ReviewJobUpdateArgs struct {
	State            *string
	ReviewerThreadID *int64
	NotesJSON        *json.RawMessage
}

type RuntimePrereqEventCreateArgs struct {
	SessionID   *int64
	Requirement string
	Status      string
	Detail      string
}

type GraphNodeCreateArgs struct {
	NodeType          string
	Facet             string
	Title             string
	Status            string
	Priority          int
	ParentID          *int64
	WorktreeID        *int64
	OwnerSessionID    *int64
	Summary           string
	RiskLevel         *int
	TokenEstimate     *int
	AffectedFilesJSON string
	ApprovalState     string
}

type GraphNodeFilter struct {
	NodeType string
	Facet    string
	Status   string
	ParentID *int64
}

type GraphEdgeCreateArgs struct {
	FromNodeID int64
	ToNodeID   int64
	EdgeType   string
}

type NodeChecklistUpsertArgs struct {
	NodeID   int64
	ItemText string
	Status   string
	OrderNo  int64
	Facet    string
}

type NodeSnapshotCreateArgs struct {
	NodeID            int64
	SnapshotType      string
	Summary           string
	AffectedFilesJSON string
	NextAction        string
}
