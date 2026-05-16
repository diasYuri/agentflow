import "@testing-library/jest-dom";
import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { StatusBar } from "./StatusBar.js";

describe("StatusBar", () => {
	const baseState = {
		currentPath: "/path/to/workflow.yaml",
		workflow: null,
		workflowYaml: "",
		inputJson: "{}",
		recentFiles: [],
		validation: null,
		graph: null,
		dryRun: null,
		view: "editor" as const,
		isLoading: false,
		error: null,
		dirty: false,
	};

	it("renders current path", () => {
		render(<StatusBar state={baseState} />);
		expect(screen.getByText(/path\/to\/workflow\.yaml/)).toBeInTheDocument();
	});

	it("shows modified when dirty", () => {
		render(<StatusBar state={{ ...baseState, dirty: true }} />);
		expect(screen.getByText(/Modified/)).toBeInTheDocument();
	});

	it("shows loading state", () => {
		render(<StatusBar state={{ ...baseState, isLoading: true }} />);
		expect(screen.getByText(/Loading/)).toBeInTheDocument();
	});

	it("shows node count when workflow is loaded", () => {
		render(
			<StatusBar
				state={{
					...baseState,
					workflow: {
						name: "Test",
						description: "",
						version: "",
						inputs: {},
						nodes: [{ id: "a", kind: "task" }],
						sourcePath: "/test.yaml",
					},
				}}
			/>,
		);
		expect(screen.getByText(/1 nodes/)).toBeInTheDocument();
	});
});
