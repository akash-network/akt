// Package steps defines the StepExecutor interface and a registry of
// built-in step type implementations for the workflow engine.
package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"pkg.akt.dev/akt/internal/workflow"
)

// ChainClient is the interface workflows use to interact with the chain.
// Implemented by the chain client package (Phase 1.8).
type ChainClient interface {
	BroadcastTx(ctx context.Context, msgType string, params map[string]string) (*TxResult, error)
	Query(ctx context.Context, path string, params map[string]string) (json.RawMessage, error)
}

// TxResult holds the result of a broadcast transaction.
type TxResult struct {
	TxHash string          `json:"tx_hash"`
	Height int64           `json:"height"`
	Code   uint32          `json:"code"`
	Data   json.RawMessage `json:"data,omitempty"`
}

// ProviderClient is the interface for provider gateway operations.
// Implemented by the provider client package (Phase 2).
type ProviderClient interface {
	SendManifest(ctx context.Context, provider string, dseq uint64, sdl []byte) error
	SendManifestToActiveLeases(ctx context.Context, dseq uint64, sdl []byte) ([]string, error)
	LeaseStatus(ctx context.Context, provider string, dseq uint64) (json.RawMessage, error)
}

// Registry maps step types to their executors.
type Registry struct {
	mu        sync.RWMutex
	executors map[workflow.StepType]workflow.StepExecutor
}

// NewRegistry creates a registry pre-loaded with all built-in step executors.
func NewRegistry(chain ChainClient, provider ProviderClient) *Registry {
	r := &Registry{
		executors: make(map[workflow.StepType]workflow.StepExecutor),
	}

	// Register built-in step types.
	r.Register(&TxExecutor{chain: chain})
	r.Register(&QueryExecutor{chain: chain})
	r.Register(&WaitExecutor{chain: chain})
	r.Register(&PromptExecutor{})
	r.Register(&ProviderExecutor{provider: provider})
	r.Register(&OutputExecutor{})
	r.Register(&ShellExecutor{})
	r.Register(&CheckExecutor{})

	return r
}

// Register adds or replaces an executor for a step type.
func (r *Registry) Register(e workflow.StepExecutor) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.executors[e.Type()] = e
}

// Get returns the executor for the given step type.
func (r *Registry) Get(t workflow.StepType) (workflow.StepExecutor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	e, ok := r.executors[t]
	if !ok {
		return nil, fmt.Errorf("no executor registered for step type %q", t)
	}

	return e, nil
}
