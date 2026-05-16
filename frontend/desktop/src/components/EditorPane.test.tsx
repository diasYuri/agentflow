import "@testing-library/jest-dom";
import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { EditorPane } from "./EditorPane.js";

describe("EditorPane", () => {
	const baseState = {
		currentPath: null,
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

	it("shows empty state when no workflow", () => {
		render(
			<EditorPane
				state={baseState}
				onChangeYaml={vi.fn()}
				onChangeInput={vi.fn()}
			/>,
		);
		expect(screen.getByText(/No workflow open/)).toBeInTheDocument();
	});

	it("renders yaml and json editors when workflow is loaded", () => {
		const state = {
			...baseState,
			workflow: {
				name: "Test",
				description: "",
				version: "",
				inputs: {},
				nodes: [],
				sourcePath: "/test.yaml",
				rawYaml: "name: test",
			},
			workflowYaml: "name: test",
			inputJson: "{}",
		};
		render(
			<EditorPane
				state={state}
				onChangeYaml={vi.fn()}
				onChangeInput={vi.fn()}
			/>,
		);
		expect(screen.getByText(/Workflow YAML/)).toBeInTheDocument();
		expect(screen.getByText(/Input JSON/)).toBeInTheDocument();
	});

	it("shows dirty badge when modified", () => {
		const state = {
			...baseState,
			workflow: {
				name: "Test",
				description: "",
				version: "",
				inputs: {},
				nodes: [],
				sourcePath: "/test.yaml",
				rawYaml: "name: test",
			},
			workflowYaml: "name: test2",
			inputJson: "{}",
			dirty: true,
		};
		render(
			<EditorPane
				state={state}
				onChangeYaml={vi.fn()}
				onChangeInput={vi.fn()}
			/>,
		);
		expect(screen.getByText(/unsaved/)).toBeInTheDocument();
	});

	it("calls onChangeYaml when typing", () => {
		const onChangeYaml = vi.fn();
		const state = {
			...baseState,
			workflow: {
				name: "Test",
				description: "",
				version: "",
				inputs: {},
				nodes: [],
				sourcePath: "/test.yaml",
				rawYaml: "name: test",
			},
			workflowYaml: "name: test",
			inputJson: "{}",
		};
		render(
			<EditorPane
				state={state}
				onChangeYaml={onChangeYaml}
				onChangeInput={vi.fn()}
			/>,
		);
		const textarea = screen.getAllByRole("textbox")[0];
		fireEvent.change(textarea, { target: { value: "name: test\n" } });
		expect(onChangeYaml).toHaveBeenCalledWith("name: test\n");
	});
});
