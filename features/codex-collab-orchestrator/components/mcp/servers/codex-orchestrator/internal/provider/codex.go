package provider

import (
	"regexp"
	"strings"
)

var (
	ansiPattern        = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	codexIdlePrompt    = regexp.MustCompile(`(?:❯|›|codex>)`)
	codexIdleAtEnd     = regexp.MustCompile(`(?m)(?:^\s*(?:❯|›|codex>)\s*)\s*\z`)
	codexUserPrefix    = regexp.MustCompile(`(?m)^You\b`)
	codexAssistant     = regexp.MustCompile(`(?m)^(?:assistant|codex|agent)\s*:`)
	codexProcessing    = regexp.MustCompile(`(?i)\b(?:thinking|working|running|executing|processing|analyzing)\b`)
	codexWaitingPrompt = regexp.MustCompile(`(?m)^(?:Approve|Allow)\b.*\b(?:y/n|yes/no|yes|no)\b`)
	codexErrorPattern  = regexp.MustCompile(`(?m)^(?:Error:|ERROR:|Traceback \(most recent call last\):|panic:)`)
)

type CodexProvider struct{}

func NewCodexProvider() *CodexProvider {
	return &CodexProvider{}
}

func (p *CodexProvider) Name() string { return "codex" }

func (p *CodexProvider) GetIdlePatternForLog() string {
	return `(?:❯|›|codex>)\s*$`
}

func (p *CodexProvider) ExitCommand() string { return "/exit" }

func (p *CodexProvider) GetStatus(rawOutput string) Status {
	clean := ansiPattern.ReplaceAllString(rawOutput, "")

	// Find last user message position.
	var lastUserIdx int = -1
	matches := codexUserPrefix.FindAllStringIndex(clean, -1)
	if len(matches) > 0 {
		lastUserIdx = matches[len(matches)-1][0]
	}

	afterUser := clean
	if lastUserIdx >= 0 {
		afterUser = clean[lastUserIdx:]
	}
	assistantAfterUser := lastUserIdx >= 0 && codexAssistant.MatchString(afterUser)
	hasIdleAtEnd := codexIdleAtEnd.MatchString(clean)

	// Check error/waiting only after last user message, not in assistant response.
	if lastUserIdx >= 0 && !assistantAfterUser {
		if codexWaitingPrompt.MatchString(afterUser) {
			return StatusWaitingUserAnswer
		}
		if codexErrorPattern.MatchString(afterUser) {
			return StatusError
		}
	}

	if hasIdleAtEnd {
		if lastUserIdx >= 0 && codexAssistant.MatchString(clean[lastUserIdx:]) {
			return StatusCompleted
		}
		return StatusIdle
	}

	return StatusProcessing
}

func (p *CodexProvider) ExtractLastResponse(rawOutput string) string {
	clean := ansiPattern.ReplaceAllString(rawOutput, "")
	lines := strings.Split(clean, "\n")

	// Walk backward to find last assistant response block.
	var responseLines []string
	inResponse := false
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if codexIdlePrompt.MatchString(line) && line == strings.TrimSpace(codexIdlePrompt.ReplaceAllString(line, "")) {
			continue
		}
		if codexAssistant.MatchString(line) {
			inResponse = true
			responseLines = append([]string{codexAssistant.ReplaceAllString(line, "")}, responseLines...)
			break
		}
		if codexUserPrefix.MatchString(line) {
			break
		}
		if inResponse || len(responseLines) > 0 || line != "" {
			responseLines = append([]string{line}, responseLines...)
			inResponse = true
		}
	}

	return strings.TrimSpace(strings.Join(responseLines, "\n"))
}
