import "@testing-library/jest-dom";
import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { DryRunPane } from "./DryRunPane.js";

const baseState = {
	currentPath: null,
	workflow: null,
	workflowYaml: "",
	inputJson: "{}",
	recentFiles: [],
	validation: null,
	graph: null,
	dryRun: null,
	view: "dry-run" as const,
	isLoading: false,
	error: null,
	dirty: false,
};

describe("DryRunPane", () => {
	it("shows empty state when no workflow", () => {
		render(<DryRunPane state={baseState} />);
		expect(screen.getByText(/No workflow open/)).toBeInTheDocument();
	});

	it("shows prompt when workflow is open but dry-run not executed", () => {
		const state = {
			...baseState,
			workflow: {
				name: "Test",
				description: "",
				version: "",
				inputs: {},
				nodes: [],
				sourcePath: "/test.yaml",
				rawYaml: "",
			},
		};
		render(<DryRunPane state={state} />);
		expect(screen.getByText(/Dry-run not executed/)).toBeInTheDocument();
	});

	it("renders dry-run results", () => {
		const state = {
			...baseState,
			workflow: {
				name: "Test",
				description: "",
				version: "",
				inputs: {},
				nodes: [],
				sourcePath: "/test.yaml",
				rawYaml: "",
			},
			dryRun: {
				valid: true,
				workflow: "Test",
				inputs: { name: "value" },
				order: ["a"],
				nodes: {
					a: { id: "a", kind: "task", dependencies: [], dependents: [] },
				},
				errors: [],
			},
		};
		render(<DryRunPane state={state} />);
		expect(screen.getByText(/Test/)).toBeInTheDocument();
		expect(screen.getByText(/Valid/)).toBeInTheDocument();
		expect(screen.getByText(/Resolved Inputs/)).toBeInTheDocument();
		expect(screen.getByText(/Execution Order/)).toBeInTheDocument();
	});
});
