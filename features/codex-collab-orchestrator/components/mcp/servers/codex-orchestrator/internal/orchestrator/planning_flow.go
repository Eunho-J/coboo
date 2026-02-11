package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"github.com/cayde/llm/features/codex-collab-orchestrator/components/mcp/servers/codex-orchestrator/internal/store"
)

func (service *Service) planBootstrap(ctx context.Context, input planBootstrapInput) (map[string]any, error) {
	if input.InitiativeTitle == "" {
		return nil, errors.New("initiative_title is required")
	}
	if input.PlanTitle == "" {
		return nil, errors.New("plan_title is required")
	}

	initiative, err := service.store.CreateGraphNode(ctx, store.GraphNodeCreateArgs{
		NodeType:       "initiative",
		Facet:          "planning",
		Title:          input.InitiativeTitle,
		Status:         "in_progress",
		Priority:       input.Priority,
		OwnerSessionID: input.OwnerSessionID,
		Summary:        input.Summary,
	})
	if err != nil {
		return nil, err
	}

	planNode, err := service.store.CreateGraphNode(ctx, store.GraphNodeCreateArgs{
		NodeType:       "plan",
		Facet:          "planning",
		Title:          input.PlanTitle,
		Status:         "todo",
		Priority:       input.Priority,
		ParentID:       &initiative.ID,
		OwnerSessionID: input.OwnerSessionID,
	})
	if err != nil {
		return nil, err
	}

	edge, edgeErr := service.store.CreateGraphEdge(ctx, store.GraphEdgeCreateArgs{
		FromNodeID: initiative.ID,
		ToNodeID:   planNode.ID,
		EdgeType:   "contains",
	})
	if edgeErr != nil {
		return nil, edgeErr
	}

	return map[string]any{
		"initiative": initiative,
		"plan":       planNode,
		"edge":       edge,
	}, nil
}

func (service *Service) planSliceGenerate(ctx context.Context, input planSliceGenerateInput) (map[string]any, error) {
	if input.PlanNodeID <= 0 {
		return nil, errors.New("plan_node_id is required")
	}
	if len(input.SliceSpecs) == 0 {
		return nil, errors.New("slice_specs is required")
	}

	createdSlices := make([]store.GraphNode, 0, len(input.SliceSpecs))
	createdEdges := make([]store.GraphEdge, 0, len(input.SliceSpecs))
	for _, spec := range input.SliceSpecs {
		if spec.Title == "" {
			return nil, errors.New("slice_specs[].title is required")
		}
		var tokenEstimate *int
		if spec.TokenEstimate > 0 {
			tokenEstimate = &spec.TokenEstimate
		}

		sliceNode, err := service.store.CreateGraphNode(ctx, store.GraphNodeCreateArgs{
			NodeType:          "slice",
			Facet:             "planning",
			Title:             spec.Title,
			Status:            "todo",
			Priority:          spec.Priority,
			ParentID:          &input.PlanNodeID,
			OwnerSessionID:    input.OwnerSessionID,
			TokenEstimate:     tokenEstimate,
			AffectedFilesJSON: marshalStringSlice(spec.AffectedFiles),
			Summary:           spec.Summary,
		})
		if err != nil {
			return nil, err
		}
		createdSlices = append(createdSlices, sliceNode)

		edge, edgeErr := service.store.CreateGraphEdge(ctx, store.GraphEdgeCreateArgs{
			FromNodeID: input.PlanNodeID,
			ToNodeID:   sliceNode.ID,
			EdgeType:   "contains",
		})
		if edgeErr != nil {
			return nil, edgeErr
		}
		createdEdges = append(createdEdges, edge)
	}

	return map[string]any{
		"plan_node_id": input.PlanNodeID,
		"slices":       createdSlices,
		"edges":        createdEdges,
	}, nil
}

func (service *Service) planSliceReplan(ctx context.Context, input planSliceReplanInput) (store.NodeSnapshot, error) {
	if input.NodeID <= 0 {
		return store.NodeSnapshot{}, errors.New("node_id is required")
	}
	if input.Reason == "" {
		return store.NodeSnapshot{}, errors.New("reason is required")
	}
	return service.store.CreateNodeSnapshot(ctx, store.NodeSnapshotCreateArgs{
		NodeID:            input.NodeID,
		SnapshotType:      "replan",
		Summary:           input.Reason,
		AffectedFilesJSON: marshalStringSlice(input.AffectedFiles),
		NextAction:        input.NextAction,
	})
}

func (service *Service) planRollupSubmit(ctx context.Context, input planRollupSubmitInput) (map[string]any, error) {
	if input.NodeID <= 0 {
		return nil, errors.New("node_id is required")
	}
	snapshot, err := service.store.CreateNodeSnapshot(ctx, store.NodeSnapshotCreateArgs{
		NodeID:            input.NodeID,
		SnapshotType:      "rollup",
		Summary:           input.Summary,
		AffectedFilesJSON: marshalStringSlice(input.AffectedFiles),
		NextAction:        input.NextAction,
	})
	if err != nil {
		return nil, err
	}

	node, err := service.store.UpdateGraphNodeApprovalState(ctx, input.NodeID, "pending", "in_review")
	if err != nil {
		return nil, err
	}

	preview, previewErr := service.store.RollupPreview(ctx, input.NodeID)
	if previewErr != nil {
		return nil, fmt.Errorf("rollup snapshot created but preview failed: %w", previewErr)
	}

	return map[string]any{
		"snapshot": snapshot,
		"node":     node,
		"preview":  preview,
	}, nil
}

type graphNodeCreateInput struct {
	NodeType       string   `json:"node_type"`
	Facet          string   `json:"facet"`
	Title          string   `json:"title"`
	Status         string   `json:"status"`
	Priority       int      `json:"priority"`
	ParentID       *int64   `json:"parent_id"`
	WorktreeID     *int64   `json:"worktree_id"`
	OwnerSessionID *int64   `json:"owner_session_id"`
	Summary        string   `json:"summary"`
	RiskLevel      *int     `json:"risk_level"`
	TokenEstimate  *int     `json:"token_estimate"`
	AffectedFiles  []string `json:"affected_files"`
	ApprovalState  string   `json:"approval_state"`
}

type graphNodeListInput struct {
	NodeType string `json:"node_type"`
	Facet    string `json:"facet"`
	Status   string `json:"status"`
	ParentID *int64 `json:"parent_id"`
}

type graphEdgeCreateInput struct {
	FromNodeID int64  `json:"from_node_id"`
	ToNodeID   int64  `json:"to_node_id"`
	EdgeType   string `json:"edge_type"`
}

type graphChecklistUpsertInput struct {
	NodeID   int64  `json:"node_id"`
	ItemText string `json:"item_text"`
	Status   string `json:"status"`
	OrderNo  int64  `json:"order_no"`
	Facet    string `json:"facet"`
}

type graphSnapshotCreateInput struct {
	NodeID        int64    `json:"node_id"`
	SnapshotType  string   `json:"snapshot_type"`
	Summary       string   `json:"summary"`
	AffectedFiles []string `json:"affected_files"`
	NextAction    string   `json:"next_action"`
}

type planBootstrapInput struct {
	InitiativeTitle string `json:"initiative_title"`
	PlanTitle       string `json:"plan_title"`
	Priority        int    `json:"priority"`
	OwnerSessionID  *int64 `json:"owner_session_id"`
	Summary         string `json:"summary"`
}

type planSliceSpecInput struct {
	Title         string   `json:"title"`
	Priority      int      `json:"priority"`
	TokenEstimate int      `json:"token_estimate"`
	AffectedFiles []string `json:"affected_files"`
	Summary       string   `json:"summary"`
}

type planSliceGenerateInput struct {
	PlanNodeID     int64                `json:"plan_node_id"`
	OwnerSessionID *int64               `json:"owner_session_id"`
	SliceSpecs     []planSliceSpecInput `json:"slice_specs"`
}

type planSliceReplanInput struct {
	NodeID        int64    `json:"node_id"`
	OwnerSessionID *int64  `json:"owner_session_id"`
	Reason        string   `json:"reason"`
	AffectedFiles []string `json:"affected_files"`
	NextAction    string   `json:"next_action"`
}

type planRollupPreviewInput struct {
	ParentNodeID int64 `json:"parent_node_id"`
}

type planRollupSubmitInput struct {
	NodeID        int64    `json:"node_id"`
	Summary       string   `json:"summary"`
	AffectedFiles []string `json:"affected_files"`
	NextAction    string   `json:"next_action"`
}

type planRollupApproveInput struct {
	NodeID int64 `json:"node_id"`
}

type planRollupRejectInput struct {
	NodeID int64 `json:"node_id"`
}
