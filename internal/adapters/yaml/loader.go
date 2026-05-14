package yaml

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/diasYuri/agentflow/internal/core/workflow"
)

type WorkflowRepository struct {
	localRoot  string
	globalRoot string
}

func NewWorkflowRepository(roots ...string) *WorkflowRepository {
	localRoot, globalRoot := defaultWorkflowRoots()
	if len(roots) > 0 && strings.TrimSpace(roots[0]) != "" {
		localRoot = roots[0]
	}
	if len(roots) > 1 && strings.TrimSpace(roots[1]) != "" {
		globalRoot = roots[1]
	}
	return &WorkflowRepository{localRoot: localRoot, globalRoot: globalRoot}
}

func (r *WorkflowRepository) Load(ctx context.Context, ref string) (*workflow.WorkflowSpec, string, error) {
	_ = ctx
	name := strings.TrimSpace(ref)
	if name == "" {
		return nil, "", fmt.Errorf("workflow name is required")
	}

	localSpec, localPath, localErrs, err := r.findInScope(ctx, r.localRoot, name)
	if err != nil {
		return nil, "", fmt.Errorf("local workflows: %w", err)
	}
	if localSpec != nil {
		return localSpec, localPath, nil
	}

	globalSpec, globalPath, globalErrs, err := r.findInScope(ctx, r.globalRoot, name)
	if err != nil {
		return nil, "", fmt.Errorf("global workflows: %w", err)
	}
	if globalSpec != nil {
		return globalSpec, globalPath, nil
	}

	var scanErrs []error
	scanErrs = append(scanErrs, localErrs...)
	scanErrs = append(scanErrs, globalErrs...)
	if len(scanErrs) > 0 {
		return nil, "", fmt.Errorf("workflow %q not found: %w", name, errors.Join(scanErrs...))
	}
	return nil, "", fmt.Errorf("workflow %q not found in %q or %q", name, r.localRoot, r.globalRoot)
}

func (r *WorkflowRepository) findInScope(ctx context.Context, root string, name string) (*workflow.WorkflowSpec, string, []error, error) {
	if strings.TrimSpace(root) == "" {
		return nil, "", nil, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", nil, nil
		}
		return nil, "", []error{fmt.Errorf("%s: %w", root, err)}, nil
	}

	var (
		matches  []scopeMatch
		scanErrs []error
	)
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, "", scanErrs, err
		}
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".yaml", ".yml":
		default:
			continue
		}
		spec, decodeErr := decodeWorkflow(path)
		if decodeErr != nil {
			scanErrs = append(scanErrs, fmt.Errorf("%s: %w", path, decodeErr))
			continue
		}
		if spec.Name != name {
			continue
		}
		matches = append(matches, scopeMatch{spec: spec, path: path})
	}

	if len(matches) > 1 {
		paths := make([]string, 0, len(matches))
		for _, match := range matches {
			paths = append(paths, match.path)
		}
		return nil, "", scanErrs, fmt.Errorf("duplicate workflow name %q in %q: %s", name, root, strings.Join(paths, ", "))
	}
	if len(matches) == 1 {
		return matches[0].spec, matches[0].path, scanErrs, nil
	}
	return nil, "", scanErrs, nil
}

type scopeMatch struct {
	spec *workflow.WorkflowSpec
	path string
}

func decodeWorkflow(path string) (*workflow.WorkflowSpec, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	var spec workflow.WorkflowSpec
	if err := decoder.Decode(&spec); err != nil {
		return nil, fmt.Errorf("decode workflow yaml: %w", err)
	}
	if spec.Inputs == nil {
		spec.Inputs = map[string]workflow.InputSpec{}
	}
	if spec.Vars == nil {
		spec.Vars = map[string]any{}
	}
	return &spec, nil
}

func defaultWorkflowRoots() (string, string) {
	localRoot := filepath.Join("agentflow", "workflows")
	if cwd, err := os.Getwd(); err == nil {
		localRoot = filepath.Join(cwd, localRoot)
	}

	globalRoot := filepath.Join(".agentflow", "workflows")
	if home, err := os.UserHomeDir(); err == nil {
		globalRoot = filepath.Join(home, ".agentflow", "workflows")
	}
	return filepath.Clean(localRoot), filepath.Clean(globalRoot)
}
