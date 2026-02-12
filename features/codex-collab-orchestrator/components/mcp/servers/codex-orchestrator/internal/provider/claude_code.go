package provider

import (
	"regexp"
	"strings"
)

var (
	ccResponseMarker = regexp.MustCompile(`⏺(?:\x1b\[[0-9;]*m)*\s+`)
	ccProcessing     = regexp.MustCompile(`[✶✢✽✻·✳].*….*\(esc to interrupt.*\)`)
	ccIdlePrompt     = regexp.MustCompile(`>[\s\xa0]`)
	ccWaiting        = regexp.MustCompile(`❯.*\d+\.`)
	ccSeparator      = regexp.MustCompile(`─{4,}`)
)

type ClaudeCodeProvider struct{}

func NewClaudeCodeProvider() *ClaudeCodeProvider {
	return &ClaudeCodeProvider{}
}

func (p *ClaudeCodeProvider) Name() string { return "claude_code" }

func (p *ClaudeCodeProvider) GetIdlePatternForLog() string {
	return `>\s`
}

func (p *ClaudeCodeProvider) ExitCommand() string { return "/exit" }

func (p *ClaudeCodeProvider) GetStatus(rawOutput string) Status {
	if ccProcessing.MatchString(rawOutput) {
		return StatusProcessing
	}

	if ccWaiting.MatchString(rawOutput) {
		return StatusWaitingUserAnswer
	}

	hasResponse := ccResponseMarker.MatchString(rawOutput)
	hasPrompt := ccIdlePrompt.MatchString(rawOutput)

	if hasPrompt && hasResponse {
		return StatusCompleted
	}
	if hasPrompt {
		return StatusIdle
	}

	return StatusError
}

func (p *ClaudeCodeProvider) ExtractLastResponse(rawOutput string) string {
	matches := ccResponseMarker.FindAllStringIndex(rawOutput, -1)
	if len(matches) == 0 {
		return ""
	}

	lastMatch := matches[len(matches)-1]
	remaining := rawOutput[lastMatch[1]:]
	lines := strings.Split(remaining, "\n")

	var responseLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if regexp.MustCompile(`^>\s`).MatchString(trimmed) || ccSeparator.MatchString(trimmed) {
			break
		}
		responseLines = append(responseLines, strings.TrimSpace(line))
	}

	result := strings.TrimSpace(strings.Join(responseLines, "\n"))
	return ansiPattern.ReplaceAllString(result, "")
}
