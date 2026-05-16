import "@testing-library/jest-dom";
import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { GraphPane } from "./GraphPane.js";

const baseState = {
	currentPath: null,
	workflow: null,
	workflowYaml: "",
	inputJson: "{}",
	recentFiles: [],
	validation: null,
	graph: null,
	dryRun: null,
	view: "graph" as const,
	isLoading: false,
	error: null,
	dirty: false,
};

describe("GraphPane", () => {
	it("shows empty state when no workflow", () => {
		render(<GraphPane state={baseState} />);
		expect(screen.getByText(/No workflow open/)).toBeInTheDocument();
	});

	it("renders mermaid output when graph is available", () => {
		const state = {
			...baseState,
			workflow: {
				name: "Test",
				description: "",
				version: "",
				inputs: {},
				nodes: [{ id: "a", kind: "task" }],
				sourcePath: "/test.yaml",
				rawYaml: "",
			},
			graph: { valid: true, mermaid: "graph TD\nA-->B" },
		};
		render(<GraphPane state={state} />);
		expect(screen.getByText(/graph TD/)).toBeInTheDocument();
	});
});
