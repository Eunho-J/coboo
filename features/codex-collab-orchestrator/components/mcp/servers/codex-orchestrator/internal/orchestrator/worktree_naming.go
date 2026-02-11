package orchestrator

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var slugTokenPattern = regexp.MustCompile(`[a-z0-9]+`)

var slugStopwords = map[string]struct{}{
	"a": {}, "an": {}, "the": {}, "to": {}, "for": {}, "of": {}, "and": {}, "or": {},
	"in": {}, "on": {}, "with": {}, "by": {}, "from": {}, "at": {}, "is": {}, "are": {},
}

func deriveWorktreeSlug(preferred string, fallback string) string {
	candidate := strings.TrimSpace(preferred)
	if candidate == "" {
		candidate = strings.TrimSpace(fallback)
	}
	if candidate == "" {
		return "work"
	}
	normalized := normalizeSlugCandidate(candidate)
	if normalized == "" {
		return "work"
	}
	return normalized
}

func normalizeSlugCandidate(value string) string {
	tokens := slugTokenPattern.FindAllString(strings.ToLower(strings.TrimSpace(value)), -1)
	if len(tokens) == 0 {
		return ""
	}

	filtered := make([]string, 0, 2)
	for _, token := range tokens {
		if _, blocked := slugStopwords[token]; blocked {
			continue
		}
		filtered = append(filtered, token)
		if len(filtered) == 2 {
			break
		}
	}
	if len(filtered) == 0 {
		filtered = append(filtered, tokens[0])
		if len(tokens) > 1 {
			filtered = append(filtered, tokens[1])
		}
	}
	return strings.Join(filtered, "-")
}

func slugWithSuffix(base string, attempt int) string {
	normalizedBase := normalizeSlugCandidate(base)
	if normalizedBase == "" {
		normalizedBase = "work"
	}
	if attempt <= 0 {
		return normalizedBase
	}
	return fmt.Sprintf("%s-%d", normalizedBase, attempt+1)
}

func sanitizeTmuxName(value string) string {
	parts := slugTokenPattern.FindAllString(strings.ToLower(strings.TrimSpace(value)), -1)
	if len(parts) == 0 {
		return ""
	}
	joined := strings.Join(parts, "-")
	if len(joined) > 80 {
		joined = strings.Trim(joined[:80], "-")
	}
	return joined
}

func (service *Service) buildViewerTmuxSessionName(worktreePath string) string {
	repositoryName := sanitizeTmuxName(filepath.Base(service.repoPath))
	if repositoryName == "" {
		repositoryName = "repo"
	}
	worktreeName := sanitizeTmuxName(filepath.Base(strings.TrimSpace(worktreePath)))
	if worktreeName == "" {
		worktreeName = "worktree"
	}
	sessionName := fmt.Sprintf("%s-%s", repositoryName, worktreeName)
	if len(sessionName) > 80 {
		sessionName = strings.Trim(sessionName[:80], "-")
	}
	if sessionName == "" {
		return "repo-worktree"
	}
	return sessionName
}
