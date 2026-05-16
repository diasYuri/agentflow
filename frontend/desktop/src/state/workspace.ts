import { useCallback, useEffect, useState } from "react";
import * as api from "../api/bindings.js";

export type View = "editor" | "graph" | "dry-run" | "runs" | "settings";

export interface WorkspaceState {
	currentPath: string | null;
	workflow: api.LoadedWorkflow | null;
	workflowYaml: string;
	inputJson: string;
	recentFiles: string[];
	validation: api.ValidationResult | null;
	graph: api.GraphResult | null;
	dryRun: api.DryRunResult | null;
	view: View;
	isLoading: boolean;
	error: string | null;
	dirty: boolean;
}

export function useWorkspace() {
	const [state, setState] = useState<WorkspaceState>({
		currentPath: null,
		workflow: null,
		workflowYaml: "",
		inputJson: "{}",
		recentFiles: [],
		validation: null,
		graph: null,
		dryRun: null,
		view: "editor",
		isLoading: false,
		error: null,
		dirty: false,
	});

	const setError = useCallback((error: string | null) => {
		setState((s) => ({ ...s, error }));
	}, []);

	const clearResults = useCallback(() => {
		setState((s) => ({
			...s,
			validation: null,
			graph: null,
			dryRun: null,
			error: null,
		}));
	}, []);

	const loadSettings = useCallback(async () => {
		try {
			const settings = await api.getAppSettings();
			setState((s) => ({ ...s, recentFiles: settings.recentFiles ?? [] }));
		} catch {
			// silently ignore settings load errors
		}
	}, []);

	const openWorkflow = useCallback(async (path: string) => {
		setState((s) => ({ ...s, isLoading: true, error: null }));
		try {
			const wf = await api.loadWorkflow(path);
			const inputs: Record<string, unknown> = {};
			for (const [key, doc] of Object.entries(wf.inputs ?? {})) {
				if (!doc) continue;
				inputs[key] =
					doc.default ??
					(doc.type === "string"
						? ""
						: doc.type === "boolean"
							? false
							: doc.type === "number"
								? 0
								: null);
			}
			setState((s) => ({
				...s,
				currentPath: path,
				workflow: wf,
				workflowYaml: wf.rawYaml ?? "",
				inputJson: JSON.stringify(inputs, null, 2),
				recentFiles: [path, ...s.recentFiles.filter((r) => r !== path)].slice(
					0,
					20,
				),
				validation: null,
				graph: null,
				dryRun: null,
				dirty: false,
				isLoading: false,
				view: s.view,
			}));
		} catch (err) {
			setState((s) => ({
				...s,
				isLoading: false,
				error: err instanceof Error ? err.message : String(err),
			}));
		}
	}, []);

	const setWorkflowYaml = useCallback((workflowYaml: string) => {
		setState((s) => ({ ...s, workflowYaml, dirty: true }));
	}, []);

	const setInputJson = useCallback((inputJson: string) => {
		setState((s) => ({ ...s, inputJson }));
	}, []);

	const setView = useCallback((view: View) => {
		setState((s) => ({ ...s, view }));
	}, []);

	const saveWorkflow = useCallback(async () => {
		if (!state.currentPath) return;
		setState((s) => ({ ...s, isLoading: true, error: null }));
		try {
			await api.saveWorkflow(state.currentPath, state.workflowYaml);
			setState((s) => ({ ...s, dirty: false, isLoading: false }));
		} catch (err) {
			setState((s) => ({
				...s,
				isLoading: false,
				error: err instanceof Error ? err.message : String(err),
			}));
		}
	}, [state.currentPath, state.workflowYaml]);

	const ensureWorkflowSaved = useCallback(async () => {
		if (!state.currentPath) return null;
		if (!state.dirty) return state.currentPath;
		setState((s) => ({ ...s, isLoading: true, error: null }));
		try {
			await api.saveWorkflow(state.currentPath, state.workflowYaml);
			setState((s) => ({ ...s, dirty: false, isLoading: false }));
			return state.currentPath;
		} catch (err) {
			setState((s) => ({
				...s,
				isLoading: false,
				error: err instanceof Error ? err.message : String(err),
			}));
			return null;
		}
	}, [state.currentPath, state.dirty, state.workflowYaml]);

	const doValidate = useCallback(async () => {
		const workflowPath = await ensureWorkflowSaved();
		if (!workflowPath) return;
		setState((s) => ({ ...s, isLoading: true, error: null, validation: null }));
		try {
			const result = await api.validateWorkflow(workflowPath);
			setState((s) => ({ ...s, validation: result, isLoading: false }));
		} catch (err) {
			setState((s) => ({
				...s,
				isLoading: false,
				error: err instanceof Error ? err.message : String(err),
			}));
		}
	}, [ensureWorkflowSaved]);

	const doGraph = useCallback(async () => {
		const workflowPath = await ensureWorkflowSaved();
		if (!workflowPath) return;
		setState((s) => ({ ...s, isLoading: true, error: null, graph: null }));
		try {
			const result = await api.generateGraph(workflowPath);
			setState((s) => ({ ...s, graph: result, isLoading: false }));
		} catch (err) {
			setState((s) => ({
				...s,
				isLoading: false,
				error: err instanceof Error ? err.message : String(err),
			}));
		}
	}, [ensureWorkflowSaved]);

	const doDryRun = useCallback(async () => {
		const workflowPath = await ensureWorkflowSaved();
		if (!workflowPath) return;
		let inputs: Record<string, unknown> = {};
		try {
			inputs = JSON.parse(state.inputJson);
		} catch {
			setState((s) => ({ ...s, error: "Invalid JSON input" }));
			return;
		}
		setState((s) => ({ ...s, isLoading: true, error: null, dryRun: null }));
		try {
			const resolved = await api.resolveInput(workflowPath, inputs);
			const result = await api.dryRunWorkflow(workflowPath, resolved);
			setState((s) => ({ ...s, dryRun: result, isLoading: false }));
		} catch (err) {
			setState((s) => ({
				...s,
				isLoading: false,
				error: err instanceof Error ? err.message : String(err),
			}));
		}
	}, [ensureWorkflowSaved, state.inputJson]);

	useEffect(() => {
		loadSettings();
	}, [loadSettings]);

	return {
		state,
		setError,
		clearResults,
		openWorkflow,
		setWorkflowYaml,
		setInputJson,
		setView,
		saveWorkflow,
		ensureWorkflowSaved,
		doValidate,
		doGraph,
		doDryRun,
	};
}
