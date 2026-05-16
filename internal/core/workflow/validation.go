package workflow

import (
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strings"
)

type ProviderLookup interface {
	HasProvider(name string) bool
}

var nodeOutputReference = regexp.MustCompile(`\bnodes\.([A-Za-z_][A-Za-z0-9_-]*)\.(outputs|output)\b`)

func Validate(spec *WorkflowSpec, agentProviders ProviderLookup, worktreeProviders ProviderLookup) error {
	_ = worktreeProviders
	if spec == nil {
		return fmt.Errorf("workflow is nil")
	}
	if spec.Version != "1" {
		return fmt.Errorf("unsupported workflow version %q", spec.Version)
	}
	if strings.TrimSpace(spec.Name) == "" {
		return fmt.Errorf("workflow name is required")
	}
	if len(spec.Nodes) == 0 {
		return fmt.Errorf("workflow must define at least one node")
	}
	if err := validateInputSpecs(spec.Inputs); err != nil {
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

func validateInputSpecs(inputs map[string]InputSpec) error {
	for name, input := range inputs {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("inputs must not contain an empty name")
		}
		if !isSupportedInputType(input.Type) {
			return fmt.Errorf("input %q type must be one of string, integer, number, boolean, array, object", name)
		}
		if input.Default != nil {
			if err := validateInputValue(input.Type, input.Default); err != nil {
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
		if err := validateInputValue(input.Type, value); err != nil {
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

type StaticProviderSet map[string]struct{}

func (s StaticProviderSet) HasProvider(name string) bool {
	_, ok := s[name]
	return ok
}

func DefaultProviders() StaticProviderSet {
	return StaticProviderSet{"codex": {}, "claude": {}, "pi": {}}
}
