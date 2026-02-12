package provider

import (
	"fmt"
	"sync"
)

// Manager maps thread IDs to their Provider instances.
type Manager struct {
	mu        sync.RWMutex
	providers map[int64]Provider
}

func NewManager() *Manager {
	return &Manager{
		providers: make(map[int64]Provider),
	}
}

// Register binds a provider to a thread ID.
func (m *Manager) Register(threadID int64, p Provider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.providers[threadID] = p
}

// Get returns the provider for a thread ID.
func (m *Manager) Get(threadID int64) (Provider, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.providers[threadID]
	return p, ok
}

// Remove removes the provider for a thread ID.
func (m *Manager) Remove(threadID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.providers, threadID)
}

// Create creates a new provider by type name and registers it.
func (m *Manager) Create(threadID int64, providerType string) (Provider, error) {
	p, err := NewByType(providerType)
	if err != nil {
		return nil, err
	}
	m.Register(threadID, p)
	return p, nil
}

// NewByType creates a Provider by its type name.
func NewByType(providerType string) (Provider, error) {
	switch providerType {
	case "codex":
		return NewCodexProvider(), nil
	case "claude_code":
		return NewClaudeCodeProvider(), nil
	default:
		return nil, fmt.Errorf("unknown provider type: %s", providerType)
	}
}
