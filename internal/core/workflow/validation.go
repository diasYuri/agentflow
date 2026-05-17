package workflow

import (
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/expr-lang/expr"
)

type ProviderLookup interface {
	HasProvider(name string) bool
}

var nodeOutputReference = regexp.MustCompile(`\bnodes\.([A-Za-z_][A-Za-z0-9_-]*)\.(outputs|output)\b`)
var scopeReference = regexp.MustCompile(`(^|[^A-Za-z0-9_.-])([A-Za-z_][A-Za-z0-9_-]*)\.`)
var stringLiteral = regexp.MustCompile(`'[^']*'|"[^"]*"`)

func Validate(spec *WorkflowSpec, agentProviders ProviderLookup, worktreeProviders ProviderLookup) error {
	if spec == nil {
		return fmt.Errorf("workflow is nil")
	}
	if strings.TrimSpace(spec.Version) == "" {
		return fmt.Errorf("workflow version is required")
	}
	switch spec.Version {
	case WorkflowVersion1:
		return validateV1(spec, agentProviders, worktreeProviders)
	case WorkflowVersion2:
		return validateV2(spec, agentProviders, worktreeProviders)
	default:
		return fmt.Errorf("unsupported workflow version %q", spec.Version)
	}
}

func validateV1(spec *WorkflowSpec, agentProviders ProviderLookup, worktreeProviders ProviderLookup) error {
	if err := validateCommon(spec, agentProviders, worktreeProviders); err != nil {
		return err
	}
	// V1-specific: reject V2 fields when detectable, including empty-but-present blocks.
	if spec.Imports != nil {
		return fmt.Errorf("imports are not supported in workflow version %q", spec.Version)
	}
	if spec.Outputs != nil {
		return fmt.Errorf("outputs are not supported in workflow version %q", spec.Version)
	}
	if spec.Hooks != nil {
		return fmt.Errorf("hooks are not supported in workflow version %q", spec.Version)
	}
	if spec.Steps != nil {
		return fmt.Errorf("steps are not supported in workflow version %q", spec.Version)
	}
	if err := validateV1WorkflowScope(spec.Nodes, spec.Version); err != nil {
		return err
	}
	return nil
}

func validateV2(spec *WorkflowSpec, agentProviders ProviderLookup, worktreeProviders ProviderLookup) error {
	if err := validateCommon(spec, agentProviders, worktreeProviders); err != nil {
		return err
	}
	nodeScope := flattenNodeScope(spec.Nodes)
	if err := validateWorkflowOutputs(spec.Outputs, nodeScope); err != nil {
		return err
	}
	if err := validateNodeOutputs(spec.Nodes); err != nil {
		return err
	}
	if err := validateHooks(spec.Hooks); err != nil {
		return err
	}
	if err := validateExpressionsInWorkflow(spec); err != nil {
		return err
	}
	return nil
}

func validateV1WorkflowScope(nodes []NodeSpec, version string) error {
	for _, node := range nodes {
		if node.Ref != "" {
			return fmt.Errorf("node ref is not supported in workflow version %q", version)
		}
		if node.Params != nil {
			return fmt.Errorf("node params are not supported in workflow version %q", version)
		}
		if node.Outputs != nil {
			return fmt.Errorf("node outputs are not supported in workflow version %q", version)
		}
		if len(node.Nodes) > 0 {
			if err := validateV1WorkflowScope(node.Nodes, version); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateHooks(hooks []HookSpec) error {
	for i, hook := range hooks {
		if strings.TrimSpace(hook.Phase) == "" {
			return fmt.Errorf("hooks[%d].phase is required", i)
		}
		if !IsValidHookPhase(hook.Phase) {
			return fmt.Errorf("hooks[%d].phase %q is not valid", i, hook.Phase)
		}
		if strings.TrimSpace(hook.Kind) == "" {
			return fmt.Errorf("hooks[%d].kind is required", i)
		}
		if hook.Kind != "bash" {
			return fmt.Errorf("hooks[%d].kind %q is not supported (only \"bash\" is supported)", i, hook.Kind)
		}
		if strings.TrimSpace(hook.Command) == "" {
			return fmt.Errorf("hooks[%d].command is required", i)
		}
		if hook.Timeout < 0 {
			return fmt.Errorf("hooks[%d].timeout must be >= 0", i)
		}
	}
	return nil
}

func validateCommon(spec *WorkflowSpec, agentProviders ProviderLookup, worktreeProviders ProviderLookup) error {
	_ = worktreeProviders
	if strings.TrimSpace(spec.Name) == "" {
		return fmt.Errorf("workflow name is required")
	}
	if len(spec.Nodes) == 0 {
		return fmt.Errorf("workflow must define at least one node")
	}
	if err := validateInputSpecs(spec.Inputs, spec.Version); err != nil {
		return err
	}
	if err := validateWorktree(spec.Worktree, agentProviders); err != nil {
		return err
	}
	if err := validateWorkflowScope(*spec, agentProviders, nil); err != nil {
		return err
	}
	if _, err := BuildPlan(*spec); err != nil {
		return err
	}
	return nil
}

func validateWorktree(w WorktreeSpec, providers ProviderLookup) error {
	if !w.Enabled {
		return nil
	}
	if providers != nil {
		if !providers.HasProvider(w.Provider) {
			return fmt.Errorf("unknown worktree agent provider %q", w.Provider)
		}
	} else {
		switch w.Provider {
		case "codex", "claude", "pi":
		default:
			return fmt.Errorf("unknown worktree agent provider %q", w.Provider)
		}
	}
	switch w.Base {
	case "current":
	default:
		return fmt.Errorf("unsupported worktree base %q", w.Base)
	}
	switch w.Merge.Strategy {
	case "deterministic":
	default:
		return fmt.Errorf("unsupported worktree merge.strategy %q", w.Merge.Strategy)
	}
	switch w.Merge.OnConflict {
	case "agent":
	default:
		return fmt.Errorf("unsupported worktree merge.on_conflict %q", w.Merge.OnConflict)
	}
	switch w.Cleanup.OnFailure {
	case "keep", "cleanup":
	default:
		return fmt.Errorf("unsupported worktree cleanup.on_failure %q", w.Cleanup.OnFailure)
	}
	return nil
}

func validateWorkflowScope(spec WorkflowSpec, providers ProviderLookup, outer map[string]NodeSpec) error {
	local := map[string]NodeSpec{}
	for i, node := range spec.Nodes {
		if strings.TrimSpace(node.ID) == "" {
			return fmt.Errorf("nodes[%d].id is required", i)
		}
		if _, ok := outer[node.ID]; ok {
			return fmt.Errorf("duplicate node id %q", node.ID)
		}
		if _, ok := local[node.ID]; ok {
			return fmt.Errorf("duplicate node id %q", node.ID)
		}
		local[node.ID] = node
	}
	scope := make(map[string]NodeSpec, len(outer)+len(local))
	for id, node := range outer {
		scope[id] = node
	}
	for id, node := range local {
		scope[id] = node
	}
	for _, node := range spec.Nodes {
		if err := validateNode(&spec, node, providers); err != nil {
			return fmt.Errorf("node %q: %w", node.ID, err)
		}
		if err := validateNodeReferences(node, scope); err != nil {
			return fmt.Errorf("node %q: %w", node.ID, err)
		}
	}
	for _, node := range spec.Nodes {
		for _, dep := range node.DependsOn {
			if _, ok := scope[dep]; !ok {
				return fmt.Errorf("node %q depends on unknown node %q", node.ID, dep)
			}
		}
		if node.GoToIf != nil {
			if strings.TrimSpace(node.GoToIf.When) == "" {
				return fmt.Errorf("node %q go_to_if.when is required", node.ID)
			}
			if strings.TrimSpace(node.GoToIf.Target) == "" {
				return fmt.Errorf("node %q go_to_if.target is required", node.ID)
			}
			if _, ok := local[node.GoToIf.Target]; !ok {
				return fmt.Errorf("node %q go_to_if.target references unknown node %q", node.ID, node.GoToIf.Target)
			}
		}
		if len(node.Nodes) > 0 {
			childSpec := WorkflowSpec{
				Version:   spec.Version,
				Name:      spec.Name + "/" + node.ID,
				Inputs:    spec.Inputs,
				Vars:      spec.Vars,
				Secrets:   spec.Secrets,
				Defaults:  spec.Defaults,
				Execution: spec.Execution,
				Nodes:     node.Nodes,
			}
			if err := validateWorkflowScope(childSpec, providers, scope); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateInputSpecs(inputs map[string]InputSpec, version string) error {
	for name, input := range inputs {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("inputs must not contain an empty name")
		}
		if version == WorkflowVersion1 && len(input.Schema) > 0 {
			return fmt.Errorf("inputs.%s.schema is not supported in workflow version %q", name, version)
		}
		if len(input.Schema) > 0 {
			if err := ValidateSchemaDecl(input.Schema, "inputs."+name+".schema"); err != nil {
				return err
			}
			if input.Type != "" {
				if schemaType, ok := input.Schema["type"].(string); ok && schemaType != input.Type {
					return fmt.Errorf("input %q: type %q conflicts with schema.type %q", name, input.Type, schemaType)
				}
			}
		} else {
			if !isSupportedInputType(input.Type) {
				return fmt.Errorf("input %q type must be one of string, integer, number, boolean, array, object", name)
			}
		}
		if input.Default != nil {
			if err := CoerceAndValidateInputValue(input.Default, input.Type, input.Schema, name); err != nil {
				return fmt.Errorf("input %q default: %w", name, err)
			}
		}
	}
	return nil
}

func ValidateInputValues(spec WorkflowSpec, provided map[string]any) error {
	for name, value := range provided {
		input, ok := spec.Inputs[name]
		if !ok || value == nil {
			continue
		}
		if err := CoerceAndValidateInputValue(value, input.Type, input.Schema, name); err != nil {
			return fmt.Errorf("input %q: %w", name, err)
		}
	}
	return nil
}

func isSupportedInputType(inputType string) bool {
	switch inputType {
	case "string", "integer", "number", "boolean", "array", "object":
		return true
	default:
		return false
	}
}

func validateInputValue(inputType string, value any) error {
	if value == nil {
		return nil
	}
	if inputType == "" {
		return fmt.Errorf("type is required")
	}
	switch inputType {
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("got %T, want string", value)
		}
	case "integer":
		if !isInteger(value) {
			return fmt.Errorf("got %T, want integer", value)
		}
	case "number":
		if !isNumber(value) {
			return fmt.Errorf("got %T, want number", value)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("got %T, want boolean", value)
		}
	case "array":
		if !hasKind(value, reflect.Array, reflect.Slice) {
			return fmt.Errorf("got %T, want array", value)
		}
	case "object":
		if !hasKind(value, reflect.Map, reflect.Struct) {
			return fmt.Errorf("got %T, want object", value)
		}
	default:
		return fmt.Errorf("unsupported type %q", inputType)
	}
	return nil
}

func isInteger(value any) bool {
	switch typed := value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	case float32:
		return math.Trunc(float64(typed)) == float64(typed)
	case float64:
		return math.Trunc(typed) == typed
	default:
		rv := reflect.ValueOf(value)
		if !rv.IsValid() {
			return false
		}
		switch rv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return true
		case reflect.Float32, reflect.Float64:
			return math.Trunc(rv.Float()) == rv.Float()
		default:
			return false
		}
	}
}

func isNumber(value any) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return true
	default:
		rv := reflect.ValueOf(value)
		if !rv.IsValid() {
			return false
		}
		switch rv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Float32, reflect.Float64:
			return true
		default:
			return false
		}
	}
}

func hasKind(value any, kinds ...reflect.Kind) bool {
	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		return false
	}
	for _, kind := range kinds {
		if rv.Kind() == kind {
			return true
		}
	}
	return false
}

func validateNode(spec *WorkflowSpec, node NodeSpec, providers ProviderLookup) error {
	switch node.Kind {
	case NodeKindAgent:
		if strings.TrimSpace(node.Prompt) == "" {
			return fmt.Errorf("agent prompt is required")
		}
		if err := validateAgentPermission(node); err != nil {
			return err
		}
		provider := node.Provider
		if provider == "" {
			provider = "codex"
		}
		if providers != nil && !providers.HasProvider(provider) {
			return fmt.Errorf("unknown agent provider %q", provider)
		}
	case NodeKindBash:
		if strings.TrimSpace(node.Command) == "" {
			return fmt.Errorf("bash command is required")
		}
	case NodeKindTransform:
		if strings.TrimSpace(node.Operation) == "" {
			return fmt.Errorf("transform operation is required")
		}
	case NodeKindNoop:
	case NodeKindMap:
		if len(node.Nodes) == 0 {
			return fmt.Errorf("map nodes must define nested nodes")
		}
	default:
		return fmt.Errorf("unknown kind %q", node.Kind)
	}
	if err := validateArtifacts(node.Artifacts); err != nil {
		return err
	}
	if node.Kind != NodeKindAgent && node.Permission != nil {
		return fmt.Errorf("permission is only supported for agent nodes")
	}
	if node.Concurrency < 0 {
		return fmt.Errorf("concurrency must be greater than zero")
	}
	if node.ForEach != "" && node.Concurrency == 0 {
		// Filled by execution defaults; validation only rejects explicit negatives.
	}
	if node.MaxItems < 0 {
		return fmt.Errorf("max_items must be greater than zero")
	}
	if node.Retries < 0 {
		return fmt.Errorf("retries must be >= 0")
	}
	if node.Timeout < 0 {
		return fmt.Errorf("timeout must be >= 0")
	}
	return nil
}

func validateArtifacts(artifacts []ArtifactSpec) error {
	seen := map[string]int{}
	for i, art := range artifacts {
		if strings.TrimSpace(art.Name) == "" {
			return fmt.Errorf("artifacts[%d].name is required", i)
		}
		if strings.TrimSpace(art.Path) == "" {
			return fmt.Errorf("artifacts[%d].path is required", i)
		}
		if filepath.IsAbs(art.Path) {
			return fmt.Errorf("artifacts[%d].path must be relative", i)
		}
		clean := filepath.Clean(art.Path)
		if strings.Contains(clean, "..") {
			return fmt.Errorf("artifacts[%d].path must not contain ..", i)
		}
		key := normalizeArtifactKey(art.Name)
		if isReservedArtifactKey(key) {
			return fmt.Errorf("artifacts[%d].name %q is reserved", i, art.Name)
		}
		if prev, ok := seen[key]; ok {
			return fmt.Errorf("artifacts[%d].name %q collides with artifacts[%d].name %q after normalization", i, art.Name, prev, artifacts[prev].Name)
		}
		seen[key] = i
	}
	return nil
}

func normalizeArtifactKey(name string) string {
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
	return strings.Trim(b.String(), "_")
}

func isReservedArtifactKey(key string) bool {
	switch key {
	case "stdout", "stderr", "result", "stdout_txt", "stderr_txt", "result_json":
		return true
	default:
		return false
	}
}

func validateAgentPermission(node NodeSpec) error {
	if node.Permission == nil {
		return nil
	}
	if node.Permission.Write == nil {
		return fmt.Errorf("permission.write is required when permission is set")
	}
	return nil
}

func validateNodeReferences(node NodeSpec, nodes map[string]NodeSpec) error {
	check := func(field string, value string) error {
		if strings.TrimSpace(value) == "" {
			return nil
		}
		return validateStaticNodeOutputReferences(field, value, nodes)
	}
	if err := check("when", node.When); err != nil {
		return err
	}
	if node.GoToIf != nil {
		if err := check("go_to_if.when", node.GoToIf.When); err != nil {
			return err
		}
	}
	if err := check("for_each", node.ForEach); err != nil {
		return err
	}
	if err := check("prompt", node.Prompt); err != nil {
		return err
	}
	if err := check("system", node.System); err != nil {
		return err
	}
	if err := check("command", node.Command); err != nil {
		return err
	}
	if err := check("working_dir", node.WorkingDir); err != nil {
		return err
	}
	if err := check("input", node.Input); err != nil {
		return err
	}
	for key, value := range node.Env {
		if err := check("env."+key, value); err != nil {
			return err
		}
	}
	return nil
}

func validateStaticNodeOutputReferences(field string, value string, nodes map[string]NodeSpec) error {
	for _, match := range nodeOutputReference.FindAllStringSubmatch(value, -1) {
		id := match[1]
		accessor := match[2]
		referenced, ok := nodes[id]
		if !ok {
			return fmt.Errorf("%s: unknown node reference %q", field, id)
		}
		if referenced.ForEach != "" && accessor == "output" {
			return fmt.Errorf("%s: nodes.%s.output is invalid because node %q is expanded; use nodes.%s.outputs", field, id, id, id)
		}
		if referenced.ForEach == "" && accessor == "outputs" {
			return fmt.Errorf("%s: nodes.%s.outputs is invalid because node %q is not expanded; use nodes.%s.output", field, id, id, id)
		}
	}
	return nil
}

func validateAllowedOutputScopeRoots(field string, value string) error {
	sanitized := nodeOutputReference.ReplaceAllString(value, "nodes.")
	sanitized = stringLiteral.ReplaceAllString(sanitized, "")
	for _, match := range scopeReference.FindAllStringSubmatch(sanitized, -1) {
		switch match[2] {
		case "inputs", "vars", "secrets", "nodes", "run":
		default:
			return fmt.Errorf("%s: invalid reference %q", field, match[2])
		}
	}
	return nil
}

func validateWorkflowOutputs(outputs map[string]OutputSpec, nodes map[string]NodeSpec) error {
	for name, out := range outputs {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("outputs must not contain an empty name")
		}
		if out.Value == nil {
			return fmt.Errorf("outputs.%s: value is required", name)
		}
		if out.Type != "" && !isSupportedInputType(out.Type) {
			return fmt.Errorf("outputs.%s: type must be one of string, integer, number, boolean, array, object", name)
		}
		if len(out.Schema) > 0 {
			if err := ValidateSchemaDecl(out.Schema, "outputs."+name+".schema"); err != nil {
				return err
			}
			if out.Type != "" {
				if schemaType, ok := out.Schema["type"].(string); ok && schemaType != out.Type {
					return fmt.Errorf("outputs.%s: type %q conflicts with schema.type %q", name, out.Type, schemaType)
				}
			}
		}
		valueStr, ok := out.Value.(string)
		if ok && strings.Contains(valueStr, "${") {
			if err := validateAllowedOutputScopeRoots("outputs."+name, valueStr); err != nil {
				return err
			}
			if err := validateStaticNodeOutputReferences("outputs."+name, valueStr, nodes); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateNodeOutputs(nodes []NodeSpec) error {
	for _, node := range nodes {
		for name, out := range node.Outputs {
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("node %q outputs must not contain an empty name", node.ID)
			}
			if out.Type != "" && !isSupportedInputType(out.Type) {
				return fmt.Errorf("node %q outputs.%s: type must be one of string, integer, number, boolean, array, object", node.ID, name)
			}
			if len(out.Schema) > 0 {
				if err := ValidateSchemaDecl(out.Schema, "nodes."+node.ID+".outputs."+name+".schema"); err != nil {
					return err
				}
				if out.Type != "" {
					if schemaType, ok := out.Schema["type"].(string); ok && schemaType != out.Type {
						return fmt.Errorf("node %q outputs.%s: type %q conflicts with schema.type %q", node.ID, name, out.Type, schemaType)
					}
				}
			}
		}
		if len(node.Nodes) > 0 {
			if err := validateNodeOutputs(node.Nodes); err != nil {
				return err
			}
		}
	}
	return nil
}

type StaticProviderSet map[string]struct{}

func (s StaticProviderSet) HasProvider(name string) bool {
	_, ok := s[name]
	return ok
}

func validateExpressionsInWorkflow(spec *WorkflowSpec) error {
	// Minimal environment for static syntax checking.
	minCtx := EvalContext{
		Inputs:  map[string]any{},
		Vars:    map[string]any{},
		Secrets: map[string]any{},
		Nodes:   map[string]any{},
		Run:     map[string]any{},
	}
	minEnv := env(minCtx)
	checkTemplate := func(field string, exprStr string) error {
		if strings.TrimSpace(exprStr) == "" {
			return nil
		}
		// Extract template expressions like ${...}
		for _, match := range templateExpr.FindAllStringSubmatch(exprStr, -1) {
			inner := strings.TrimSpace(match[1])
			if _, err := expr.Compile(inner, expr.Env(minEnv), expr.AllowUndefinedVariables()); err != nil {
				return fmt.Errorf("%s: expression %q compile error: %w", field, inner, err)
			}
		}
		return nil
	}
	checkRaw := func(field string, exprStr string) error {
		if strings.TrimSpace(exprStr) == "" {
			return nil
		}
		normalized := templateExpr.ReplaceAllString(exprStr, `($1)`)
		if _, err := expr.Compile(normalized, expr.Env(minEnv), expr.AllowUndefinedVariables()); err != nil {
			return fmt.Errorf("%s: expression compile error: %w", field, err)
		}
		return nil
	}
	nodeScope := flattenNodeScope(spec.Nodes)
	for _, node := range spec.Nodes {
		if err := validateExpressionsInNode(node, checkTemplate, checkRaw); err != nil {
			return err
		}
	}
	for name, out := range spec.Outputs {
		if s, ok := out.Value.(string); ok {
			if err := checkTemplate("outputs."+name+".value", s); err != nil {
				return err
			}
			if err := validateStaticNodeOutputReferences("outputs."+name, s, nodeScope); err != nil {
				return err
			}
		}
	}
	for i, hook := range spec.Hooks {
		if err := checkTemplate(fmt.Sprintf("hooks[%d].command", i), hook.Command); err != nil {
			return err
		}
	}
	return nil
}

func validateExpressionsInNode(node NodeSpec, checkTemplate func(string, string) error, checkRaw func(string, string) error) error {
	prefix := "node " + node.ID
	fields := []struct {
		name  string
		value string
		raw   bool
	}{
		{prefix + ".when", node.When, true},
		{prefix + ".for_each", node.ForEach, true},
		{prefix + ".prompt", node.Prompt, false},
		{prefix + ".system", node.System, false},
		{prefix + ".command", node.Command, false},
		{prefix + ".working_dir", node.WorkingDir, false},
		{prefix + ".input", node.Input, false},
	}
	for _, f := range fields {
		checker := checkTemplate
		if f.raw {
			checker = checkRaw
		}
		if err := checker(f.name, f.value); err != nil {
			return err
		}
	}
	if node.GoToIf != nil {
		if err := checkRaw(prefix+".go_to_if.when", node.GoToIf.When); err != nil {
			return err
		}
	}
	for k, v := range node.Env {
		if err := checkTemplate(prefix+".env."+k, v); err != nil {
			return err
		}
	}
	for _, child := range node.Nodes {
		if err := validateExpressionsInNode(child, checkTemplate, checkRaw); err != nil {
			return err
		}
	}
	return nil
}

func flattenNodeScope(nodes []NodeSpec) map[string]NodeSpec {
	scope := make(map[string]NodeSpec)
	var walk func([]NodeSpec)
	walk = func(items []NodeSpec) {
		for _, node := range items {
			if node.ID != "" {
				scope[node.ID] = node
			}
			if len(node.Nodes) > 0 {
				walk(node.Nodes)
			}
		}
	}
	walk(nodes)
	return scope
}

func DefaultProviders() StaticProviderSet {
	return StaticProviderSet{"codex": {}, "claude": {}, "pi": {}}
}
