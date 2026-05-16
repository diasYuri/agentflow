package ports

import (
	"context"
	"errors"
)

var (
	ErrWorktreeStructural  = errors.New("worktree structural error")
	ErrWorktreeResolvable  = errors.New("worktree resolvable error")
	ErrWorktreeRecoverable = errors.New("worktree recoverable error")
)

const (
	WorktreeCleanRequire = "require_clean"
)

type WorktreeProvider interface {
	Create(ctx context.Context, req CreateWorktreeRequest) (Worktree, error)
	Status(ctx context.Context, worktree Worktree) (WorktreeStatus, error)
	Diff(ctx context.Context, worktree Worktree) (ChangeSet, error)
	Apply(ctx context.Context, req ApplyWorktreeRequest) (MergeResult, error)
	Cleanup(ctx context.Context, req CleanupWorktreeRequest) (CleanupResult, error)
}

type CreateWorktreeRequest struct {
	WorkflowName string
	BaseCommit   string
	WorkingDir   string
	TargetDir    string
	CleanPolicy  string
}

type Worktree struct {
	ID           string
	Name         string
	Path         string
	Branch       string
	BaseCommit   string
	WorkflowName string
	Commands     []GitCommand
}

type WorktreeStatus struct {
	Clean    bool
	Files    []FileStatus
	Raw      string
	Commands []GitCommand
}

type FileStatus struct {
	Path   string
	Status string
}

type ChangeSet struct {
	Empty    bool
	Files    []FileChange
	Diff     string
	Commands []GitCommand
}

type FileChange struct {
	Path    string
	Status  string
	OldPath string
}

type ApplyWorktreeRequest struct {
	Worktree   Worktree
	TargetDir  string
	BaseCommit string
	Diff       string
}

type MergeResult struct {
	Success   bool
	Conflicts []Conflict
	Commands  []GitCommand
}

type Conflict struct {
	Path   string
	Reason string
}

type CleanupWorktreeRequest struct {
	Worktree Worktree
	Force    bool
}

type CleanupResult struct {
	Removed  bool
	Commands []GitCommand
}

type GitCommand struct {
	Command  string
	ExitCode int
	Stdout   string
	Stderr   string
}

type WorktreeProviderRegistry interface {
	Get(name string) (WorktreeProvider, bool)
	HasProvider(name string) bool
}
