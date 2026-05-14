package workflow

import (
	"fmt"
	"io"
)

func WriteMermaidGraph(w io.Writer, plan ExecutionPlan) error {
	if _, err := fmt.Fprintln(w, "graph TD"); err != nil {
		return err
	}
	for _, edge := range plan.Edges {
		if _, err := fmt.Fprintf(w, "  %s --> %s\n", edge.From, edge.To); err != nil {
			return err
		}
	}
	for _, edge := range plan.Jumps {
		if _, err := fmt.Fprintf(w, "  %s -.-> %s\n", edge.From, edge.To); err != nil {
			return err
		}
	}
	for _, node := range plan.Workflow.Nodes {
		planned := plan.Nodes[node.ID]
		if len(planned.Dependencies) == 0 && len(planned.Dependents) == 0 {
			if _, err := fmt.Fprintf(w, "  %s\n", node.ID); err != nil {
				return err
			}
		}
	}
	return nil
}
