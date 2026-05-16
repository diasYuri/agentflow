package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diasYuri/agentflow/internal/adapters/shell"
	"github.com/diasYuri/agentflow/internal/core/ports"
)

func TestCreateWorktree_NameFormat(t *testing.T) {
	ctx := context.Background()
	repo := initRepo(t)
	provider := newProvider(t)

	wt, err := provider.Create(ctx, ports.CreateWorktreeRequest{
		WorkflowName: "my-workflow",
		BaseCommit:   repo.head,
		WorkingDir:   repo.dir,
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	defer cleanupWorktree(t, ctx, provider, wt)

	if !strings.Contains(wt.Path, "my-workflow_") {
		t.Fatalf("expected path to contain 'my-workflow_', got %s", wt.Path)
	}
	if !strings.HasPrefix(wt.Branch, "agentflow/my-workflow_") {
		t.Fatalf("expected branch prefix 'agentflow/my-workflow_', got %s", wt.Branch)
	}
	if wt.BaseCommit != repo.head {
		t.Fatalf("expected base commit %s, got %s", repo.head, wt.BaseCommit)
	}
	if !strings.Contains(wt.Name, "my-workflow_") {
		t.Fatalf("expected persisted name to contain 'my-workflow_', got %s", wt.Name)
	}
	if strings.Contains(runGitOutput(t, repo.dir, "status", "--porcelain=v1"), "agentflow-worktrees") {
		t.Fatalf("expected default worktree path to stay out of tracked status")
	}
}

func TestCreateWorktree_NormalizeName(t *testing.T) {
	ctx := context.Background()
	repo := initRepo(t)
	provider := newProvider(t)

	wt, err := provider.Create(ctx, ports.CreateWorktreeRequest{
		WorkflowName: "Flávio's Workflow / API v1.0!",
		BaseCommit:   repo.head,
		WorkingDir:   repo.dir,
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	defer cleanupWorktree(t, ctx, provider, wt)

	expected := "Fl-vio-s-Workflow-API-v1.0"
	if !strings.Contains(wt.Path, expected+"_") {
		t.Fatalf("expected normalized name %q in path, got %s", expected, wt.Path)
	}
}

func TestCreateWorktree_NotAGitRepo(t *testing.T) {
	ctx := context.Background()
	provider := newProvider(t)
	tmp := t.TempDir()

	_, err := provider.Create(ctx, ports.CreateWorktreeRequest{
		WorkflowName: "wf",
		BaseCommit:   "abc123",
		WorkingDir:   tmp,
	})
	if err == nil {
		t.Fatal("expected error for non-git repo")
	}
	if !errors.Is(err, ports.ErrWorktreeStructural) {
		t.Fatalf("expected structural error, got %v", err)
	}
}

func TestCreateWorktree_MergeInProgress(t *testing.T) {
	ctx := context.Background()
	repo := initRepo(t)
	provider := newProvider(t)

	// Create a second branch and start a merge.
	runGit(t, repo.dir, "checkout", "-b", "feature")
	writeFile(t, filepath.Join(repo.dir, "feat.txt"), "feat")
	runGit(t, repo.dir, "add", ".")
	runGit(t, repo.dir, "commit", "-m", "feature")
	runGit(t, repo.dir, "checkout", "main")
	// Start merge but abort immediately after creating MERGE_HEAD manually
	writeFile(t, filepath.Join(repo.dir, ".git", "MERGE_HEAD"), repo.head)

	_, err := provider.Create(ctx, ports.CreateWorktreeRequest{
		WorkflowName: "wf",
		BaseCommit:   repo.head,
		WorkingDir:   repo.dir,
	})
	if err == nil {
		t.Fatal("expected error when merge in progress")
	}
	if !errors.Is(err, ports.ErrWorktreeStructural) {
		t.Fatalf("expected structural error, got %v", err)
	}
}

func TestCreateWorktree_RequireCleanRejectsDirtyWorkspace(t *testing.T) {
	ctx := context.Background()
	repo := initRepo(t)
	provider := newProvider(t)
	writeFile(t, filepath.Join(repo.dir, "dirty.txt"), "dirty")

	_, err := provider.Create(ctx, ports.CreateWorktreeRequest{
		WorkflowName: "wf",
		BaseCommit:   repo.head,
		WorkingDir:   repo.dir,
		CleanPolicy:  ports.WorktreeCleanRequire,
	})
	if err == nil {
		t.Fatal("expected dirty workspace error")
	}
	if !errors.Is(err, ports.ErrWorktreeStructural) {
		t.Fatalf("expected structural error, got %v", err)
	}
}

func TestDiff_NoChanges(t *testing.T) {
	ctx := context.Background()
	repo := initRepo(t)
	provider := newProvider(t)

	wt, err := provider.Create(ctx, ports.CreateWorktreeRequest{
		WorkflowName: "wf",
		BaseCommit:   repo.head,
		WorkingDir:   repo.dir,
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	defer cleanupWorktree(t, ctx, provider, wt)

	cs, err := provider.Diff(ctx, wt)
	if err != nil {
		t.Fatalf("diff failed: %v", err)
	}
	if !cs.Empty {
		t.Fatalf("expected empty changeset")
	}
	if len(cs.Files) != 0 {
		t.Fatalf("expected no files, got %d", len(cs.Files))
	}
}

func TestDiff_WithChanges(t *testing.T) {
	ctx := context.Background()
	repo := initRepo(t)
	provider := newProvider(t)

	wt, err := provider.Create(ctx, ports.CreateWorktreeRequest{
		WorkflowName: "wf",
		BaseCommit:   repo.head,
		WorkingDir:   repo.dir,
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	defer cleanupWorktree(t, ctx, provider, wt)

	// Modify existing file.
	writeFile(t, filepath.Join(wt.Path, "hello.txt"), "modified")
	// Add new file.
	writeFile(t, filepath.Join(wt.Path, "new.txt"), "new content")
	// Remove file.
	os.Remove(filepath.Join(wt.Path, "world.txt"))

	cs, err := provider.Diff(ctx, wt)
	if err != nil {
		t.Fatalf("diff failed: %v", err)
	}
	if cs.Empty {
		t.Fatal("expected non-empty changeset")
	}
	if len(cs.Files) != 3 {
		t.Fatalf("expected 3 file changes, got %d", len(cs.Files))
	}

	statusByPath := make(map[string]string)
	for _, f := range cs.Files {
		statusByPath[f.Path] = f.Status
	}
	if statusByPath["hello.txt"] != "M" {
		t.Fatalf("expected hello.txt modified, got %s", statusByPath["hello.txt"])
	}
	if statusByPath["new.txt"] != "A" {
		t.Fatalf("expected new.txt added, got %s", statusByPath["new.txt"])
	}
	if statusByPath["world.txt"] != "D" {
		t.Fatalf("expected world.txt deleted, got %s", statusByPath["world.txt"])
	}
}

func TestApply_TargetChanged(t *testing.T) {
	ctx := context.Background()
	repo := initRepo(t)
	provider := newProvider(t)

	wt, err := provider.Create(ctx, ports.CreateWorktreeRequest{
		WorkflowName: "wf",
		BaseCommit:   repo.head,
		WorkingDir:   repo.dir,
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	defer cleanupWorktree(t, ctx, provider, wt)

	// Make a change in worktree and capture diff.
	writeFile(t, filepath.Join(wt.Path, "hello.txt"), "from worktree")
	cs, err := provider.Diff(ctx, wt)
	if err != nil {
		t.Fatalf("diff failed: %v", err)
	}

	// Advance target repo HEAD by committing a change.
	writeFile(t, filepath.Join(repo.dir, "other.txt"), "other")
	runGit(t, repo.dir, "add", ".")
	runGit(t, repo.dir, "commit", "-m", "advance")

	_, err = provider.Apply(ctx, ports.ApplyWorktreeRequest{
		Worktree:   wt,
		TargetDir:  repo.dir,
		BaseCommit: repo.head,
		Diff:       cs.Diff,
	})
	if err == nil {
		t.Fatal("expected apply to fail when target changed")
	}
	if !errors.Is(err, ports.ErrWorktreeRecoverable) {
		t.Fatalf("expected recoverable error, got %v", err)
	}
}

func TestApply_TargetDirty(t *testing.T) {
	ctx := context.Background()
	repo := initRepo(t)
	provider := newProvider(t)

	wt, err := provider.Create(ctx, ports.CreateWorktreeRequest{
		WorkflowName: "wf",
		BaseCommit:   repo.head,
		WorkingDir:   repo.dir,
		CleanPolicy:  ports.WorktreeCleanRequire,
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	defer cleanupWorktree(t, ctx, provider, wt)

	writeFile(t, filepath.Join(wt.Path, "hello.txt"), "from worktree")
	cs, err := provider.Diff(ctx, wt)
	if err != nil {
		t.Fatalf("diff failed: %v", err)
	}

	writeFile(t, filepath.Join(repo.dir, "local.txt"), "local")
	_, err = provider.Apply(ctx, ports.ApplyWorktreeRequest{
		Worktree:   wt,
		TargetDir:  repo.dir,
		BaseCommit: repo.head,
		Diff:       cs.Diff,
	})
	if err == nil {
		t.Fatal("expected apply to fail when target is dirty")
	}
	if !errors.Is(err, ports.ErrWorktreeRecoverable) {
		t.Fatalf("expected recoverable error, got %v", err)
	}
}

func TestApply_PatchFailureIsResolvable(t *testing.T) {
	ctx := context.Background()
	repo := initRepo(t)
	provider := newProvider(t)

	wt, err := provider.Create(ctx, ports.CreateWorktreeRequest{
		WorkflowName: "wf",
		BaseCommit:   repo.head,
		WorkingDir:   repo.dir,
		CleanPolicy:  ports.WorktreeCleanRequire,
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	defer cleanupWorktree(t, ctx, provider, wt)

	badDiff := "diff --git a/hello.txt b/hello.txt\n--- a/hello.txt\n+++ b/hello.txt\n@@ -1 +1 @@\n-not-current\n+new\n"
	res, err := provider.Apply(ctx, ports.ApplyWorktreeRequest{
		Worktree:   wt,
		TargetDir:  repo.dir,
		BaseCommit: repo.head,
		Diff:       badDiff,
	})
	if err == nil {
		t.Fatal("expected apply failure")
	}
	if !errors.Is(err, ports.ErrWorktreeResolvable) {
		t.Fatalf("expected resolvable error, got %v", err)
	}
	if len(res.Conflicts) == 0 || res.Conflicts[0].Path != "hello.txt" {
		t.Fatalf("expected hello.txt conflict, got %+v", res.Conflicts)
	}
}

func TestStatus_CleanAndDirty(t *testing.T) {
	ctx := context.Background()
	repo := initRepo(t)
	provider := newProvider(t)

	wt, err := provider.Create(ctx, ports.CreateWorktreeRequest{
		WorkflowName: "wf",
		BaseCommit:   repo.head,
		WorkingDir:   repo.dir,
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	defer cleanupWorktree(t, ctx, provider, wt)

	status, err := provider.Status(ctx, wt)
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if !status.Clean {
		t.Fatal("expected clean status")
	}

	writeFile(t, filepath.Join(wt.Path, "dirty.txt"), "dirty")
	status, err = provider.Status(ctx, wt)
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if status.Clean {
		t.Fatal("expected dirty status")
	}
	if len(status.Files) != 1 || status.Files[0].Path != "dirty.txt" {
		t.Fatalf("expected dirty.txt in status, got %+v", status.Files)
	}
}

func TestCleanup_RemovesWorktree(t *testing.T) {
	ctx := context.Background()
	repo := initRepo(t)
	provider := newProvider(t)

	wt, err := provider.Create(ctx, ports.CreateWorktreeRequest{
		WorkflowName: "wf",
		BaseCommit:   repo.head,
		WorkingDir:   repo.dir,
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	res, err := provider.Cleanup(ctx, ports.CleanupWorktreeRequest{Worktree: wt, Force: true})
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
	if !res.Removed {
		t.Fatal("expected removed")
	}
	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Fatalf("expected worktree path to be removed")
	}
}

// ---- helpers ----

type testRepo struct {
	dir  string
	head string
}

func initRepo(t *testing.T) testRepo {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@agentflow")
	runGit(t, dir, "config", "user.name", "Test")
	writeFile(t, filepath.Join(dir, "hello.txt"), "hello")
	writeFile(t, filepath.Join(dir, "world.txt"), "world")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial")
	head := strings.TrimSpace(runGitOutput(t, dir, "rev-parse", "HEAD"))
	return testRepo{dir: dir, head: head}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	out := runGitOutput(t, dir, args...)
	_ = out
}

func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := "git " + strings.Join(args, " ")
	runner := shell.NewRunner()
	res, err := runner.Run(context.Background(), ports.ShellRequest{
		Command:    cmd,
		WorkingDir: dir,
	})
	if err != nil && res.ExitCode != 0 {
		t.Fatalf("git %s failed (exit %d): %s", strings.Join(args, " "), res.ExitCode, res.Stderr)
	}
	return res.Stdout
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func newProvider(t *testing.T) *Provider {
	t.Helper()
	return New(shell.NewRunner())
}

func cleanupWorktree(t *testing.T, ctx context.Context, p *Provider, wt ports.Worktree) {
	t.Helper()
	_, _ = p.Cleanup(ctx, ports.CleanupWorktreeRequest{Worktree: wt, Force: true})
}
