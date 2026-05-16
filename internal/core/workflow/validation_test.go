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
