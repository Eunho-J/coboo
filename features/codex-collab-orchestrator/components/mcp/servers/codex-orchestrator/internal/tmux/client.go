package tmux

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/google/uuid"
)

const SessionPrefix = "cds-"

type Client struct{}

func NewClient() *Client {
	return &Client{}
}

func (c *Client) HasSession(ctx context.Context, sessionName string) bool {
	_, err := c.run(ctx, "tmux", "has-session", "-t", sessionName)
	return err == nil
}

func (c *Client) NewSession(ctx context.Context, sessionName, windowName, workdir string) error {
	args := []string{"new-session", "-d", "-s", sessionName}
	if windowName != "" {
		args = append(args, "-n", windowName)
	}
	if workdir != "" {
		args = append(args, "-c", workdir)
	}
	_, err := c.run(ctx, "tmux", args...)
	return err
}

func (c *Client) KillSession(ctx context.Context, sessionName string) error {
	_, err := c.run(ctx, "tmux", "kill-session", "-t", sessionName)
	return err
}

func (c *Client) ListSessions(ctx context.Context) ([]string, error) {
	output, err := c.run(ctx, "tmux", "list-sessions", "-F", "#{session_name}")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(output) == "" {
		return nil, nil
	}
	return strings.Split(strings.TrimSpace(output), "\n"), nil
}

func (c *Client) ListOwnedSessions(ctx context.Context) ([]string, error) {
	all, err := c.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	owned := make([]string, 0, len(all))
	for _, s := range all {
		if strings.HasPrefix(s, SessionPrefix) {
			owned = append(owned, s)
		}
	}
	return owned, nil
}

func (c *Client) RenameWindow(ctx context.Context, target, name string) error {
	_, err := c.run(ctx, "tmux", "rename-window", "-t", target, name)
	return err
}

func (c *Client) SplitWindow(ctx context.Context, target, workdir, direction string) (string, error) {
	dirFlag := "-v"
	if strings.EqualFold(strings.TrimSpace(direction), "horizontal") {
		dirFlag = "-h"
	}
	output, err := c.run(ctx, "tmux", "split-window", dirFlag, "-t", target, "-c", workdir, "-P", "-F", "#{pane_id}")
	if err != nil {
		return "", err
	}
	paneID := strings.TrimSpace(output)
	if paneID == "" {
		return "", fmt.Errorf("split-window returned empty pane id")
	}
	return paneID, nil
}

func (c *Client) KillPane(ctx context.Context, paneID string) error {
	_, err := c.run(ctx, "tmux", "kill-pane", "-t", paneID)
	return err
}

func (c *Client) ListPanes(ctx context.Context, target string) ([]string, error) {
	output, err := c.run(ctx, "tmux", "list-panes", "-t", target, "-F", "#{pane_id}")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(output) == "" {
		return nil, nil
	}
	return strings.Split(strings.TrimSpace(output), "\n"), nil
}

func (c *Client) PaneExists(ctx context.Context, paneID string) bool {
	if strings.TrimSpace(paneID) == "" {
		return false
	}
	_, err := c.run(ctx, "tmux", "display-message", "-p", "-t", paneID, "#{pane_id}")
	return err == nil
}

// SendKeys sends text via load-buffer + paste-buffer pattern for reliable
// multiline delivery (CAO tmux.py:109-142).
func (c *Client) SendKeys(ctx context.Context, target, text string) error {
	bufName := fmt.Sprintf("cds_%s", uuid.New().String()[:8])

	// 1. Load text into a named buffer via stdin.
	loadCmd := exec.CommandContext(ctx, "tmux", "load-buffer", "-b", bufName, "-")
	loadCmd.Stdin = strings.NewReader(text)
	if out, err := loadCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux load-buffer failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	// 2. Paste buffer with bracketed paste mode (-p).
	if _, err := c.run(ctx, "tmux", "paste-buffer", "-p", "-b", bufName, "-t", target); err != nil {
		_ = c.deleteBuffer(ctx, bufName)
		return err
	}

	// 3. Send Enter to submit.
	if _, err := c.run(ctx, "tmux", "send-keys", "-t", target, "Enter"); err != nil {
		_ = c.deleteBuffer(ctx, bufName)
		return err
	}

	// 4. Cleanup buffer.
	_ = c.deleteBuffer(ctx, bufName)
	return nil
}

// SendKeysRaw sends raw key sequences (e.g. "C-c", "exit", "C-m") without
// the load-buffer pattern. Use for control sequences only.
func (c *Client) SendKeysRaw(ctx context.Context, target string, keys ...string) error {
	args := append([]string{"send-keys", "-t", target}, keys...)
	_, err := c.run(ctx, "tmux", args...)
	return err
}

// StartPipePane starts streaming pane output to a log file (CAO tmux.py:255-278).
func (c *Client) StartPipePane(ctx context.Context, target, logFilePath string) error {
	_, err := c.run(ctx, "tmux", "pipe-pane", "-t", target, "-o", fmt.Sprintf("cat >> %s", logFilePath))
	return err
}

// StopPipePane stops pipe-pane streaming.
func (c *Client) StopPipePane(ctx context.Context, target string) error {
	_, err := c.run(ctx, "tmux", "pipe-pane", "-t", target)
	return err
}

// CaptureHistory captures pane content with optional history depth.
func (c *Client) CaptureHistory(ctx context.Context, paneID string, lines int) (string, error) {
	startLine := fmt.Sprintf("-%d", lines)
	output, err := c.run(ctx, "tmux", "capture-pane", "-e", "-p", "-S", startLine, "-t", paneID)
	if err != nil {
		return "", err
	}
	return output, nil
}

// GetPaneWorkingDirectory returns the current working directory of a pane.
func (c *Client) GetPaneWorkingDirectory(ctx context.Context, paneID string) (string, error) {
	output, err := c.run(ctx, "tmux", "display-message", "-p", "-t", paneID, "#{pane_current_path}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func (c *Client) deleteBuffer(ctx context.Context, bufName string) error {
	_, err := c.run(ctx, "tmux", "delete-buffer", "-b", bufName)
	return err
}

func (c *Client) run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %s failed: %w (%s)", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}
