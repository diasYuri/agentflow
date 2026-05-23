package chatagent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	yamlrepo "github.com/diasYuri/agentflow/internal/adapters/yaml"
)

type projectWorkflowSummary struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
	NodeCount   int    `json:"node_count"`
	Path        string `json:"path"`
	Source      string `json:"source"`
}

type listWorkflowsOutput struct {
	Definitions []projectWorkflowSummary `json:"definitions"`
}

func listProjectWorkflows(projectPath string) ([]projectWorkflowSummary, error) {
	projectPath = strings.TrimSpace(projectPath)
	if projectPath == "" {
		return nil, nil
	}

	roots := []struct {
		path   string
		source string
	}{
		{path: filepath.Join(projectPath, ".agentflow", "workflows"), source: "project"},
		{path: filepath.Join(projectPath, "samples", "workflows"), source: "samples"},
	}
	seen := make(map[string]struct{})
	var definitions []projectWorkflowSummary
	for _, root := range roots {
		entries, err := os.ReadDir(root.path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read workflow definitions %s: %w", root.path, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !isWorkflowDefinitionFile(entry.Name()) {
				continue
			}
			path := filepath.Join(root.path, entry.Name())
			spec, err := yamlrepo.DecodeWorkflow(path)
			if err != nil {
				continue
			}
			if spec.Name == "" {
				continue
			}
			if _, ok := seen[spec.Name]; ok {
				continue
			}
			seen[spec.Name] = struct{}{}
			definitions = append(definitions, projectWorkflowSummary{
				Name:        spec.Name,
				Version:     spec.Version,
				Description: spec.Description,
				NodeCount:   len(spec.Nodes),
				Path:        path,
				Source:      root.source,
			})
		}
	}
	sort.SliceStable(definitions, func(i, j int) bool {
		return definitions[i].Name < definitions[j].Name
	})
	return definitions, nil
}

func listDaemonWorkflows(ctx context.Context, env *ToolEnvironment) ([]projectWorkflowSummary, error) {
	if env.Definitions == nil {
		return nil, nil
	}
	resp, err := env.Definitions.ListWorkflowDefinitions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list workflow definitions: %w", err)
	}
	out := make([]projectWorkflowSummary, 0, len(resp.Definitions))
	for _, def := range resp.Definitions {
		out = append(out, projectWorkflowSummary{
			Name:        def.Name,
			Version:     def.Version,
			Description: def.Description,
			Path:        def.ID,
			Source:      "daemon",
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func isWorkflowDefinitionFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".yaml", ".yml":
		return true
	default:
		return false
	}
}
