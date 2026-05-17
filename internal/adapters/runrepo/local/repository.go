package local

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/diasYuri/agentflow/internal/core/run"
)

type Repository struct {
	baseDir string
	runs    map[string]string
	mu      sync.Mutex
}

func New(baseDir string) *Repository {
	if baseDir == "" {
		baseDir = defaultRunRoot()
	}
	return &Repository{baseDir: baseDir, runs: map[string]string{}}
}

func (r *Repository) CreateRun(ctx context.Context, meta run.RunMetadata) (run.RunHandle, error) {
	_ = ctx
	meta.OutputDir = r.baseDir
	dir := filepath.Join(r.baseDir, meta.RunID)
	if err := os.MkdirAll(filepath.Join(dir, "nodes"), 0o755); err != nil {
		return run.RunHandle{}, err
	}
	if err := os.MkdirAll(filepath.Join(dir, "artifacts"), 0o755); err != nil {
		return run.RunHandle{}, err
	}
	r.runs[meta.RunID] = dir
	runPath := filepath.Join(dir, "run.json")
	if _, err := os.Stat(runPath); err != nil {
		if !os.IsNotExist(err) {
			return run.RunHandle{}, err
		}
		if err := writeJSON(runPath, meta); err != nil {
			return run.RunHandle{}, err
		}
	}
	return run.RunHandle{RunID: meta.RunID, Dir: dir}, nil
}

func (r *Repository) SaveWorkflow(ctx context.Context, runID string, sourcePath string, normalized any, plan any) error {
	_ = ctx
	dir := r.ensureRunDir(runID)
	if sourcePath != "" {
		if data, err := os.ReadFile(sourcePath); err == nil {
			if err := os.WriteFile(filepath.Join(dir, "workflow.yaml"), data, 0o644); err != nil {
				return err
			}
		}
	}
	if err := writeJSON(filepath.Join(dir, "normalized.json"), normalized); err != nil {
		return err
	}
	return writeJSON(filepath.Join(dir, "plan.json"), plan)
}

func (r *Repository) SaveNodeResult(ctx context.Context, runID string, result run.NodeResult) error {
	_ = ctx
	dir := r.ensureRunDir(runID)
	parts := []string{dir, "nodes"}
	parts = append(parts, result.Path...)
	if len(result.Path) == 0 || result.Path[len(result.Path)-1] != result.NodeID {
		parts = append(parts, result.NodeID)
	}
	nodeDir := filepath.Join(parts...)
	if result.InstanceID != "" {
		nodeDir = filepath.Join(nodeDir, result.InstanceID)
	}
	if err := os.MkdirAll(nodeDir, 0o755); err != nil {
		return err
	}
	if result.Stdout != "" {
		if err := os.WriteFile(filepath.Join(nodeDir, "stdout.txt"), []byte(result.Stdout), 0o644); err != nil {
			return err
		}
	}
	if result.Stderr != "" {
		if err := os.WriteFile(filepath.Join(nodeDir, "stderr.txt"), []byte(result.Stderr), 0o644); err != nil {
			return err
		}
	}
	return writeJSON(filepath.Join(nodeDir, "result.json"), result)
}

func (r *Repository) SaveCheckpoint(ctx context.Context, checkpoint run.Checkpoint) error {
	_ = ctx
	dir := r.ensureRunDir(checkpoint.RunID)
	return writeJSONAtomic(filepath.Join(dir, "checkpoint.json"), checkpoint)
}

func (r *Repository) LoadCheckpoint(ctx context.Context, runID string) (run.Checkpoint, error) {
	_ = ctx
	dir := r.ensureRunDir(runID)
	data, err := os.ReadFile(filepath.Join(dir, "checkpoint.json"))
	if err != nil {
		return run.Checkpoint{}, err
	}
	var checkpoint run.Checkpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return run.Checkpoint{}, err
	}
	if checkpoint.RunID == "" {
		checkpoint.RunID = runID
	}
	if checkpoint.Tag == "" {
		checkpoint.Tag = r.loadRunTag(dir)
	}
	return checkpoint, nil
}

func (r *Repository) loadRunTag(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "run.json"))
	if err != nil {
		return ""
	}
	var meta run.RunMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return ""
	}
	return meta.Tag
}

func (r *Repository) ClearCheckpoint(ctx context.Context, runID string) error {
	_ = ctx
	dir := r.ensureRunDir(runID)
	err := os.Remove(filepath.Join(dir, "checkpoint.json"))
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return err
}

func (r *Repository) SaveArtifact(ctx context.Context, runID string, artifact run.Artifact, data []byte) error {
	_ = ctx
	dir := r.ensureRunDir(runID)

	if artifact.ID == "" {
		return fmt.Errorf("artifact id is empty")
	}
	if artifact.RelativePath == "" {
		artifact.RelativePath = artifact.ID
	}

	path, err := resolveArtifactPath(dir, artifact.RelativePath)
	if err != nil {
		return fmt.Errorf("invalid artifact relative_path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("artifact path is a symlink")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}

	artifact.RunID = runID
	artifact.SizeBytes = int64(len(data))
	if artifact.CreatedAt.IsZero() {
		artifact.CreatedAt = time.Now().UTC()
	}

	return r.updateArtifactIndex(dir, artifact)
}

func (r *Repository) ListArtifacts(ctx context.Context, runID string) ([]run.Artifact, error) {
	_ = ctx
	dir := r.ensureRunDir(runID)
	index, err := r.loadArtifactIndex(dir)
	if err != nil {
		return nil, err
	}
	artifacts := make([]run.Artifact, 0, len(index))
	for _, a := range index {
		artifacts = append(artifacts, a)
	}
	sortArtifacts(artifacts)
	return artifacts, nil
}

func (r *Repository) ReadArtifact(ctx context.Context, runID, artifactID string) ([]byte, run.Artifact, error) {
	_ = ctx
	dir := r.ensureRunDir(runID)
	index, err := r.loadArtifactIndex(dir)
	if err != nil {
		return nil, run.Artifact{}, err
	}
	artifact, ok := index[artifactID]
	if !ok {
		return nil, run.Artifact{}, fmt.Errorf("artifact not found: %s", artifactID)
	}
	path, err := resolveArtifactPath(dir, artifact.RelativePath)
	if err != nil {
		return nil, run.Artifact{}, fmt.Errorf("invalid stored relative_path: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, run.Artifact{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, run.Artifact{}, fmt.Errorf("artifact path is a symlink")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, run.Artifact{}, err
	}
	return data, artifact, nil
}

func (r *Repository) updateArtifactIndex(dir string, artifact run.Artifact) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	index, err := r.loadArtifactIndexLocked(dir)
	if err != nil {
		return err
	}
	index[artifact.ID] = artifact

	indexPath := filepath.Join(dir, "artifacts", "index.json")
	return writeJSONAtomic(indexPath, index)
}

func (r *Repository) loadArtifactIndex(dir string) (map[string]run.Artifact, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.loadArtifactIndexLocked(dir)
}

func (r *Repository) loadArtifactIndexLocked(dir string) (map[string]run.Artifact, error) {
	indexPath := filepath.Join(dir, "artifacts", "index.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]run.Artifact{}, nil
		}
		return nil, err
	}
	var index map[string]run.Artifact
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, err
	}
	if index == nil {
		return map[string]run.Artifact{}, nil
	}
	return index, nil
}

func sortArtifacts(artifacts []run.Artifact) {
	sort.SliceStable(artifacts, func(i, j int) bool {
		a, b := artifacts[i], artifacts[j]
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.Before(b.CreatedAt)
		}
		if a.NodeID != b.NodeID {
			return a.NodeID < b.NodeID
		}
		if a.InstanceID != b.InstanceID {
			return a.InstanceID < b.InstanceID
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.ID < b.ID
	})
}

func validateRelativePath(rel string) error {
	if rel == "" {
		return fmt.Errorf("relative path is empty")
	}
	clean := filepath.Clean(rel)
	if filepath.IsAbs(clean) {
		return fmt.Errorf("relative path is absolute")
	}
	if strings.Contains(clean, "..") {
		return fmt.Errorf("relative path contains ..")
	}
	// Normalize to forward slashes for consistent validation.
	slash := filepath.ToSlash(clean)
	if strings.HasPrefix(slash, "..") {
		return fmt.Errorf("relative path escapes artifacts directory")
	}
	return nil
}

func resolveArtifactPath(runDir, relativePath string) (string, error) {
	if err := validateRelativePath(relativePath); err != nil {
		return "", err
	}
	artifactsDir := filepath.Join(runDir, "artifacts")
	if err := validateNoSymlinkAncestors(artifactsDir, relativePath); err != nil {
		return "", err
	}
	return filepath.Join(artifactsDir, filepath.FromSlash(relativePath)), nil
}

func validateNoSymlinkAncestors(baseDir, relativePath string) error {
	current := baseDir
	parts := strings.Split(filepath.ToSlash(filepath.Clean(relativePath)), "/")
	for i, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("artifact path is a symlink")
		}
		if i < len(parts)-1 && !info.IsDir() {
			return fmt.Errorf("artifact path is not a directory")
		}
	}
	return nil
}

func (r *Repository) FinalizeRun(ctx context.Context, runID string, summary run.Summary) error {
	_ = ctx
	return writeJSON(filepath.Join(r.ensureRunDir(runID), "summary.json"), summary)
}

func (r *Repository) RunDir(runID string) (string, bool) {
	if dir, ok := r.runs[runID]; ok {
		return dir, true
	}
	dir := filepath.Join(r.baseDir, runID)
	if _, err := os.Stat(dir); err == nil {
		r.runs[runID] = dir
		return dir, true
	}
	return "", false
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func writeJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".atomic-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func (r *Repository) ensureRunDir(runID string) string {
	if dir, ok := r.runs[runID]; ok {
		return dir
	}
	dir := filepath.Join(r.baseDir, runID)
	r.runs[runID] = dir
	return dir
}

func defaultRunRoot() string {
	root := filepath.Join(".agentflow", "runs")
	if home, err := os.UserHomeDir(); err == nil {
		root = filepath.Join(home, ".agentflow", "runs")
	}
	return filepath.Clean(root)
}
