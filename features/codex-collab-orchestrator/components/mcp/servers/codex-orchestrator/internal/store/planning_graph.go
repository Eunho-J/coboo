package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

func (store *Store) CreateGraphNode(ctx context.Context, args GraphNodeCreateArgs) (GraphNode, error) {
	if strings.TrimSpace(args.NodeType) == "" {
		return GraphNode{}, errors.New("node_type is required")
	}
	if strings.TrimSpace(args.Title) == "" {
		return GraphNode{}, errors.New("title is required")
	}
	facet := strings.TrimSpace(args.Facet)
	if facet == "" {
		facet = "planning"
	}
	status := strings.TrimSpace(args.Status)
	if status == "" {
		status = "todo"
	}
	approvalState := strings.TrimSpace(args.ApprovalState)
	if approvalState == "" {
		approvalState = "none"
	}

	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return GraphNode{}, err
	}
	defer transaction.Rollback()

	now := nowTimestamp()
	result, err := transaction.ExecContext(
		ctx,
		`INSERT INTO graph_nodes(node_type, facet, title, status, priority, parent_id, worktree_id, owner_session_id, summary, risk_level, token_estimate, affected_files_json, approval_state, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		args.NodeType,
		facet,
		args.Title,
		status,
		args.Priority,
		args.ParentID,
		args.WorktreeID,
		args.OwnerSessionID,
		nullableText(args.Summary),
		args.RiskLevel,
		args.TokenEstimate,
		nullableText(args.AffectedFilesJSON),
		approvalState,
		now,
		now,
	)
	if err != nil {
		return GraphNode{}, err
	}
	nodeID, err := result.LastInsertId()
	if err != nil {
		return GraphNode{}, err
	}

	if err := store.bumpVersionTx(ctx, transaction); err != nil {
		return GraphNode{}, err
	}

	row := transaction.QueryRowContext(
		ctx,
		`SELECT id, node_type, facet, title, status, priority, parent_id, worktree_id, owner_session_id, summary, risk_level, token_estimate, affected_files_json, approval_state, created_at, updated_at
		 FROM graph_nodes
		 WHERE id = ?`,
		nodeID,
	)
	node, err := scanGraphNode(row)
	if err != nil {
		return GraphNode{}, err
	}
	if err := transaction.Commit(); err != nil {
		return GraphNode{}, err
	}
	return node, nil
}

func (store *Store) ListGraphNodes(ctx context.Context, filter GraphNodeFilter) ([]GraphNode, error) {
	query := `SELECT id, node_type, facet, title, status, priority, parent_id, worktree_id, owner_session_id, summary, risk_level, token_estimate, affected_files_json, approval_state, created_at, updated_at
		FROM graph_nodes`
	whereClauses := make([]string, 0, 4)
	parameters := make([]any, 0, 4)

	if strings.TrimSpace(filter.NodeType) != "" {
		whereClauses = append(whereClauses, "node_type = ?")
		parameters = append(parameters, filter.NodeType)
	}
	if strings.TrimSpace(filter.Facet) != "" {
		whereClauses = append(whereClauses, "facet = ?")
		parameters = append(parameters, filter.Facet)
	}
	if strings.TrimSpace(filter.Status) != "" {
		whereClauses = append(whereClauses, "status = ?")
		parameters = append(parameters, filter.Status)
	}
	if filter.ParentID != nil {
		whereClauses = append(whereClauses, "parent_id = ?")
		parameters = append(parameters, *filter.ParentID)
	}
	if len(whereClauses) > 0 {
		query += " WHERE " + strings.Join(whereClauses, " AND ")
	}
	query += " ORDER BY priority DESC, updated_at ASC, id ASC"

	rows, err := store.database.QueryContext(ctx, query, parameters...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nodes := make([]GraphNode, 0)
	for rows.Next() {
		node, scanErr := scanGraphNode(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

func (store *Store) GetGraphNodeByID(ctx context.Context, nodeID int64) (GraphNode, error) {
	row := store.database.QueryRowContext(
		ctx,
		`SELECT id, node_type, facet, title, status, priority, parent_id, worktree_id, owner_session_id, summary, risk_level, token_estimate, affected_files_json, approval_state, created_at, updated_at
		 FROM graph_nodes
		 WHERE id = ?`,
		nodeID,
	)
	return scanGraphNode(row)
}

func (store *Store) UpdateGraphNodeApprovalState(ctx context.Context, nodeID int64, approvalState string, status string) (GraphNode, error) {
	if strings.TrimSpace(approvalState) == "" {
		return GraphNode{}, errors.New("approval_state is required")
	}

	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return GraphNode{}, err
	}
	defer transaction.Rollback()

	if strings.TrimSpace(status) == "" {
		_, err = transaction.ExecContext(
			ctx,
			`UPDATE graph_nodes
			 SET approval_state = ?, updated_at = ?
			 WHERE id = ?`,
			approvalState,
			nowTimestamp(),
			nodeID,
		)
	} else {
		_, err = transaction.ExecContext(
			ctx,
			`UPDATE graph_nodes
			 SET approval_state = ?, status = ?, updated_at = ?
			 WHERE id = ?`,
			approvalState,
			status,
			nowTimestamp(),
			nodeID,
		)
	}
	if err != nil {
		return GraphNode{}, err
	}

	if err := store.bumpVersionTx(ctx, transaction); err != nil {
		return GraphNode{}, err
	}

	row := transaction.QueryRowContext(
		ctx,
		`SELECT id, node_type, facet, title, status, priority, parent_id, worktree_id, owner_session_id, summary, risk_level, token_estimate, affected_files_json, approval_state, created_at, updated_at
		 FROM graph_nodes
		 WHERE id = ?`,
		nodeID,
	)
	node, err := scanGraphNode(row)
	if err != nil {
		return GraphNode{}, err
	}
	if err := transaction.Commit(); err != nil {
		return GraphNode{}, err
	}
	return node, nil
}

func (store *Store) CreateGraphEdge(ctx context.Context, args GraphEdgeCreateArgs) (GraphEdge, error) {
	if args.FromNodeID <= 0 || args.ToNodeID <= 0 {
		return GraphEdge{}, errors.New("from_node_id and to_node_id are required")
	}
	edgeType := strings.TrimSpace(args.EdgeType)
	if edgeType == "" {
		return GraphEdge{}, errors.New("edge_type is required")
	}

	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return GraphEdge{}, err
	}
	defer transaction.Rollback()

	result, err := transaction.ExecContext(
		ctx,
		`INSERT INTO graph_edges(from_node_id, to_node_id, edge_type, created_at)
		 VALUES(?, ?, ?, ?)`,
		args.FromNodeID,
		args.ToNodeID,
		edgeType,
		nowTimestamp(),
	)
	if err != nil {
		return GraphEdge{}, err
	}
	edgeID, err := result.LastInsertId()
	if err != nil {
		return GraphEdge{}, err
	}

	if err := store.bumpVersionTx(ctx, transaction); err != nil {
		return GraphEdge{}, err
	}

	row := transaction.QueryRowContext(
		ctx,
		`SELECT id, from_node_id, to_node_id, edge_type, created_at
		 FROM graph_edges
		 WHERE id = ?`,
		edgeID,
	)
	edge, err := scanGraphEdge(row)
	if err != nil {
		return GraphEdge{}, err
	}
	if err := transaction.Commit(); err != nil {
		return GraphEdge{}, err
	}
	return edge, nil
}

func (store *Store) UpsertNodeChecklistItem(ctx context.Context, args NodeChecklistUpsertArgs) (NodeChecklistItem, error) {
	if args.NodeID <= 0 {
		return NodeChecklistItem{}, errors.New("node_id is required")
	}
	if strings.TrimSpace(args.ItemText) == "" {
		return NodeChecklistItem{}, errors.New("item_text is required")
	}
	status := strings.TrimSpace(args.Status)
	if status == "" {
		status = "todo"
	}
	if args.OrderNo <= 0 {
		args.OrderNo = 1
	}
	facet := strings.TrimSpace(args.Facet)
	if facet == "" {
		facet = "planning"
	}

	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return NodeChecklistItem{}, err
	}
	defer transaction.Rollback()

	row := transaction.QueryRowContext(
		ctx,
		`SELECT id
		 FROM node_checklists
		 WHERE node_id = ? AND order_no = ? AND facet = ?
		 LIMIT 1`,
		args.NodeID,
		args.OrderNo,
		facet,
	)
	var checklistID int64
	queryErr := row.Scan(&checklistID)
	switch {
	case queryErr == nil:
		_, err = transaction.ExecContext(
			ctx,
			`UPDATE node_checklists
			 SET item_text = ?, status = ?, updated_at = ?
			 WHERE id = ?`,
			args.ItemText,
			status,
			nowTimestamp(),
			checklistID,
		)
		if err != nil {
			return NodeChecklistItem{}, err
		}
	case errors.Is(queryErr, sql.ErrNoRows):
		result, insertErr := transaction.ExecContext(
			ctx,
			`INSERT INTO node_checklists(node_id, item_text, status, order_no, facet, created_at, updated_at)
			 VALUES(?, ?, ?, ?, ?, ?, ?)`,
			args.NodeID,
			args.ItemText,
			status,
			args.OrderNo,
			facet,
			nowTimestamp(),
			nowTimestamp(),
		)
		if insertErr != nil {
			return NodeChecklistItem{}, insertErr
		}
		checklistID, err = result.LastInsertId()
		if err != nil {
			return NodeChecklistItem{}, err
		}
	default:
		return NodeChecklistItem{}, queryErr
	}

	if err := store.bumpVersionTx(ctx, transaction); err != nil {
		return NodeChecklistItem{}, err
	}

	refRow := transaction.QueryRowContext(
		ctx,
		`SELECT id, node_id, item_text, status, order_no, facet, created_at, updated_at
		 FROM node_checklists
		 WHERE id = ?`,
		checklistID,
	)
	checklist, err := scanNodeChecklistItem(refRow)
	if err != nil {
		return NodeChecklistItem{}, err
	}

	if err := transaction.Commit(); err != nil {
		return NodeChecklistItem{}, err
	}
	return checklist, nil
}

func (store *Store) CreateNodeSnapshot(ctx context.Context, args NodeSnapshotCreateArgs) (NodeSnapshot, error) {
	if args.NodeID <= 0 {
		return NodeSnapshot{}, errors.New("node_id is required")
	}
	if strings.TrimSpace(args.SnapshotType) == "" {
		return NodeSnapshot{}, errors.New("snapshot_type is required")
	}

	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return NodeSnapshot{}, err
	}
	defer transaction.Rollback()

	result, err := transaction.ExecContext(
		ctx,
		`INSERT INTO node_snapshots(node_id, snapshot_type, summary, affected_files_json, next_action, created_at)
		 VALUES(?, ?, ?, ?, ?, ?)`,
		args.NodeID,
		args.SnapshotType,
		nullableText(args.Summary),
		nullableText(args.AffectedFilesJSON),
		nullableText(args.NextAction),
		nowTimestamp(),
	)
	if err != nil {
		return NodeSnapshot{}, err
	}
	snapshotID, err := result.LastInsertId()
	if err != nil {
		return NodeSnapshot{}, err
	}

	if err := store.bumpVersionTx(ctx, transaction); err != nil {
		return NodeSnapshot{}, err
	}

	row := transaction.QueryRowContext(
		ctx,
		`SELECT id, node_id, snapshot_type, summary, affected_files_json, next_action, created_at
		 FROM node_snapshots
		 WHERE id = ?`,
		snapshotID,
	)
	snapshot, err := scanNodeSnapshot(row)
	if err != nil {
		return NodeSnapshot{}, err
	}
	if err := transaction.Commit(); err != nil {
		return NodeSnapshot{}, err
	}
	return snapshot, nil
}

func (store *Store) GetPlanningRule(ctx context.Context) (PlanningRule, error) {
	row := store.database.QueryRowContext(
		ctx,
		`SELECT max_token_per_slice, max_files_per_slice, replan_triggers_json, approval_policy, updated_at
		 FROM planning_rules
		 WHERE id = 1`,
	)
	var rule PlanningRule
	err := row.Scan(&rule.MaxTokenPerSlice, &rule.MaxFilesPerSlice, &rule.ReplanTriggersJSON, &rule.ApprovalPolicy, &rule.UpdatedAt)
	if err != nil {
		return PlanningRule{}, err
	}
	return rule, nil
}

func (store *Store) RollupPreview(ctx context.Context, parentNodeID int64) (map[string]any, error) {
	nodes, err := store.ListGraphNodes(ctx, GraphNodeFilter{
		ParentID: &parentNodeID,
	})
	if err != nil {
		return nil, err
	}

	statusCounts := map[string]int{}
	for _, node := range nodes {
		statusCounts[node.Status]++
	}
	return map[string]any{
		"parent_node_id": parentNodeID,
		"child_count":    len(nodes),
		"status_counts":  statusCounts,
		"children":       nodes,
	}, nil
}

func scanGraphNode(scanner rowScanner) (GraphNode, error) {
	var node GraphNode
	var parentID sql.NullInt64
	var worktreeID sql.NullInt64
	var ownerSessionID sql.NullInt64
	var summary sql.NullString
	var riskLevel sql.NullInt64
	var tokenEstimate sql.NullInt64
	var affectedFilesJSON sql.NullString
	err := scanner.Scan(
		&node.ID,
		&node.NodeType,
		&node.Facet,
		&node.Title,
		&node.Status,
		&node.Priority,
		&parentID,
		&worktreeID,
		&ownerSessionID,
		&summary,
		&riskLevel,
		&tokenEstimate,
		&affectedFilesJSON,
		&node.ApprovalState,
		&node.CreatedAt,
		&node.UpdatedAt,
	)
	if err != nil {
		return GraphNode{}, err
	}
	if parentID.Valid {
		node.ParentID = &parentID.Int64
	}
	if worktreeID.Valid {
		node.WorktreeID = &worktreeID.Int64
	}
	if ownerSessionID.Valid {
		node.OwnerSessionID = &ownerSessionID.Int64
	}
	if summary.Valid {
		node.Summary = &summary.String
	}
	if riskLevel.Valid {
		value := int(riskLevel.Int64)
		node.RiskLevel = &value
	}
	if tokenEstimate.Valid {
		value := int(tokenEstimate.Int64)
		node.TokenEstimate = &value
	}
	if affectedFilesJSON.Valid {
		node.AffectedFilesJSON = &affectedFilesJSON.String
	}
	return node, nil
}

func scanGraphEdge(scanner rowScanner) (GraphEdge, error) {
	var edge GraphEdge
	err := scanner.Scan(
		&edge.ID,
		&edge.FromNodeID,
		&edge.ToNodeID,
		&edge.EdgeType,
		&edge.CreatedAt,
	)
	if err != nil {
		return GraphEdge{}, err
	}
	return edge, nil
}

func scanNodeChecklistItem(scanner rowScanner) (NodeChecklistItem, error) {
	var item NodeChecklistItem
	err := scanner.Scan(
		&item.ID,
		&item.NodeID,
		&item.ItemText,
		&item.Status,
		&item.OrderNo,
		&item.Facet,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return NodeChecklistItem{}, err
	}
	return item, nil
}

func scanNodeSnapshot(scanner rowScanner) (NodeSnapshot, error) {
	var snapshot NodeSnapshot
	var summary sql.NullString
	var affectedFilesJSON sql.NullString
	var nextAction sql.NullString
	err := scanner.Scan(
		&snapshot.ID,
		&snapshot.NodeID,
		&snapshot.SnapshotType,
		&summary,
		&affectedFilesJSON,
		&nextAction,
		&snapshot.CreatedAt,
	)
	if err != nil {
		return NodeSnapshot{}, err
	}
	if summary.Valid {
		snapshot.Summary = &summary.String
	}
	if affectedFilesJSON.Valid {
		snapshot.AffectedFilesJSON = &affectedFilesJSON.String
	}
	if nextAction.Valid {
		snapshot.NextAction = &nextAction.String
	}
	return snapshot, nil
}
