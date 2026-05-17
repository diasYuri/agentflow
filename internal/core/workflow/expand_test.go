package workflow

import (
	"strings"
	"testing"
)

func TestExpandMacrosNoOpForV1(t *testing.T) {
	spec := &WorkflowSpec{
		Version: WorkflowVersion1,
		Nodes: []NodeSpec{
			{ID: "a", Kind: NodeKindNoop},
		},
	}
	if err := ExpandMacros(spec); err != nil {
		t.Fatal(err)
	}
	if len(spec.Nodes) != 1 || spec.Nodes[0].ID != "a" {
		t.Fatalf("unexpected nodes: %+v", spec.Nodes)
	}
}

func TestExpandMacrosSingleNodeStep(t *testing.T) {
	spec := &WorkflowSpec{
		Version: WorkflowVersion2,
		Steps: map[string]ReusableStepSpec{
			"greet": {
				Parameters: []string{"name"},
				Nodes: []NodeSpec{
					{ID: "say", Kind: NodeKindBash, Command: "echo ${name}"},
				},
			},
		},
		Nodes: []NodeSpec{
			{ID: "hello", Ref: "greet", Params: map[string]any{"name": "world"}},
		},
	}
	if err := ExpandMacros(spec); err != nil {
		t.Fatal(err)
	}
	if len(spec.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(spec.Nodes))
	}
	if spec.Nodes[0].ID != "hello" {
		t.Fatalf("expected id hello, got %q", spec.Nodes[0].ID)
	}
	if spec.Nodes[0].Command != "echo world" {
		t.Fatalf("expected command 'echo world', got %q", spec.Nodes[0].Command)
	}
}

func TestExpandMacrosMultiNodeStep(t *testing.T) {
	spec := &WorkflowSpec{
		Version: WorkflowVersion2,
		Steps: map[string]ReusableStepSpec{
			"pipeline": {
				Parameters: []string{"target"},
				Nodes: []NodeSpec{
					{ID: "build", Kind: NodeKindBash, Command: "build ${target}"},
					{ID: "test", Kind: NodeKindBash, Command: "test ${target}", DependsOn: []string{"build"}},
				},
			},
		},
		Nodes: []NodeSpec{
			{ID: "ci", Ref: "pipeline", Params: map[string]any{"target": "app"}},
		},
	}
	if err := ExpandMacros(spec); err != nil {
		t.Fatal(err)
	}
	if len(spec.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(spec.Nodes))
	}
	build, ok := nodeByID(spec.Nodes, "ci-build")
	if !ok {
		t.Fatalf("expected ci-build node")
	}
	if build.Command != "build app" {
		t.Fatalf("expected 'build app', got %q", build.Command)
	}
	test, ok := nodeByID(spec.Nodes, "ci-test")
	if !ok {
		t.Fatalf("expected ci-test node")
	}
	if test.Command != "test app" {
		t.Fatalf("expected 'test app', got %q", test.Command)
	}
	if len(test.DependsOn) != 1 || test.DependsOn[0] != "ci-build" {
		t.Fatalf("unexpected deps: %+v", test.DependsOn)
	}
}

func TestExpandMacrosRequiresIDForMultiNode(t *testing.T) {
	spec := &WorkflowSpec{
		Version: WorkflowVersion2,
		Steps: map[string]ReusableStepSpec{
			"pipeline": {
				Parameters: []string{"x"},
				Nodes: []NodeSpec{
					{ID: "a", Kind: NodeKindNoop},
					{ID: "b", Kind: NodeKindNoop, DependsOn: []string{"a"}},
				},
			},
		},
		Nodes: []NodeSpec{
			{Ref: "pipeline", Params: map[string]any{"x": "y"}},
		},
	}
	err := ExpandMacros(spec)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "must have an id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExpandMacrosUnknownStep(t *testing.T) {
	spec := &WorkflowSpec{
		Version: WorkflowVersion2,
		Nodes: []NodeSpec{
			{ID: "x", Ref: "missing"},
		},
	}
	err := ExpandMacros(spec)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown step") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExpandMacrosRecursiveStep(t *testing.T) {
	spec := &WorkflowSpec{
		Version: WorkflowVersion2,
		Steps: map[string]ReusableStepSpec{
			"a": {
				Parameters: []string{"x"},
				Nodes: []NodeSpec{
					{ID: "n", Ref: "a", Params: map[string]any{"x": "y"}},
				},
			},
		},
		Nodes: []NodeSpec{
			{ID: "root", Ref: "a", Params: map[string]any{"x": "y"}},
		},
	}
	err := ExpandMacros(spec)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "recursive step expansion") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExpandMacrosUndeclaredParam(t *testing.T) {
	spec := &WorkflowSpec{
		Version: WorkflowVersion2,
		Steps: map[string]ReusableStepSpec{
			"a": {
				Parameters: []string{"x"},
				Nodes: []NodeSpec{
					{ID: "n", Kind: NodeKindNoop},
				},
			},
		},
		Nodes: []NodeSpec{
			{ID: "root", Ref: "a", Params: map[string]any{"x": "y", "z": "w"}},
		},
	}
	err := ExpandMacros(spec)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "does not declare parameter") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExpandMacrosMissingParam(t *testing.T) {
	spec := &WorkflowSpec{
		Version: WorkflowVersion2,
		Steps: map[string]ReusableStepSpec{
			"a": {
				Parameters: []string{"x"},
				Nodes: []NodeSpec{
					{ID: "n", Kind: NodeKindNoop},
				},
			},
		},
		Nodes: []NodeSpec{
			{ID: "root", Ref: "a", Params: map[string]any{}},
		},
	}
	err := ExpandMacros(spec)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "requires parameter") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func nodeByID(nodes []NodeSpec, id string) (NodeSpec, bool) {
	for _, n := range nodes {
		if n.ID == id {
			return n, true
		}
	}
	return NodeSpec{}, false
}
