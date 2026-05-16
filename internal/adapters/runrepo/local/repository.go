package local

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diasYuri/agentflow/internal/core/run"
)

type Repository struct {
	baseDir string
	runs    map[string]string
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
	return checkpoint, nil
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

func (r *Repository) SaveArtifact(ctx context.Context, runID string, name string, data []byte) error {
	_ = ctx
	dir := r.ensureRunDir(runID)
	path, err := safeArtifactPath(dir, name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func safeArtifactPath(baseDir, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("artifact name is empty")
	}
	clean := filepath.Clean(name)
	if filepath.IsAbs(clean) {
		return "", fmt.Errorf("artifact name is absolute")
	}
	if strings.Contains(clean, "..") {
		return "", fmt.Errorf("artifact name contains ..")
	}
	full := filepath.Join(baseDir, "artifacts", clean)
	rel, err := filepath.Rel(baseDir, full)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("artifact path escapes base directory")
	}
	return full, nil
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
	tmp, err := os.CreateTemp(filepath.Dir(path), ".checkpoint-*.tmp")
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
