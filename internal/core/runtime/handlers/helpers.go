package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	coreports "github.com/diasYuri/agentflow/internal/core/ports"
	corerun "github.com/diasYuri/agentflow/internal/core/run"
	coreworkflow "github.com/diasYuri/agentflow/internal/core/workflow"
)

type Services struct {
	Workflows coreports.WorkflowRepository
	Runs      coreports.RunRepository
	Events    coreports.EventSink
	Agents    coreports.AgentProviderRegistry
	Shell     coreports.ShellRunner
	Worktrees coreports.WorktreeProviderRegistry
	Now       func() time.Time
}

type Options struct {
	WorkflowRef    string
	RunID          string
	ResumeRunID    string
	Inputs         map[string]any
	Vars           map[string]any
	MaxConcurrency int
	WorkingDir     string
	DryRun         bool
	Pause          PauseSignaller
	Tag            string
}

// PauseSignaller is the subset of *runtime.PauseController used by the handlers.
// The runtime package wires the concrete controller; the handlers stay free of
// the import cycle.
type PauseSignaller interface {
	Requested() bool
}

type Result struct {
	RunID   string
	RunDir  string
	Status  corerun.RunStatus
	Summary corerun.Summary
	Plan    coreworkflow.ExecutionPlan
}

type ExecutionRequest struct {
	RunID              string
	ResumeRunID        string
	WorkflowSourcePath string
	Plan               coreworkflow.ExecutionPlan
	Inputs             map[string]any
	WorkingDir         string
	Pause              PauseSignaller
	Tag                string
}

func ResolveInputs(spec coreworkflow.WorkflowSpec, provided map[string]any) (map[string]any, error) {
	resolved := map[string]any{}
	for name, input := range spec.Inputs {
		value, ok := provided[name]
		if !ok {
			value = input.Default
		}
		if value == nil && input.Required {
			return nil, fmt.Errorf("input %q is required", name)
		}
		if value != nil {
			resolved[name] = value
		}
	}
	for name, value := range provided {
		if _, declared := spec.Inputs[name]; !declared {
			resolved[name] = value
		}
	}
	return resolved, nil
}

func ApplyWorkflowOverrides(spec *coreworkflow.WorkflowSpec, opts Options) {
	mergeVars(spec, opts.Vars)
	if opts.MaxConcurrency > 0 {
		spec.Execution.MaxConcurrency = opts.MaxConcurrency
	}
}

func mergeVars(spec *coreworkflow.WorkflowSpec, overrides map[string]any) {
	if spec.Vars == nil {
		spec.Vars = map[string]any{}
	}
	for key, value := range overrides {
		spec.Vars[key] = value
	}
}

func loadSecrets(spec coreworkflow.WorkflowSpec) (map[string]any, error) {
	secrets := map[string]any{}
	for name, secret := range spec.Secrets {
		if secret.Env == "" {
			if secret.Required {
				return nil, fmt.Errorf("secret %q env is required", name)
			}
			continue
		}
		if value, ok := os.LookupEnv(secret.Env); ok {
			secrets[name] = value
			continue
		}
		if secret.Required {
			return nil, fmt.Errorf("secret %q requires environment variable %q", name, secret.Env)
		}
	}
	return secrets, nil
}

func effectiveRetries(spec coreworkflow.WorkflowSpec, node coreworkflow.NodeSpec) int {
	if node.Retries > 0 {
		return node.Retries
	}
	return spec.Defaults.Retries
}

func effectiveTimeout(spec coreworkflow.WorkflowSpec, node coreworkflow.NodeSpec) int {
	if node.Timeout > 0 {
		return node.Timeout
	}
	return spec.Defaults.Timeout
}

func effectiveWorkingDir(spec coreworkflow.WorkflowSpec, node coreworkflow.NodeSpec) string {
	if node.WorkingDir != "" {
		return node.WorkingDir
	}
	if spec.Defaults.WorkingDir != "" {
		return spec.Defaults.WorkingDir
	}
	return "."
}

func resolvePath(baseDir string, path string) string {
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	if baseDir == "" {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(baseDir, path))
}

func effectiveShell(node coreworkflow.NodeSpec) string {
	if node.Shell != "" {
		return node.Shell
	}
	return "bash"
}

func effectiveAgentSandbox(node coreworkflow.NodeSpec) coreworkflow.SandboxSpec {
	if node.Sandbox.Mode != "" {
		return node.Sandbox
	}
	if node.Permission == nil || node.Permission.Write == nil {
		return node.Sandbox
	}
	if *node.Permission.Write {
		return coreworkflow.SandboxSpec{Mode: "workspace-write"}
	}
	return coreworkflow.SandboxSpec{Mode: "read-only"}
}

func maxOutputBytes(spec coreworkflow.WorkflowSpec) int64 {
	if spec.Execution.MaxNodeOutputBytes > 0 {
		return spec.Execution.MaxNodeOutputBytes
	}
	return 1024 * 1024
}

func isFailure(status corerun.NodeStatus) bool {
	return status == corerun.NodeFailed || status == corerun.NodeTimeout || status == corerun.NodeCancelled
}

func eventForResult(success string, failure string, status corerun.NodeStatus) string {
	if isFailure(status) {
		return failure
	}
	return success
}

func sanitizeName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('-')
	}
	out := strings.Trim(b.String(), "-_")
	if out == "" {
		return "workflow"
	}
	return out
}

func exprSafeKey(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-':
			b.WriteRune('_')
		default:
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "artifact"
	}
	return out
}

func JSONString(value any) string {
	data, _ := json.MarshalIndent(value, "", "  ")
	return string(data)
}

func appendPath(base []string, parts ...string) []string {
	out := make([]string, 0, len(base)+len(parts))
	out = append(out, base...)
	out = append(out, parts...)
	return out
}

func resultValue(result corerun.NodeResult) any {
	if len(result.Outputs) > 0 {
		return result.Outputs
	}
	return result.Output
}

func materializeDeclaredOutputs(node coreworkflow.NodeSpec, rawOutput any) (map[string]any, error) {
	if len(node.Outputs) == 0 {
		return nil, nil
	}
	if len(node.Outputs) == 1 {
		for name, out := range node.Outputs {
			if values, ok := toStringKeyMap(rawOutput); ok {
				value, exists := values[name]
				if !exists {
					return nil, fmt.Errorf("node %q outputs.%s: value is required", node.ID, name)
				}
				if err := validateDeclaredOutputValue(node.ID, name, value, out); err != nil {
					return nil, err
				}
				return map[string]any{name: value}, nil
			}
			if err := validateDeclaredOutputValue(node.ID, name, rawOutput, out); err != nil {
				return nil, err
			}
			return map[string]any{name: rawOutput}, nil
		}
	}
	values, ok := toStringKeyMap(rawOutput)
	if !ok {
		return nil, fmt.Errorf("node %q outputs: expected object result, got %T", node.ID, rawOutput)
	}
	materialized := make(map[string]any, len(node.Outputs))
	for name, out := range node.Outputs {
		value, exists := values[name]
		if !exists {
			return nil, fmt.Errorf("node %q outputs.%s: value is required", node.ID, name)
		}
		if err := validateDeclaredOutputValue(node.ID, name, value, out); err != nil {
			return nil, err
		}
		materialized[name] = value
	}
	return materialized, nil
}

func validateDeclaredOutputValue(nodeID string, name string, value any, spec coreworkflow.NodeOutputSpec) error {
	path := fmt.Sprintf("node %q outputs.%s", nodeID, name)
	if spec.Type != "" {
		if err := coreworkflow.ValidateSchema(value, map[string]any{"type": spec.Type}, path); err != nil {
			return err
		}
	}
	if len(spec.Schema) > 0 {
		if err := coreworkflow.ValidateSchema(value, spec.Schema, path); err != nil {
			return err
		}
	}
	return nil
}

func toStringKeyMap(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	case map[string]string:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = item
		}
		return out, true
	}
	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		return nil, false
	}
	if rv.Kind() == reflect.Map {
		if rv.Type().Key().Kind() != reflect.String {
			return nil, false
		}
		out := make(map[string]any, rv.Len())
		for _, key := range rv.MapKeys() {
			out[key.String()] = rv.MapIndex(key).Interface()
		}
		return out, true
	}
	if rv.Kind() == reflect.Struct {
		out := make(map[string]any, rv.NumField())
		rt := rv.Type()
		for i := 0; i < rv.NumField(); i++ {
			field := rt.Field(i)
			if field.PkgPath != "" {
				continue
			}
			out[field.Name] = rv.Field(i).Interface()
		}
		return out, true
	}
	return nil, false
}

func (e *Executor) forEachItems(ctx context.Context, state *ExecutionState, node coreworkflow.NodeSpec) ([]any, error) {
	if strings.TrimSpace(node.ForEach) == "" {
		return []any{nil}, nil
	}
	value, err := coreworkflow.EvalTemplateValue(node.ForEach, state.evalContext(nil, nil, nil))
	if err != nil {
		return nil, err
	}
	return coreworkflow.ToAnySlice(value)
}
