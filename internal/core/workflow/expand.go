package workflow

import (
	"fmt"
	"strings"
)

// ExpandMacros expands reusable step references (node.Ref) into concrete nodes.
// It mutates the spec in place, replacing nodes that reference steps with the
// step's node list after parameter substitution. Recursive expansion is rejected.
func ExpandMacros(spec *WorkflowSpec) error {
	if spec.Version != WorkflowVersion2 {
		return nil
	}
	expanded, err := expandNodes(spec.Nodes, spec.Steps, nil)
	if err != nil {
		return err
	}
	spec.Nodes = expanded
	return nil
}

func expandNodes(nodes []NodeSpec, steps map[string]ReusableStepSpec, expanding map[string]struct{}) ([]NodeSpec, error) {
	if expanding == nil {
		expanding = make(map[string]struct{})
	}
	var result []NodeSpec
	for _, node := range nodes {
		if node.Ref == "" {
			if len(node.Nodes) > 0 {
				children, err := expandNodes(node.Nodes, steps, expanding)
				if err != nil {
					return nil, err
				}
				node.Nodes = children
			}
			result = append(result, node)
			continue
		}
		step, ok := steps[node.Ref]
		if !ok {
			return nil, fmt.Errorf("unknown step %q", node.Ref)
		}
		if _, ok := expanding[node.Ref]; ok {
			return nil, fmt.Errorf("recursive step expansion detected: %q", node.Ref)
		}
		expanding[node.Ref] = struct{}{}
		expanded, err := expandStep(step, node, steps, expanding)
		delete(expanding, node.Ref)
		if err != nil {
			return nil, err
		}
		result = append(result, expanded...)
	}
	return result, nil
}

func expandStep(step ReusableStepSpec, caller NodeSpec, steps map[string]ReusableStepSpec, expanding map[string]struct{}) ([]NodeSpec, error) {
	if len(step.Nodes) == 0 {
		return nil, fmt.Errorf("step %q defines no nodes", caller.Ref)
	}

	paramSet := make(map[string]struct{}, len(step.Parameters))
	for _, p := range step.Parameters {
		paramSet[p] = struct{}{}
	}
	for key := range caller.Params {
		if _, ok := paramSet[key]; !ok {
			return nil, fmt.Errorf("step %q does not declare parameter %q", caller.Ref, key)
		}
	}

	paramValues := make(map[string]string, len(step.Parameters))
	for _, p := range step.Parameters {
		v, ok := caller.Params[p]
		if !ok {
			return nil, fmt.Errorf("step %q requires parameter %q", caller.Ref, p)
		}
		paramValues[p] = fmt.Sprint(v)
	}

	prefix := caller.ID
	if prefix == "" && len(step.Nodes) > 1 {
		return nil, fmt.Errorf("node using step %q must have an id because the step produces multiple nodes", caller.Ref)
	}

	expanded := make([]NodeSpec, len(step.Nodes))
	for i, n := range step.Nodes {
		clone := cloneNode(n)
		clone = substituteParamsInNode(clone, paramValues)
		if len(step.Nodes) > 1 {
			clone.ID = prefix + "-" + clone.ID
			for j, dep := range clone.DependsOn {
				clone.DependsOn[j] = prefix + "-" + dep
			}
			if clone.GoToIf != nil {
				clone.GoToIf.Target = prefix + "-" + clone.GoToIf.Target
			}
		} else if prefix != "" {
			clone.ID = prefix
		}
		expanded[i] = clone
	}

	return expandNodes(expanded, steps, expanding)
}

func substituteParamsInNode(node NodeSpec, params map[string]string) NodeSpec {
	node.Prompt = substituteParams(node.Prompt, params)
	node.System = substituteParams(node.System, params)
	node.Command = substituteParams(node.Command, params)
	node.When = substituteParams(node.When, params)
	node.ForEach = substituteParams(node.ForEach, params)
	node.WorkingDir = substituteParams(node.WorkingDir, params)
	node.Input = substituteParams(node.Input, params)
	node.Operation = substituteParams(node.Operation, params)
	node.Provider = substituteParams(node.Provider, params)
	node.Model = substituteParams(node.Model, params)
	node.Shell = substituteParams(node.Shell, params)
	if node.GoToIf != nil {
		node.GoToIf.When = substituteParams(node.GoToIf.When, params)
		node.GoToIf.Target = substituteParams(node.GoToIf.Target, params)
	}
	for k, v := range node.Env {
		node.Env[k] = substituteParams(v, params)
	}
	for i := range node.Nodes {
		node.Nodes[i] = substituteParamsInNode(node.Nodes[i], params)
	}
	return node
}

func substituteParams(s string, params map[string]string) string {
	if s == "" {
		return s
	}
	for k, v := range params {
		s = strings.ReplaceAll(s, "${"+k+"}", v)
	}
	return s
}

func cloneNode(n NodeSpec) NodeSpec {
	clone := n
	if n.DependsOn != nil {
		clone.DependsOn = append([]string(nil), n.DependsOn...)
	}
	if n.Nodes != nil {
		clone.Nodes = make([]NodeSpec, len(n.Nodes))
		for i, child := range n.Nodes {
			clone.Nodes[i] = cloneNode(child)
		}
	}
	if n.Env != nil {
		clone.Env = make(map[string]string, len(n.Env))
		for k, v := range n.Env {
			clone.Env[k] = v
		}
	}
	if n.OutputSchema != nil {
		clone.OutputSchema = make(map[string]any, len(n.OutputSchema))
		for k, v := range n.OutputSchema {
			clone.OutputSchema[k] = v
		}
	}
	if n.Params != nil {
		clone.Params = make(map[string]any, len(n.Params))
		for k, v := range n.Params {
			clone.Params[k] = v
		}
	}
	if n.Permission != nil {
		p := *n.Permission
		clone.Permission = &p
	}
	if n.GoToIf != nil {
		g := *n.GoToIf
		clone.GoToIf = &g
	}
	return clone
}
