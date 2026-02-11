package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func pathExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	if _, err := os.Stat(path); err != nil {
		return false
	}
	return true
}
