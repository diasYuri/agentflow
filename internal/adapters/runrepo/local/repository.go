package local

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

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
	if err := writeJSON(filepath.Join(dir, "run.json"), meta); err != nil {
		return run.RunHandle{}, err
	}
	return run.RunHandle{RunID: meta.RunID, Dir: dir}, nil
}

func (r *Repository) SaveWorkflow(ctx context.Context, runID string, sourcePath string, normalized any, plan any) error {
	_ = ctx
	dir := r.runs[runID]
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
	parts := []string{r.runs[runID], "nodes"}
	parts = append(parts, result.Path...)
	if len(result.Path) == 0 || result.Path[len(result.Path)-1] != result.NodeID {
		parts = append(parts, result.NodeID)
	}
	dir := filepath.Join(parts...)
	if result.InstanceID != "" {
		dir = filepath.Join(dir, result.InstanceID)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if result.Stdout != "" {
		if err := os.WriteFile(filepath.Join(dir, "stdout.txt"), []byte(result.Stdout), 0o644); err != nil {
			return err
		}
	}
	if result.Stderr != "" {
		if err := os.WriteFile(filepath.Join(dir, "stderr.txt"), []byte(result.Stderr), 0o644); err != nil {
			return err
		}
	}
	return writeJSON(filepath.Join(dir, "result.json"), result)
}

func (r *Repository) SaveArtifact(ctx context.Context, runID string, name string, data []byte) error {
	_ = ctx
	path := filepath.Join(r.runs[runID], "artifacts", filepath.Clean(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (r *Repository) FinalizeRun(ctx context.Context, runID string, summary run.Summary) error {
	_ = ctx
	return writeJSON(filepath.Join(r.runs[runID], "summary.json"), summary)
}

func (r *Repository) RunDir(runID string) (string, bool) {
	dir, ok := r.runs[runID]
	return dir, ok
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func defaultRunRoot() string {
	root := filepath.Join(".agentflow", "runs")
	if home, err := os.UserHomeDir(); err == nil {
		root = filepath.Join(home, ".agentflow", "runs")
	}
	return filepath.Clean(root)
}
