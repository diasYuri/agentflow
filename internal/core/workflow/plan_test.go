package workflow

import "testing"

func TestValidateRejectsCycle(t *testing.T) {
	spec := &WorkflowSpec{
		Version: "1",
		Name:    "cycle",
		Nodes: []NodeSpec{
			{ID: "a", Kind: NodeKindNoop, DependsOn: []string{"b"}},
			{ID: "b", Kind: NodeKindNoop, DependsOn: []string{"a"}},
		},
	}
	if err := Validate(spec, DefaultProviders(), nil); err == nil {
		t.Fatal("expected cycle validation error")
	}
}

func TestBuildPlanPreservesTopologicalOrder(t *testing.T) {
	spec := WorkflowSpec{
		Version: "1",
		Name:    "linear",
		Nodes: []NodeSpec{
			{ID: "plan", Kind: NodeKindNoop},
			{ID: "test", Kind: NodeKindNoop, DependsOn: []string{"plan"}},
		},
	}
	plan, err := BuildPlan(spec)
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Order; len(got) != 2 || got[0] != "plan" || got[1] != "test" {
		t.Fatalf("unexpected order: %#v", got)
	}
}

func TestBuildPlanBuildsChildPlanForMapNodes(t *testing.T) {
	spec := WorkflowSpec{
		Version: "1",
		Name:    "nested",
		Nodes: []NodeSpec{
			{
				ID:      "group",
				Kind:    NodeKindMap,
				ForEach: "${inputs.items}",
				Nodes: []NodeSpec{
					{ID: "draft", Kind: NodeKindNoop},
					{ID: "finish", Kind: NodeKindNoop, DependsOn: []string{"draft"}},
				},
			},
		},
	}
	plan, err := BuildPlan(spec)
	if err != nil {
		t.Fatal(err)
	}
	child := plan.Nodes["group"].ChildPlan
	if child == nil {
		t.Fatal("expected child plan for map node")
	}
	if got := child.Order; len(got) != 2 || got[0] != "draft" || got[1] != "finish" {
		t.Fatalf("unexpected child order: %#v", got)
	}
}

func TestBuildPlanIncludesGoToIfJump(t *testing.T) {
	spec := WorkflowSpec{
		Version: "1",
		Name:    "loop",
		Nodes: []NodeSpec{
			{
				ID:   "check",
				Kind: NodeKindNoop,
				GoToIf: &GoToIfSpec{
					When:   "true",
					Target: "check",
				},
			},
		},
	}
	plan, err := BuildPlan(spec)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(plan.Jumps); got != 1 {
		t.Fatalf("expected 1 jump edge, got %d", got)
	}
	if plan.Jumps[0].From != "check" || plan.Jumps[0].To != "check" {
		t.Fatalf("unexpected jump edge: %#v", plan.Jumps[0])
	}
}
