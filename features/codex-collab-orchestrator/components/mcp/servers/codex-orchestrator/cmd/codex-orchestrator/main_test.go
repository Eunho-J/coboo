package main

import (
	"testing"
)

// TestToolGroupsDefinition verifies that toolGroups slice contains 8 groups
// and each group has at least one method.
func TestToolGroupsDefinition(t *testing.T) {
	if len(toolGroups) != 8 {
		t.Errorf("expected 8 tool groups, got %d", len(toolGroups))
	}

	expectedGroups := []string{
		"orch_session",
		"orch_task",
		"orch_graph",
		"orch_workspace",
		"orch_thread",
		"orch_lifecycle",
		"orch_merge",
		"orch_system",
	}

	for i, expected := range expectedGroups {
		if i >= len(toolGroups) {
			t.Errorf("missing tool group: %s", expected)
			continue
		}
		if toolGroups[i].Name != expected {
			t.Errorf("tool group %d: expected name %s, got %s", i, expected, toolGroups[i].Name)
		}
		if len(toolGroups[i].Methods) == 0 {
			t.Errorf("tool group %s has no methods", toolGroups[i].Name)
		}
		if toolGroups[i].Description == "" {
			t.Errorf("tool group %s has no description", toolGroups[i].Name)
		}
	}
}

// TestToolGroupMethodIndex verifies that the toolGroupMethodIndex contains
// 8 entries and properly maps methods to their groups.
func TestToolGroupMethodIndex(t *testing.T) {
	if len(toolGroupMethodIndex) != 8 {
		t.Errorf("expected 8 entries in method index, got %d", len(toolGroupMethodIndex))
	}

	// Test specific method mappings for each group
	testCases := []struct {
		group  string
		method string
	}{
		{"orch_session", "workspace.init"},
		{"orch_session", "session.open"},
		{"orch_task", "task.create"},
		{"orch_task", "case.begin"},
		{"orch_graph", "graph.node.create"},
		{"orch_workspace", "scheduler.decide_worktree"},
		{"orch_thread", "thread.child.spawn"},
		{"orch_lifecycle", "work.current_ref"},
		{"orch_merge", "merge.request"},
		{"orch_system", "runtime.tmux.ensure"},
	}

	for _, tc := range testCases {
		methods, ok := toolGroupMethodIndex[tc.group]
		if !ok {
			t.Errorf("%s not found in toolGroupMethodIndex", tc.group)
			continue
		}
		if !methods[tc.method] {
			t.Errorf("%s missing method %s", tc.group, tc.method)
		}
	}
}

// TestBuildToolsList verifies that buildToolsList returns 9 tools
// (8 domain-specific groups + 1 legacy orchestrator.call).
func TestBuildToolsList(t *testing.T) {
	tools := buildToolsList()
	if len(tools) != 9 {
		t.Errorf("expected 9 tools, got %d", len(tools))
	}

	// Verify legacy orchestrator.call tool exists
	foundLegacy := false
	for _, tool := range tools {
		if tool["name"] == "orchestrator.call" {
			foundLegacy = true
			// Verify it has required properties
			if tool["description"] == nil {
				t.Error("orchestrator.call missing description")
			}
			if tool["inputSchema"] == nil {
				t.Error("orchestrator.call missing inputSchema")
			}
			break
		}
	}
	if !foundLegacy {
		t.Error("legacy orchestrator.call not found in tools list")
	}

	// Verify all 8 domain-specific groups exist
	expectedGroups := []string{
		"orch_session",
		"orch_task",
		"orch_graph",
		"orch_workspace",
		"orch_thread",
		"orch_lifecycle",
		"orch_merge",
		"orch_system",
	}

	for _, expected := range expectedGroups {
		found := false
		for _, tool := range tools {
			if tool["name"] == expected {
				found = true
				// Verify schema has method enum
				schema, ok := tool["inputSchema"].(map[string]any)
				if !ok {
					t.Errorf("%s has invalid inputSchema type", expected)
					continue
				}
				props, ok := schema["properties"].(map[string]any)
				if !ok {
					t.Errorf("%s inputSchema missing properties", expected)
					continue
				}
				methodProp, ok := props["method"].(map[string]any)
				if !ok {
					t.Errorf("%s inputSchema missing method property", expected)
					continue
				}
				if methodProp["enum"] == nil {
					t.Errorf("%s method property missing enum", expected)
				}
				break
			}
		}
		if !found {
			t.Errorf("tool group %s not found in tools list", expected)
		}
	}
}

// TestMethodGroupValidation verifies that each group only contains
// its designated methods and rejects others.
func TestMethodGroupValidation(t *testing.T) {
	testCases := []struct {
		group          string
		invalidMethod  string
		shouldNotExist bool
	}{
		// orch_session should not contain task methods
		{"orch_session", "task.create", true},
		{"orch_session", "case.begin", true},
		// orch_merge should not contain workspace.init
		{"orch_merge", "workspace.init", true},
		{"orch_merge", "task.create", true},
		// orch_task should not contain merge methods
		{"orch_task", "merge.request", true},
		{"orch_task", "session.open", true},
		// Verify valid methods exist
		{"orch_session", "workspace.init", false},
		{"orch_task", "task.create", false},
		{"orch_merge", "merge.request", false},
	}

	for _, tc := range testCases {
		methods, ok := toolGroupMethodIndex[tc.group]
		if !ok {
			t.Errorf("group %s not found in index", tc.group)
			continue
		}

		exists := methods[tc.invalidMethod]
		if tc.shouldNotExist && exists {
			t.Errorf("%s should not contain %s", tc.group, tc.invalidMethod)
		}
		if !tc.shouldNotExist && !exists {
			t.Errorf("%s should contain %s", tc.group, tc.invalidMethod)
		}
	}
}

// TestNoMethodDuplicates ensures that no method appears in multiple groups.
// This prevents ambiguity in routing.
func TestNoMethodDuplicates(t *testing.T) {
	seen := make(map[string]string)

	for _, g := range toolGroups {
		for _, m := range g.Methods {
			if prev, exists := seen[m]; exists {
				t.Errorf("method %s appears in both %s and %s", m, prev, g.Name)
			}
			seen[m] = g.Name
		}
	}
}

// TestToolGroupMethodCounts verifies expected method counts for each group.
func TestToolGroupMethodCounts(t *testing.T) {
	expectedCounts := map[string]int{
		"orch_session":   5, // workspace.init, session.open, session.heartbeat, session.close, session.context
		"orch_task":      9, // task.create, task.list, task.get, case.begin, step.check, case.complete, resume.next, resume.candidates.list, resume.candidates.attach
		"orch_graph":     5, // graph.node.create, graph.node.list, graph.edge.create, graph.checklist.upsert, graph.snapshot.create
		"orch_workspace": 8, // scheduler.decide_worktree, worktree.create, worktree.list, worktree.spawn, worktree.merge_to_parent, lock.acquire, lock.heartbeat, lock.release
		"orch_thread":    6, // thread.child.spawn, thread.child.directive, thread.child.list, thread.child.interrupt, thread.child.stop, thread.attach_info
		"orch_lifecycle": 2, // work.current_ref, work.current_ref.ack
		"orch_merge":     9, // merge.request, merge.review_context, merge.review.request_auto, merge.review.thread_status, merge.main.request, merge.main.next, merge.main.status, merge.main.acquire_lock, merge.main.release_lock
		"orch_system":    11, // runtime.tmux.ensure, runtime.bundle.info, mirror.status, mirror.refresh, plan.bootstrap, plan.slice.generate, plan.slice.replan, plan.rollup.preview, plan.rollup.submit, plan.rollup.approve, plan.rollup.reject
	}

	for _, g := range toolGroups {
		expected, ok := expectedCounts[g.Name]
		if !ok {
			t.Errorf("no expected count defined for group %s", g.Name)
			continue
		}
		actual := len(g.Methods)
		if actual != expected {
			t.Errorf("group %s: expected %d methods, got %d", g.Name, expected, actual)
		}
	}
}

// TestToolGroupIndexMatchesDefinition verifies that the init() function
// correctly builds the index from the toolGroups definition.
func TestToolGroupIndexMatchesDefinition(t *testing.T) {
	for _, g := range toolGroups {
		indexMethods, ok := toolGroupMethodIndex[g.Name]
		if !ok {
			t.Errorf("group %s not found in index", g.Name)
			continue
		}

		// Check that all methods in definition are in index
		for _, method := range g.Methods {
			if !indexMethods[method] {
				t.Errorf("group %s: method %s in definition but not in index", g.Name, method)
			}
		}

		// Check that index doesn't have extra methods
		if len(indexMethods) != len(g.Methods) {
			t.Errorf("group %s: index has %d methods but definition has %d",
				g.Name, len(indexMethods), len(g.Methods))
		}
	}
}

// TestToolGroupDescriptions verifies that all groups have meaningful descriptions.
func TestToolGroupDescriptions(t *testing.T) {
	for _, g := range toolGroups {
		if len(g.Description) < 10 {
			t.Errorf("group %s has suspiciously short description: %q", g.Name, g.Description)
		}
		if g.Description == "" {
			t.Errorf("group %s has empty description", g.Name)
		}
	}
}
