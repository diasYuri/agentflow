package pi

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/diasYuri/agentflow/internal/core/ports"
	"github.com/google/uuid"
)

const (
	maxNameLen   = 50
	branchPrefix = "agentflow"
)

var repeatedHyphen = regexp.MustCompile(`-+$`)

// Provider implements the pi worktree provider using real Git commands.
type Provider struct {
	shell  ports.ShellRunner
	uuidFn func() (uuid.UUID, error)
}

// New creates a new pi worktree provider.
func New(shell ports.ShellRunner) *Provider {
	return &Provider{shell: shell, uuidFn: uuid.NewV7}
}

// Create validates the repository and creates a new Git worktree.
func (p *Provider) Create(ctx context.Context, req ports.CreateWorktreeRequest) (ports.Worktree, error) {
	var cmds []ports.GitCommand

	// 1. Git installed.
	r, err := p.git(ctx, req.WorkingDir, "--version")
	cmds = append(cmds, r.toGitCommand())
	if err != nil || r.ExitCode != 0 {
		return ports.Worktree{}, fmt.Errorf("%w: git not available: %s", ports.ErrWorktreeStructural, r.Stderr)
	}

	// 2. Resolve repository root.
	r, err = p.git(ctx, req.WorkingDir, "rev-parse", "--show-toplevel")
	cmds = append(cmds, r.toGitCommand())
	if err != nil || r.ExitCode != 0 {
		return ports.Worktree{}, fmt.Errorf("%w: not a git repository: %s", ports.ErrWorktreeStructural, r.Stderr)
	}
	repoPath := strings.TrimSpace(r.Stdout)

	// 3. Validate base commit matches HEAD.
	r, err = p.git(ctx, repoPath, "rev-parse", "HEAD")
	cmds = append(cmds, r.toGitCommand())
	if err != nil || r.ExitCode != 0 {
		return ports.Worktree{}, fmt.Errorf("%w: unable to read HEAD: %s", ports.ErrWorktreeStructural, r.Stderr)
	}
	currentHead := strings.TrimSpace(r.Stdout)
	if currentHead != req.BaseCommit {
		return ports.Worktree{}, fmt.Errorf("%w: HEAD %s does not match expected base_commit %s", ports.ErrWorktreeStructural, currentHead, req.BaseCommit)
	}

	// 4. Check ambiguous states.
	ambiguousStates := []struct {
		file string
		name string
	}{
		{"MERGE_HEAD", "merge"},
		{"REBASE_HEAD", "rebase"},
		{"CHERRY_PICK_HEAD", "cherry-pick"},
		{"REVERT_HEAD", "revert"},
	}
	for _, st := range ambiguousStates {
		r, _ = p.git(ctx, repoPath, "rev-parse", "--verify", st.file)
		cmds = append(cmds, r.toGitCommand())
		if r.ExitCode == 0 {
			return ports.Worktree{}, fmt.Errorf("%w: %s in progress", ports.ErrWorktreeStructural, st.name)
		}
	}

	// 5. Clean policy.
	if req.CleanPolicy == ports.WorktreeCleanRequire {
		r, err = p.git(ctx, repoPath, "status", "--porcelain=v1")
		cmds = append(cmds, r.toGitCommand())
		if err != nil || r.ExitCode != 0 {
			return ports.Worktree{}, fmt.Errorf("%w: unable to check status: %s", ports.ErrWorktreeStructural, r.Stderr)
		}
		if strings.TrimSpace(r.Stdout) != "" {
			return ports.Worktree{}, fmt.Errorf("%w: working directory has local changes", ports.ErrWorktreeStructural)
		}
	}

	// 6. Generate deterministic name.
	uid, err := p.uuidFn()
	if err != nil {
		return ports.Worktree{}, fmt.Errorf("%w: failed to generate uuid: %w", ports.ErrWorktreeStructural, err)
	}
	normalized := normalizeWorkflowName(req.WorkflowName)
	branchName := fmt.Sprintf("%s/%s_%s", branchPrefix, normalized, uid.String())
	dirName := fmt.Sprintf("%s_%s", normalized, uid.String())

	// Determine worktree path.
	wtPath := req.TargetDir
	if wtPath == "" {
		wtPath = filepath.Join(repoPath, ".git", "agentflow-worktrees", dirName)
	}
	// Validate path is inside repo or allowed directory to avoid traversal.
	absPath, err := filepath.Abs(wtPath)
	if err != nil {
		return ports.Worktree{}, fmt.Errorf("%w: invalid target dir: %w", ports.ErrWorktreeStructural, err)
	}
	absRepo, err := filepath.Abs(repoPath)
	if err != nil {
		return ports.Worktree{}, fmt.Errorf("%w: invalid repo path: %w", ports.ErrWorktreeStructural, err)
	}
	rel, err := filepath.Rel(absRepo, absPath)
	if err != nil {
		return ports.Worktree{}, fmt.Errorf("%w: invalid worktree path: %w", ports.ErrWorktreeStructural, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ports.Worktree{}, fmt.Errorf("%w: worktree path must be inside repository", ports.ErrWorktreeStructural)
	}
	if err := os.MkdirAll(filepath.Dir(wtPath), 0755); err != nil {
		return ports.Worktree{}, fmt.Errorf("%w: unable to create worktree parent dir: %w", ports.ErrWorktreeStructural, err)
	}

	// 7. Create worktree.
	r, err = p.git(ctx, repoPath, "worktree", "add", "-b", branchName, wtPath, req.BaseCommit)
	cmds = append(cmds, r.toGitCommand())
	if err != nil || r.ExitCode != 0 {
		return ports.Worktree{}, fmt.Errorf("%w: git worktree add failed: %s", ports.ErrWorktreeStructural, r.Stderr)
	}

	return ports.Worktree{
		ID:           uid.String(),
		Name:         dirName,
		Path:         wtPath,
		Branch:       branchName,
		BaseCommit:   req.BaseCommit,
		WorkflowName: req.WorkflowName,
		Commands:     cmds,
	}, nil
}

// Status returns the current status of the worktree.
func (p *Provider) Status(ctx context.Context, worktree ports.Worktree) (ports.WorktreeStatus, error) {
	var cmds []ports.GitCommand

	r, err := p.git(ctx, worktree.Path, "status", "--porcelain=v1")
	cmds = append(cmds, r.toGitCommand())
	if err != nil || r.ExitCode != 0 {
		return ports.WorktreeStatus{}, fmt.Errorf("%w: git status failed: %s", ports.ErrWorktreeStructural, r.Stderr)
	}

	raw := r.Stdout
	files := parsePorcelainV1(raw)

	return ports.WorktreeStatus{
		Clean:    len(files) == 0,
		Files:    files,
		Raw:      raw,
		Commands: cmds,
	}, nil
}

// Diff returns a deterministic diff and changeset for the worktree.
func (p *Provider) Diff(ctx context.Context, worktree ports.Worktree) (ports.ChangeSet, error) {
	var cmds []ports.GitCommand

	// Stage untracked files as intent-to-add so they appear in diff.
	r, _ := p.git(ctx, worktree.Path, "add", "-N", ".")
	cmds = append(cmds, r.toGitCommand())

	// Diff binary and full-index.
	r, err := p.git(ctx, worktree.Path, "diff", "--binary", "--full-index", "--find-renames", "HEAD", "--")
	cmds = append(cmds, r.toGitCommand())
	if err != nil || r.ExitCode != 0 {
		_ = p.gitNoErr(ctx, worktree.Path, "reset")
		return ports.ChangeSet{}, fmt.Errorf("%w: git diff failed: %s", ports.ErrWorktreeStructural, r.Stderr)
	}
	diffText := r.Stdout

	// Name-status for structured file list.
	r2, err := p.git(ctx, worktree.Path, "diff", "--name-status", "--find-renames", "HEAD", "--")
	cmds = append(cmds, r2.toGitCommand())
	if err != nil || r2.ExitCode != 0 {
		_ = p.gitNoErr(ctx, worktree.Path, "reset")
		return ports.ChangeSet{}, fmt.Errorf("%w: git diff --name-status failed: %s", ports.ErrWorktreeStructural, r2.Stderr)
	}
	files := parseNameStatus(r2.Stdout)

	// Reset intent-to-add.
	r3, _ := p.git(ctx, worktree.Path, "reset")
	cmds = append(cmds, r3.toGitCommand())

	return ports.ChangeSet{
		Empty:    len(files) == 0 && strings.TrimSpace(diffText) == "",
		Files:    files,
		Diff:     diffText,
		Commands: cmds,
	}, nil
}

// Apply applies the given diff to the target directory.
func (p *Provider) Apply(ctx context.Context, req ports.ApplyWorktreeRequest) (ports.MergeResult, error) {
	var cmds []ports.GitCommand

	// Validate target HEAD still matches base commit.
	r, err := p.git(ctx, req.TargetDir, "rev-parse", "HEAD")
	cmds = append(cmds, r.toGitCommand())
	if err != nil || r.ExitCode != 0 {
		return ports.MergeResult{}, fmt.Errorf("%w: unable to read target HEAD: %s", ports.ErrWorktreeStructural, r.Stderr)
	}
	if strings.TrimSpace(r.Stdout) != req.BaseCommit {
		return ports.MergeResult{}, fmt.Errorf("%w: target HEAD changed since base_commit", ports.ErrWorktreeStructural)
	}

	r, err = p.git(ctx, req.TargetDir, "status", "--porcelain=v1")
	cmds = append(cmds, r.toGitCommand())
	if err != nil || r.ExitCode != 0 {
		return ports.MergeResult{}, fmt.Errorf("%w: unable to check target status: %s", ports.ErrWorktreeStructural, r.Stderr)
	}
	if strings.TrimSpace(r.Stdout) != "" {
		return ports.MergeResult{}, fmt.Errorf("%w: target working directory has local changes", ports.ErrWorktreeStructural)
	}

	// Write diff to temporary file and apply.
	tmpFile, err := writeTempPatch(req.Diff)
	if err != nil {
		return ports.MergeResult{}, fmt.Errorf("%w: failed to write patch file: %w", ports.ErrWorktreeStructural, err)
	}
	defer os.Remove(tmpFile)

	r, err = p.git(ctx, req.TargetDir, "apply", "--index", tmpFile)
	cmds = append(cmds, r.toGitCommand())
	if err != nil || r.ExitCode != 0 {
		conflicts := parseApplyConflicts(r.Stderr)
		return ports.MergeResult{
			Success:   false,
			Conflicts: conflicts,
			Commands:  cmds,
		}, fmt.Errorf("%w: git apply failed: %s", ports.ErrWorktreeResolvable, r.Stderr)
	}

	return ports.MergeResult{
		Success:  true,
		Commands: cmds,
	}, nil
}

func writeTempPatch(diff string) (string, error) {
	f, err := os.CreateTemp("", "agentflow-apply-*.patch")
	if err != nil {
		return "", err
	}
	name := f.Name()
	if _, err := f.WriteString(diff); err != nil {
		_ = f.Close()
		_ = os.Remove(name)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return name, nil
}

// Cleanup removes the worktree.
func (p *Provider) Cleanup(ctx context.Context, req ports.CleanupWorktreeRequest) (ports.CleanupResult, error) {
	var cmds []ports.GitCommand

	// Resolve repo root from worktree path.
	r, err := p.git(ctx, req.Worktree.Path, "rev-parse", "--show-toplevel")
	cmds = append(cmds, r.toGitCommand())
	repoPath := ""
	if err == nil && r.ExitCode == 0 {
		repoPath = strings.TrimSpace(r.Stdout)
	}

	args := []string{"worktree", "remove"}
	if req.Force {
		args = append(args, "--force")
	}
	args = append(args, req.Worktree.Path)

	workingDir := repoPath
	if workingDir == "" {
		workingDir = req.Worktree.Path
	}
	r, err = p.git(ctx, workingDir, args...)
	cmds = append(cmds, r.toGitCommand())
	if err != nil || r.ExitCode != 0 {
		return ports.CleanupResult{
			Removed:  false,
			Commands: cmds,
		}, fmt.Errorf("%w: git worktree remove failed: %s", ports.ErrWorktreeStructural, r.Stderr)
	}

	return ports.CleanupResult{
		Removed:  true,
		Commands: cmds,
	}, nil
}

// git executes a git command through the shell runner.
func (p *Provider) git(ctx context.Context, dir string, args ...string) (gitResult, error) {
	cmd := "git " + strings.Join(args, " ")
	res, err := p.shell.Run(ctx, ports.ShellRequest{
		Command:    cmd,
		WorkingDir: dir,
	})
	return gitResult{ShellResult: res, Command: cmd}, err
}

func (p *Provider) gitNoErr(ctx context.Context, dir string, args ...string) gitResult {
	r, _ := p.git(ctx, dir, args...)
	return r
}

type gitResult struct {
	ports.ShellResult
	Command string
}

func (r gitResult) toGitCommand() ports.GitCommand {
	return ports.GitCommand{
		Command:  r.Command,
		ExitCode: r.ExitCode,
		Stdout:   r.Stdout,
		Stderr:   r.Stderr,
	}
}

// normalizeWorkflowName normalizes a workflow name for safe filesystem and git branch usage.
func normalizeWorkflowName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case unicode.IsLetter(r) && r < unicode.MaxASCII,
			unicode.IsDigit(r),
			r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	s := b.String()
	// Collapse repeated hyphens.
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	// Trim dangerous prefix/suffix.
	s = strings.Trim(s, "-.")
	if s == "" {
		s = "workflow"
	}
	if len(s) > maxNameLen {
		s = s[:maxNameLen]
	}
	s = repeatedHyphen.ReplaceAllString(s, "")
	return s
}

func parsePorcelainV1(raw string) []ports.FileStatus {
	var files []ports.FileStatus
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(line) < 3 {
			continue
		}
		status := strings.TrimSpace(line[:2])
		path := strings.TrimSpace(line[3:])
		if status == "??" {
			status = "?"
		}
		files = append(files, ports.FileStatus{Path: path, Status: status})
	}
	return files
}

func parseNameStatus(raw string) []ports.FileChange {
	var files []ports.FileChange
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}
		status := strings.TrimSpace(parts[0])
		// Rename status looks like "R100"
		if strings.HasPrefix(status, "R") && len(parts) >= 3 {
			files = append(files, ports.FileChange{
				Path:    parts[2],
				OldPath: parts[1],
				Status:  "R",
			})
			continue
		}
		files = append(files, ports.FileChange{
			Path:   parts[1],
			Status: status,
		})
	}
	return files
}

func parseApplyConflicts(stderr string) []ports.Conflict {
	var conflicts []ports.Conflict
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "error: patch failed: ") {
			rest := strings.TrimPrefix(line, "error: patch failed: ")
			path := rest
			if idx := strings.LastIndex(rest, ":"); idx > 0 {
				path = rest[:idx]
			}
			conflicts = append(conflicts, ports.Conflict{Path: path, Reason: line})
			continue
		}
		if strings.HasPrefix(line, "error: ") && strings.HasSuffix(line, ": patch does not apply") {
			rest := strings.TrimPrefix(line, "error: ")
			path := strings.TrimSuffix(rest, ": patch does not apply")
			conflicts = append(conflicts, ports.Conflict{Path: path, Reason: line})
			continue
		}
		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			conflicts = append(conflicts, ports.Conflict{
				Path:   strings.TrimSpace(parts[0]),
				Reason: strings.TrimSpace(parts[1]),
			})
		}
	}
	return conflicts
}
