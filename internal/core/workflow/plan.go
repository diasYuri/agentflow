package workflow

import (
	"fmt"
	"sort"
)

type ExecutionPlan struct {
	Workflow WorkflowSpec           `json:"workflow"`
	Nodes    map[string]PlannedNode `json:"nodes"`
	Edges    []Edge                 `json:"edges"`
	Jumps    []Edge                 `json:"jumps,omitempty"`
	Order    []string               `json:"order"`
}

type PlannedNode struct {
	Spec         NodeSpec       `json:"spec"`
	Dependencies []string       `json:"dependencies"`
	Dependents   []string       `json:"dependents"`
	Index        int            `json:"index"`
	ChildPlan    *ExecutionPlan `json:"child_plan,omitempty"`
}

type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func BuildPlan(spec WorkflowSpec) (ExecutionPlan, error) {
	return buildPlan(spec, nil)
}

func buildPlan(spec WorkflowSpec, external map[string]struct{}) (ExecutionPlan, error) {
	nodes := make(map[string]PlannedNode, len(spec.Nodes))
	localIDs := make(map[string]struct{}, len(spec.Nodes))
	for i, node := range spec.Nodes {
		localIDs[node.ID] = struct{}{}
		planned := PlannedNode{
			Spec:         node,
			Dependencies: append([]string(nil), node.DependsOn...),
			Dependents:   []string{},
			Index:        i,
		}
		nodes[node.ID] = planned
	}
	scope := make(map[string]struct{}, len(localIDs)+len(external))
	for id := range localIDs {
		scope[id] = struct{}{}
	}
	for id := range external {
		scope[id] = struct{}{}
	}
	for _, node := range spec.Nodes {
		if len(node.Nodes) > 0 {
			childPlan, err := buildPlan(WorkflowSpec{
				Version:   spec.Version,
				Name:      spec.Name + "/" + node.ID,
				Inputs:    spec.Inputs,
				Vars:      spec.Vars,
				Secrets:   spec.Secrets,
				Defaults:  spec.Defaults,
				Execution: spec.Execution,
				Nodes:     node.Nodes,
			}, scope)
			if err != nil {
				return ExecutionPlan{}, err
			}
			planned := nodes[node.ID]
			planned.ChildPlan = &childPlan
			nodes[node.ID] = planned
		}
	}

	var edges []Edge
	for _, node := range spec.Nodes {
		for _, dep := range node.DependsOn {
			if _, ok := nodes[dep]; !ok {
				if external != nil {
					if _, externalOK := external[dep]; externalOK {
						continue
					}
				}
				return ExecutionPlan{}, fmt.Errorf("node %q depends on unknown node %q", node.ID, dep)
			}
			plannedDep := nodes[dep]
			plannedDep.Dependents = append(plannedDep.Dependents, node.ID)
			nodes[dep] = plannedDep
			edges = append(edges, Edge{From: dep, To: node.ID})
		}
	}

	order, err := topo(nodes, external)
	if err != nil {
		return ExecutionPlan{}, err
	}
	jumps, err := conditionalJumps(spec, order)
	if err != nil {
		return ExecutionPlan{}, err
	}
	return ExecutionPlan{Workflow: spec, Nodes: nodes, Edges: edges, Jumps: jumps, Order: order}, nil
}

func topo(nodes map[string]PlannedNode, external map[string]struct{}) ([]string, error) {
	colors := map[string]int{}
	var order []string
	var stack []string

	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return nodes[ids[i]].Index < nodes[ids[j]].Index
	})

	var visit func(id string) error
	visit = func(id string) error {
		switch colors[id] {
		case 1:
			return fmt.Errorf("cycle detected: %s", cyclePath(stack, id))
		case 2:
			return nil
		}
		colors[id] = 1
		stack = append(stack, id)
		deps := make([]string, 0, len(nodes[id].Dependencies))
		for _, dep := range nodes[id].Dependencies {
			if _, ok := nodes[dep]; ok {
				deps = append(deps, dep)
			}
		}
		sort.Slice(deps, func(i, j int) bool {
			return nodes[deps[i]].Index < nodes[deps[j]].Index
		})
		for _, dep := range deps {
			if err := visit(dep); err != nil {
				return err
			}
		}
		stack = stack[:len(stack)-1]
		colors[id] = 2
		order = append(order, id)
		return nil
	}

	for _, id := range ids {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	return order, nil
}

func conditionalJumps(spec WorkflowSpec, order []string) ([]Edge, error) {
	position := make(map[string]int, len(order))
	for i, id := range order {
		position[id] = i
	}
	var jumps []Edge
	for _, node := range spec.Nodes {
		if node.GoToIf == nil || node.GoToIf.Target == "" {
			continue
		}
		targetPos, ok := position[node.GoToIf.Target]
		if !ok {
			return nil, fmt.Errorf("node %q go_to_if.target references unknown node %q", node.ID, node.GoToIf.Target)
		}
		if targetPos > position[node.ID] {
			return nil, fmt.Errorf("node %q go_to_if.target must point to the current node or an earlier node", node.ID)
		}
		jumps = append(jumps, Edge{From: node.ID, To: node.GoToIf.Target})
	}
	return jumps, nil
}

func cyclePath(stack []string, id string) string {
	for i, item := range stack {
		if item == id {
			path := append([]string(nil), stack[i:]...)
			path = append(path, id)
			out := path[0]
			for _, next := range path[1:] {
				out += " -> " + next
			}
			return out
		}
	}
	return id
}
