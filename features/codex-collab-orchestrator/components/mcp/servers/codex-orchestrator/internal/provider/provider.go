package provider

type Status string

const (
	StatusIdle              Status = "idle"
	StatusProcessing        Status = "processing"
	StatusCompleted         Status = "completed"
	StatusWaitingUserAnswer Status = "waiting_user_answer"
	StatusError             Status = "error"
)

// Provider abstracts a CLI tool (Codex, Claude Code, etc.) running in a tmux pane.
type Provider interface {
	// Name returns the provider identifier (e.g. "codex", "claude_code").
	Name() string

	// GetStatus analyzes terminal output and returns the current status.
	GetStatus(output string) Status

	// GetIdlePatternForLog returns a regex pattern for fast IDLE detection
	// in pipe-pane log file tails (tier-1 check).
	GetIdlePatternForLog() string

	// ExtractLastResponse extracts the agent's final response from captured output.
	ExtractLastResponse(output string) string

	// ExitCommand returns the command to exit the CLI (e.g. "/exit", "exit").
	ExitCommand() string
}
