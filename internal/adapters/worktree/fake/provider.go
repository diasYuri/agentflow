package fake

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/diasYuri/agentflow/internal/core/ports"
)

// Provider is a fake worktree provider for testing.
type Provider struct {
	BaseDir      string
	StatusResult *ports.WorktreeStatus
	DiffResult   *ports.ChangeSet
	ApplyResult  *ports.MergeResult
	ApplyError   error
}

// New creates a new fake worktree provider.
func New(baseDir string) *Provider {
	return &Provider{BaseDir: baseDir}
}

// Create creates a fake worktree directory.
func (p *Provider) Create(_ context.Context, req ports.CreateWorktreeRequest) (ports.Worktree, error) {
	dir := filepath.Join(p.BaseDir, fmt.Sprintf("wt-%s", req.WorkflowName))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ports.Worktree{}, err
	}
	return ports.Worktree{
		ID:           "fake-id",
		Name:         fmt.Sprintf("wt-%s", req.WorkflowName),
		Path:         dir,
		Branch:       "fake-branch",
		BaseCommit:   req.BaseCommit,
		WorkflowName: req.WorkflowName,
	}, nil
}

// Status returns a clean status by default, or the configured result.
func (p *Provider) Status(_ context.Context, _ ports.Worktree) (ports.WorktreeStatus, error) {
	if p.StatusResult != nil {
		return *p.StatusResult, nil
	}
	return ports.WorktreeStatus{Clean: true}, nil
}

// Diff returns an empty changeset by default, or the configured result.
func (p *Provider) Diff(_ context.Context, _ ports.Worktree) (ports.ChangeSet, error) {
	if p.DiffResult != nil {
		return *p.DiffResult, nil
	}
	return ports.ChangeSet{Empty: true}, nil
}

// Apply returns success by default, or the configured result/error.
func (p *Provider) Apply(_ context.Context, _ ports.ApplyWorktreeRequest) (ports.MergeResult, error) {
	if p.ApplyResult != nil {
		return *p.ApplyResult, p.ApplyError
	}
	return ports.MergeResult{Success: true}, nil
}

// Cleanup removes the worktree directory.
func (p *Provider) Cleanup(_ context.Context, req ports.CleanupWorktreeRequest) (ports.CleanupResult, error) {
	if req.Worktree.Path != "" {
		_ = os.RemoveAll(req.Worktree.Path)
	}
	return ports.CleanupResult{Removed: true}, nil
}
