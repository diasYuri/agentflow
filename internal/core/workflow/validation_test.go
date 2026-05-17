package workflow

import (
	"strings"
	"testing"
)

func ptr[T any](v T) *T { return &v }

func TestValidateRejectsInvalidInputDefaultType(t *testing.T) {
	spec := &WorkflowSpec{
		Version: "1",
		Name:    "inputs",
		Inputs: map[string]InputSpec{
			"retries": {Type: "integer", Default: "three"},
		},
		Nodes: []NodeSpec{{ID: "start", Kind: NodeKindNoop}},
	}

	err := Validate(spec, DefaultProviders(), nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), `input "retries" default`) {
		t.Fatalf("expected input default context, got %v", err)
	}
}

func TestValidateInputValuesRejectsInvalidProvidedType(t *testing.T) {
	spec := WorkflowSpec{
		Inputs: map[string]InputSpec{
			"enabled": {Type: "boolean"},
		},
	}

	err := ValidateInputValues(spec, map[string]any{"enabled": "true"})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), `input "enabled"`) {
		t.Fatalf("expected input context, got %v", err)
	}
}

func TestValidateRejectsExpandedNodeOutputReference(t *testing.T) {
	spec := &WorkflowSpec{
		Version: "1",
		Name:    "refs",
		Inputs: map[string]InputSpec{
			"items": {Type: "array", Default: []any{"a"}},
		},
		Nodes: []NodeSpec{
			{ID: "split", Kind: NodeKindNoop, ForEach: "${inputs.items}"},
			{ID: "use", Kind: NodeKindNoop, When: "len(nodes.split.output) > 0"},
		},
	}

	err := Validate(spec, DefaultProviders(), nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "when: nodes.split.output is invalid") {
		t.Fatalf("expected field-specific output reference error, got %v", err)
	}
}

func TestValidateRejectsNonExpandedNodeOutputsReference(t *testing.T) {
	spec := &WorkflowSpec{
		Version: "1",
		Name:    "refs",
		Nodes: []NodeSpec{
			{ID: "plan", Kind: NodeKindNoop},
			{ID: "use", Kind: NodeKindBash, Command: "echo ${nodes.plan.outputs}"},
		},
	}

	err := Validate(spec, DefaultProviders(), nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "command: nodes.plan.outputs is invalid") {
		t.Fatalf("expected field-specific outputs reference error, got %v", err)
	}
}

func TestValidateAllowsAgentPermissionWrite(t *testing.T) {
	write := true
	spec := &WorkflowSpec{
		Version: "1",
		Name:    "permissions",
		Nodes: []NodeSpec{
			{ID: "implement", Kind: NodeKindAgent, Prompt: "do it", Permission: &PermissionSpec{Write: &write}},
		},
	}

	if err := Validate(spec, DefaultProviders(), nil); err != nil {
		t.Fatalf("expected permission to validate, got %v", err)
	}
}

func TestValidateAllowsClaudeAgentProvider(t *testing.T) {
	spec := &WorkflowSpec{
		Version: "1",
		Name:    "claude-provider",
		Nodes: []NodeSpec{
			{ID: "implement", Kind: NodeKindAgent, Provider: "claude", Prompt: "do it"},
		},
	}

	if err := Validate(spec, DefaultProviders(), nil); err != nil {
		t.Fatalf("expected claude provider to validate, got %v", err)
	}
}

func TestValidateAllowsPiAgentProvider(t *testing.T) {
	spec := &WorkflowSpec{
		Version: "1",
		Name:    "pi-provider",
		Nodes: []NodeSpec{
			{ID: "implement", Kind: NodeKindAgent, Provider: "pi", Prompt: "do it"},
		},
	}

	if err := Validate(spec, DefaultProviders(), nil); err != nil {
		t.Fatalf("expected pi provider to validate, got %v", err)
	}
}

func TestValidateRejectsPermissionOnNonAgentNode(t *testing.T) {
	write := true
	spec := &WorkflowSpec{
		Version: "1",
		Name:    "permissions",
		Nodes: []NodeSpec{
			{ID: "test", Kind: NodeKindBash, Command: "echo ok", Permission: &PermissionSpec{Write: &write}},
		},
	}

	err := Validate(spec, DefaultProviders(), nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "permission is only supported for agent nodes") {
		t.Fatalf("expected permission scope error, got %v", err)
	}
}

func TestValidateRejectsIncompletePermissionBlock(t *testing.T) {
	spec := &WorkflowSpec{
		Version: "1",
		Name:    "permissions",
		Nodes: []NodeSpec{
			{ID: "implement", Kind: NodeKindAgent, Prompt: "do it", Permission: &PermissionSpec{}},
		},
	}

	err := Validate(spec, DefaultProviders(), nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "permission.write is required") {
		t.Fatalf("expected permission write error, got %v", err)
	}
}

func TestValidateAllowsNestedMapChildReferences(t *testing.T) {
	spec := &WorkflowSpec{
		Version: "1",
		Name:    "nested",
		Inputs: map[string]InputSpec{
			"items": {Type: "array", Default: []any{"a"}},
		},
		Nodes: []NodeSpec{
			{ID: "outer", Kind: NodeKindNoop},
			{
				ID:      "group",
				Kind:    NodeKindMap,
				ForEach: "${inputs.items}",
				Nodes: []NodeSpec{
					{ID: "draft", Kind: NodeKindBash, DependsOn: []string{"outer"}, Command: "echo ${nodes.outer.output.status}"},
				},
			},
		},
	}

	if err := Validate(spec, DefaultProviders(), nil); err != nil {
		t.Fatalf("expected nested map validation to succeed, got %v", err)
	}
}

func TestValidateRejectsForwardGoToIfTarget(t *testing.T) {
	spec := &WorkflowSpec{
		Version: "1",
		Name:    "loop",
		Nodes: []NodeSpec{
			{
				ID:   "check",
				Kind: NodeKindNoop,
				GoToIf: &GoToIfSpec{
					When:   "true",
					Target: "done",
				},
			},
			{ID: "done", Kind: NodeKindNoop},
		},
	}

	err := Validate(spec, DefaultProviders(), nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "go_to_if.target must point to the current node or an earlier node") {
		t.Fatalf("expected forward jump validation error, got %v", err)
	}
}

func TestValidateRejectsUnknownWorktreeProvider(t *testing.T) {
	spec := &WorkflowSpec{
		Version: "1",
		Name:    "worktree-provider",
		Nodes:   []NodeSpec{{ID: "ok", Kind: NodeKindNoop}},
		Worktree: WorktreeSpec{
			Enabled:  true,
			Provider: "unknown",
			Base:     "current",
			Merge:    WorktreeMergeSpec{Strategy: "deterministic", OnConflict: "agent"},
			Cleanup:  WorktreeCleanupSpec{OnSuccess: ptr(true), OnFailure: "keep"},
		},
	}

	err := Validate(spec, DefaultProviders(), nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "unknown worktree agent provider") {
		t.Fatalf("expected worktree provider error, got %v", err)
	}
}

func TestValidateAllowsWorktreeAgentProviders(t *testing.T) {
	for _, provider := range []string{"pi", "codex", "claude"} {
		t.Run(provider, func(t *testing.T) {
			spec := &WorkflowSpec{
				Version: "1",
				Name:    "worktree-" + provider,
				Nodes:   []NodeSpec{{ID: "ok", Kind: NodeKindNoop}},
				Worktree: WorktreeSpec{
					Enabled:  true,
					Provider: provider,
					Base:     "current",
					Merge:    WorktreeMergeSpec{Strategy: "deterministic", OnConflict: "agent"},
					Cleanup:  WorktreeCleanupSpec{OnSuccess: ptr(true), OnFailure: "keep"},
				},
			}

			if err := Validate(spec, DefaultProviders(), nil); err != nil {
				t.Fatalf("expected worktree provider %q to validate, got %v", provider, err)
			}
		})
	}
}

func TestValidateRejectsUnknownWorktreeBase(t *testing.T) {
	spec := &WorkflowSpec{
		Version: "1",
		Name:    "worktree-base",
		Nodes:   []NodeSpec{{ID: "ok", Kind: NodeKindNoop}},
		Worktree: WorktreeSpec{
			Enabled:  true,
			Provider: "pi",
			Base:     "other",
			Merge:    WorktreeMergeSpec{Strategy: "deterministic", OnConflict: "agent"},
			Cleanup:  WorktreeCleanupSpec{OnSuccess: ptr(true), OnFailure: "keep"},
		},
	}

	err := Validate(spec, DefaultProviders(), nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "unsupported worktree base") {
		t.Fatalf("expected worktree base error, got %v", err)
	}
}

func TestValidateRejectsUnknownWorktreeMergeStrategy(t *testing.T) {
	spec := &WorkflowSpec{
		Version: "1",
		Name:    "worktree-merge",
		Nodes:   []NodeSpec{{ID: "ok", Kind: NodeKindNoop}},
		Worktree: WorktreeSpec{
			Enabled:  true,
			Provider: "pi",
			Base:     "current",
			Merge:    WorktreeMergeSpec{Strategy: "auto", OnConflict: "agent"},
			Cleanup:  WorktreeCleanupSpec{OnSuccess: ptr(true), OnFailure: "keep"},
		},
	}

	err := Validate(spec, DefaultProviders(), nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "unsupported worktree merge.strategy") {
		t.Fatalf("expected worktree merge strategy error, got %v", err)
	}
}

func TestValidateRejectsUnknownWorktreeOnConflict(t *testing.T) {
	spec := &WorkflowSpec{
		Version: "1",
		Name:    "worktree-conflict",
		Nodes:   []NodeSpec{{ID: "ok", Kind: NodeKindNoop}},
		Worktree: WorktreeSpec{
			Enabled:  true,
			Provider: "pi",
			Base:     "current",
			Merge:    WorktreeMergeSpec{Strategy: "deterministic", OnConflict: "abort"},
			Cleanup:  WorktreeCleanupSpec{OnSuccess: ptr(true), OnFailure: "keep"},
		},
	}

	err := Validate(spec, DefaultProviders(), nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "unsupported worktree merge.on_conflict") {
		t.Fatalf("expected worktree on_conflict error, got %v", err)
	}
}

func TestValidateRejectsUnknownVersion(t *testing.T) {
	spec := &WorkflowSpec{
		Version: "99",
		Name:    "unknown",
		Nodes:   []NodeSpec{{ID: "ok", Kind: NodeKindNoop}},
	}

	err := Validate(spec, DefaultProviders(), nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), `unsupported workflow version "99"`) {
		t.Fatalf("expected unsupported version error, got %v", err)
	}
}

func TestValidateRejectsMissingVersion(t *testing.T) {
	spec := &WorkflowSpec{
		Version: "",
		Name:    "missing-version",
		Nodes:   []NodeSpec{{ID: "ok", Kind: NodeKindNoop}},
	}

	err := Validate(spec, DefaultProviders(), nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "workflow version is required") {
		t.Fatalf("expected version required error, got %v", err)
	}
}

func TestValidateRejectsV2FieldsInV1(t *testing.T) {
	t.Run("imports", func(t *testing.T) {
		spec := &WorkflowSpec{
			Version: WorkflowVersion1,
			Name:    "v1-imports",
			Nodes:   []NodeSpec{{ID: "ok", Kind: NodeKindNoop}},
			Imports: []ImportSpec{{Path: "other.yaml"}},
		}
		err := Validate(spec, DefaultProviders(), nil)
		if err == nil {
			t.Fatal("expected validation error")
		}
		if !strings.Contains(err.Error(), `imports are not supported in workflow version "1"`) {
			t.Fatalf("expected imports error, got %v", err)
		}
	})

	t.Run("outputs", func(t *testing.T) {
		spec := &WorkflowSpec{
			Version: WorkflowVersion1,
			Name:    "v1-outputs",
			Nodes:   []NodeSpec{{ID: "ok", Kind: NodeKindNoop}},
			Outputs: map[string]OutputSpec{"result": {Value: "ok"}},
		}
		err := Validate(spec, DefaultProviders(), nil)
		if err == nil {
			t.Fatal("expected validation error")
		}
		if !strings.Contains(err.Error(), `outputs are not supported in workflow version "1"`) {
			t.Fatalf("expected outputs error, got %v", err)
		}
	})

	t.Run("hooks", func(t *testing.T) {
		spec := &WorkflowSpec{
			Version: WorkflowVersion1,
			Name:    "v1-hooks",
			Nodes:   []NodeSpec{{ID: "ok", Kind: NodeKindNoop}},
			Hooks:   []HookSpec{{Kind: "pre"}},
		}
		err := Validate(spec, DefaultProviders(), nil)
		if err == nil {
			t.Fatal("expected validation error")
		}
		if !strings.Contains(err.Error(), `hooks are not supported in workflow version "1"`) {
			t.Fatalf("expected hooks error, got %v", err)
		}
	})

	t.Run("steps", func(t *testing.T) {
		spec := &WorkflowSpec{
			Version: WorkflowVersion1,
			Name:    "v1-steps",
			Nodes:   []NodeSpec{{ID: "ok", Kind: NodeKindNoop}},
			Steps:   map[string]ReusableStepSpec{"myStep": {}},
		}
		err := Validate(spec, DefaultProviders(), nil)
		if err == nil {
			t.Fatal("expected validation error")
		}
		if !strings.Contains(err.Error(), `steps are not supported in workflow version "1"`) {
			t.Fatalf("expected steps error, got %v", err)
		}
	})

	t.Run("input schema", func(t *testing.T) {
		spec := &WorkflowSpec{
			Version: WorkflowVersion1,
			Name:    "v1-input-schema",
			Inputs: map[string]InputSpec{
				"config": {Schema: map[string]any{"type": "object"}},
			},
			Nodes: []NodeSpec{{ID: "ok", Kind: NodeKindNoop}},
		}
		err := Validate(spec, DefaultProviders(), nil)
		if err == nil {
			t.Fatal("expected validation error")
		}
		if !strings.Contains(err.Error(), `inputs.config.schema is not supported in workflow version "1"`) {
			t.Fatalf("expected input schema error, got %v", err)
		}
	})

	t.Run("node ref", func(t *testing.T) {
		spec := &WorkflowSpec{
			Version: WorkflowVersion1,
			Name:    "v1-ref",
			Nodes:   []NodeSpec{{ID: "ok", Kind: NodeKindNoop, Ref: "other"}},
		}
		err := Validate(spec, DefaultProviders(), nil)
		if err == nil {
			t.Fatal("expected validation error")
		}
		if !strings.Contains(err.Error(), `node ref is not supported in workflow version "1"`) {
			t.Fatalf("expected node ref error, got %v", err)
		}
	})

	t.Run("node params", func(t *testing.T) {
		spec := &WorkflowSpec{
			Version: WorkflowVersion1,
			Name:    "v1-params",
			Nodes:   []NodeSpec{{ID: "ok", Kind: NodeKindNoop, Params: map[string]any{"name": "value"}}},
		}
		err := Validate(spec, DefaultProviders(), nil)
		if err == nil {
			t.Fatal("expected validation error")
		}
		if !strings.Contains(err.Error(), `node params are not supported in workflow version "1"`) {
			t.Fatalf("expected node params error, got %v", err)
		}
	})

	t.Run("node outputs", func(t *testing.T) {
		spec := &WorkflowSpec{
			Version: WorkflowVersion1,
			Name:    "v1-node-outputs",
			Nodes:   []NodeSpec{{ID: "ok", Kind: NodeKindNoop, Outputs: map[string]NodeOutputSpec{"result": {Type: "string"}}}},
		}
		err := Validate(spec, DefaultProviders(), nil)
		if err == nil {
			t.Fatal("expected validation error")
		}
		if !strings.Contains(err.Error(), `node outputs are not supported in workflow version "1"`) {
			t.Fatalf("expected node outputs error, got %v", err)
		}
	})
}

func TestValidateAcceptsV2Minimum(t *testing.T) {
	spec := &WorkflowSpec{
		Version: WorkflowVersion2,
		Name:    "v2-min",
		Nodes:   []NodeSpec{{ID: "ok", Kind: NodeKindNoop}},
	}

	if err := Validate(spec, DefaultProviders(), nil); err != nil {
		t.Fatalf("expected v2 minimum to validate, got %v", err)
	}
}

func TestValidateRejectsUnknownWorktreeCleanupOnFailure(t *testing.T) {
	spec := &WorkflowSpec{
		Version: "1",
		Name:    "worktree-cleanup",
		Nodes:   []NodeSpec{{ID: "ok", Kind: NodeKindNoop}},
		Worktree: WorktreeSpec{
			Enabled:  true,
			Provider: "pi",
			Base:     "current",
			Merge:    WorktreeMergeSpec{Strategy: "deterministic", OnConflict: "agent"},
			Cleanup:  WorktreeCleanupSpec{OnSuccess: ptr(true), OnFailure: "delete"},
		},
	}

	err := Validate(spec, DefaultProviders(), nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "unsupported worktree cleanup.on_failure") {
		t.Fatalf("expected worktree cleanup.on_failure error, got %v", err)
	}
}

func TestValidateAllowsValidWorktree(t *testing.T) {
	spec := &WorkflowSpec{
		Version: "1",
		Name:    "worktree-valid",
		Nodes:   []NodeSpec{{ID: "ok", Kind: NodeKindNoop}},
		Worktree: WorktreeSpec{
			Enabled:  true,
			Provider: "pi",
			Base:     "current",
			Merge:    WorktreeMergeSpec{Strategy: "deterministic", OnConflict: "agent"},
			Cleanup:  WorktreeCleanupSpec{OnSuccess: ptr(true), OnFailure: "keep"},
		},
	}

	if err := Validate(spec, DefaultProviders(), nil); err != nil {
		t.Fatalf("expected valid worktree, got %v", err)
	}
}

func TestValidateRejectsInvalidSchema(t *testing.T) {
	spec := &WorkflowSpec{
		Version: WorkflowVersion2,
		Name:    "bad-schema",
		Inputs: map[string]InputSpec{
			"config": {
				Schema: map[string]any{"type": "unknown"},
			},
		},
		Nodes: []NodeSpec{{ID: "ok", Kind: NodeKindNoop}},
	}

	err := Validate(spec, DefaultProviders(), nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "unsupported schema type") {
		t.Fatalf("expected schema type error, got %v", err)
	}
}

func TestValidateRejectsInputDefaultBySchema(t *testing.T) {
	spec := &WorkflowSpec{
		Version: WorkflowVersion2,
		Name:    "schema-default",
		Inputs: map[string]InputSpec{
			"config": {
				Schema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"enabled": map[string]any{"type": "boolean"},
					},
				},
				Default: map[string]any{"enabled": "yes"},
			},
		},
		Nodes: []NodeSpec{{ID: "ok", Kind: NodeKindNoop}},
	}

	err := Validate(spec, DefaultProviders(), nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), `input "config" default`) {
		t.Fatalf("expected input default context, got %v", err)
	}
}

func TestValidateInputValuesRejectsInvalidBySchema(t *testing.T) {
	spec := WorkflowSpec{
		Inputs: map[string]InputSpec{
			"config": {
				Schema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"count": map[string]any{"type": "integer"},
					},
				},
			},
		},
	}

	err := ValidateInputValues(spec, map[string]any{"config": map[string]any{"count": "ten"}})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), `input "config"`) {
		t.Fatalf("expected input context, got %v", err)
	}
}

func TestValidateRejectsWorkflowOutputMissingValue(t *testing.T) {
	spec := &WorkflowSpec{
		Version: WorkflowVersion2,
		Name:    "missing-value",
		Outputs: map[string]OutputSpec{
			"result": {Type: "string"},
		},
		Nodes: []NodeSpec{{ID: "ok", Kind: NodeKindNoop}},
	}

	err := Validate(spec, DefaultProviders(), nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "outputs.result: value is required") {
		t.Fatalf("expected missing value error, got %v", err)
	}
}

func TestValidateRejectsWorkflowOutputInvalidType(t *testing.T) {
	spec := &WorkflowSpec{
		Version: WorkflowVersion2,
		Name:    "bad-output-type",
		Outputs: map[string]OutputSpec{
			"result": {Value: "ok", Type: "unknown"},
		},
		Nodes: []NodeSpec{{ID: "ok", Kind: NodeKindNoop}},
	}

	err := Validate(spec, DefaultProviders(), nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "outputs.result: type must be one of") {
		t.Fatalf("expected output type error, got %v", err)
	}
}

func TestValidateRejectsWorkflowOutputTypeSchemaConflict(t *testing.T) {
	spec := &WorkflowSpec{
		Version: WorkflowVersion2,
		Name:    "conflict",
		Outputs: map[string]OutputSpec{
			"result": {
				Value:  "ok",
				Type:   "string",
				Schema: map[string]any{"type": "integer"},
			},
		},
		Nodes: []NodeSpec{{ID: "ok", Kind: NodeKindNoop}},
	}

	err := Validate(spec, DefaultProviders(), nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "conflicts with schema.type") {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestValidateRejectsWorkflowOutputInvalidReference(t *testing.T) {
	spec := &WorkflowSpec{
		Version: WorkflowVersion2,
		Name:    "bad-ref",
		Outputs: map[string]OutputSpec{
			"result": {Value: "${unknown.value}"},
		},
		Nodes: []NodeSpec{{ID: "ok", Kind: NodeKindNoop}},
	}

	err := Validate(spec, DefaultProviders(), nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "invalid reference") {
		t.Fatalf("expected invalid reference error, got %v", err)
	}
}

func TestValidateAcceptsWorkflowOutputHelperExpressions(t *testing.T) {
	spec := &WorkflowSpec{
		Version: WorkflowVersion2,
		Name:    "helper-outputs",
		Outputs: map[string]OutputSpec{
			"result": {Value: "${default(nodes.ok.output, 'fallback')}"},
		},
		Nodes: []NodeSpec{{ID: "ok", Kind: NodeKindNoop}},
	}

	if err := Validate(spec, DefaultProviders(), nil); err != nil {
		t.Fatalf("expected helper output expression to validate, got %v", err)
	}
}

func TestValidateAcceptsValidWorkflowOutputs(t *testing.T) {
	spec := &WorkflowSpec{
		Version: WorkflowVersion2,
		Name:    "valid-outputs",
		Outputs: map[string]OutputSpec{
			"result": {Value: "${nodes.ok.output.status}", Type: "string"},
		},
		Nodes: []NodeSpec{{ID: "ok", Kind: NodeKindNoop}},
	}

	if err := Validate(spec, DefaultProviders(), nil); err != nil {
		t.Fatalf("expected valid outputs, got %v", err)
	}
}

func TestValidateV1IgnoresSchema(t *testing.T) {
	spec := &WorkflowSpec{
		Version: WorkflowVersion1,
		Name:    "v1-schema-ignored",
		Inputs: map[string]InputSpec{
			"name": {Type: "string"},
		},
		Nodes: []NodeSpec{{ID: "ok", Kind: NodeKindNoop}},
	}

	if err := Validate(spec, DefaultProviders(), nil); err != nil {
		t.Fatalf("expected v1 to validate without schema, got %v", err)
	}
}

func TestValidateRejectsNodeOutputInvalidType(t *testing.T) {
	spec := &WorkflowSpec{
		Version: WorkflowVersion2,
		Name:    "node-output-type",
		Nodes: []NodeSpec{
			{
				ID:   "bad",
				Kind: NodeKindNoop,
				Outputs: map[string]NodeOutputSpec{
					"result": {Type: "unknown"},
				},
			},
		},
	}

	err := Validate(spec, DefaultProviders(), nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), `node "bad" outputs.result: type must be one of`) {
		t.Fatalf("expected node output type error, got %v", err)
	}
}

func TestValidateRejectsNodeOutputSchemaConflict(t *testing.T) {
	spec := &WorkflowSpec{
		Version: WorkflowVersion2,
		Name:    "node-output-conflict",
		Nodes: []NodeSpec{
			{
				ID:   "bad",
				Kind: NodeKindNoop,
				Outputs: map[string]NodeOutputSpec{
					"result": {Type: "string", Schema: map[string]any{"type": "integer"}},
				},
			},
		},
	}

	err := Validate(spec, DefaultProviders(), nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "conflicts with schema.type") {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestValidateRejectsV2HooksMissingPhase(t *testing.T) {
	spec := &WorkflowSpec{
		Version: WorkflowVersion2,
		Name:    "hooks-missing-phase",
		Nodes:   []NodeSpec{{ID: "ok", Kind: NodeKindNoop}},
		Hooks:   []HookSpec{{Kind: "bash", Command: "echo hi"}},
	}
	err := Validate(spec, DefaultProviders(), nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "hooks[0].phase is required") {
		t.Fatalf("expected phase required error, got %v", err)
	}
}

func TestValidateRejectsV2HooksInvalidPhase(t *testing.T) {
	spec := &WorkflowSpec{
		Version: WorkflowVersion2,
		Name:    "hooks-invalid-phase",
		Nodes:   []NodeSpec{{ID: "ok", Kind: NodeKindNoop}},
		Hooks:   []HookSpec{{Phase: "during_run", Kind: "bash", Command: "echo hi"}},
	}
	err := Validate(spec, DefaultProviders(), nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), `hooks[0].phase "during_run" is not valid`) {
		t.Fatalf("expected invalid phase error, got %v", err)
	}
}

func TestValidateRejectsV2HooksMissingKind(t *testing.T) {
	spec := &WorkflowSpec{
		Version: WorkflowVersion2,
		Name:    "hooks-missing-kind",
		Nodes:   []NodeSpec{{ID: "ok", Kind: NodeKindNoop}},
		Hooks:   []HookSpec{{Phase: "before_run", Command: "echo hi"}},
	}
	err := Validate(spec, DefaultProviders(), nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "hooks[0].kind is required") {
		t.Fatalf("expected kind required error, got %v", err)
	}
}

func TestValidateRejectsV2HooksUnsupportedKind(t *testing.T) {
	spec := &WorkflowSpec{
		Version: WorkflowVersion2,
		Name:    "hooks-unsupported-kind",
		Nodes:   []NodeSpec{{ID: "ok", Kind: NodeKindNoop}},
		Hooks:   []HookSpec{{Phase: "before_run", Kind: "agent", Command: "echo hi"}},
	}
	err := Validate(spec, DefaultProviders(), nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), `hooks[0].kind "agent" is not supported`) {
		t.Fatalf("expected unsupported kind error, got %v", err)
	}
}

func TestValidateRejectsV2HooksMissingCommand(t *testing.T) {
	spec := &WorkflowSpec{
		Version: WorkflowVersion2,
		Name:    "hooks-missing-command",
		Nodes:   []NodeSpec{{ID: "ok", Kind: NodeKindNoop}},
		Hooks:   []HookSpec{{Phase: "before_run", Kind: "bash"}},
	}
	err := Validate(spec, DefaultProviders(), nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "hooks[0].command is required") {
		t.Fatalf("expected command required error, got %v", err)
	}
}

func TestValidateRejectsV2HooksNegativeTimeout(t *testing.T) {
	spec := &WorkflowSpec{
		Version: WorkflowVersion2,
		Name:    "hooks-negative-timeout",
		Nodes:   []NodeSpec{{ID: "ok", Kind: NodeKindNoop}},
		Hooks:   []HookSpec{{Phase: "before_run", Kind: "bash", Command: "echo hi", Timeout: -1}},
	}
	err := Validate(spec, DefaultProviders(), nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "hooks[0].timeout must be >= 0") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestValidateAcceptsV2Hooks(t *testing.T) {
	spec := &WorkflowSpec{
		Version: WorkflowVersion2,
		Name:    "hooks-valid",
		Nodes:   []NodeSpec{{ID: "ok", Kind: NodeKindNoop}},
		Hooks: []HookSpec{
			{Phase: "before_run", Kind: "bash", Command: "echo before"},
			{Phase: "after_success", Kind: "bash", Command: "echo after"},
			{Phase: "after_failure", Kind: "bash", Command: "echo fail"},
			{Phase: "after_run", Kind: "bash", Command: "echo run"},
		},
	}
	if err := Validate(spec, DefaultProviders(), nil); err != nil {
		t.Fatalf("expected valid hooks, got %v", err)
	}
}

func TestValidateAcceptsValidNodeOutputs(t *testing.T) {
	spec := &WorkflowSpec{
		Version: WorkflowVersion2,
		Name:    "node-output-valid",
		Nodes: []NodeSpec{
			{
				ID:   "ok",
				Kind: NodeKindNoop,
				Outputs: map[string]NodeOutputSpec{
					"result": {Type: "string", Schema: map[string]any{"type": "string"}},
				},
			},
		},
	}

	if err := Validate(spec, DefaultProviders(), nil); err != nil {
		t.Fatalf("expected valid node outputs, got %v", err)
	}
}

func TestValidateV2RejectsInvalidExpressionSyntax(t *testing.T) {
	spec := &WorkflowSpec{
		Version: WorkflowVersion2,
		Name:    "v2-bad-expr",
		Nodes: []NodeSpec{
			{
				ID:      "bad",
				Kind:    NodeKindBash,
				Command: "echo ${inputs.name + }",
			},
		},
	}

	err := Validate(spec, DefaultProviders(), nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "compile error") {
		t.Fatalf("expected compile error, got %v", err)
	}
}

func TestValidateV2AcceptsValidExpressions(t *testing.T) {
	spec := &WorkflowSpec{
		Version: WorkflowVersion2,
		Name:    "v2-valid-expr",
		Nodes: []NodeSpec{
			{
				ID:   "prev",
				Kind: NodeKindNoop,
			},
			{
				ID:        "ok",
				Kind:      NodeKindBash,
				Command:   "echo ${inputs.name} ${vars.env} ${default(nodes.prev.output, 'fallback')}",
				When:      "len(inputs.items) > 0 && success('prev')",
				DependsOn: []string{"prev"},
			},
		},
		Outputs: map[string]OutputSpec{
			"result": {Value: "${nodes.ok.output}"},
		},
	}

	if err := Validate(spec, DefaultProviders(), nil); err != nil {
		t.Fatalf("expected valid expressions, got %v", err)
	}
}
