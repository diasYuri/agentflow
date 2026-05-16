import { Sidebar } from "./components/Sidebar.js";
import { EditorPane } from "./components/EditorPane.js";
import { GraphPane } from "./components/GraphPane.js";
import { DryRunPane } from "./components/DryRunPane.js";
import { RunsPane } from "./components/RunsPane.js";
import { SettingsView } from "./components/SettingsView.js";
import { StatusBar } from "./components/StatusBar.js";
import { useWorkspace } from "./state/workspace.js";
import { useRuns } from "./state/runs.js";
import { useSettings } from "./state/settings.js";
import type { RunWorkflowRequest } from "./api/types.js";

function App() {
	const workspace = useWorkspace();
	const runs = useRuns();
	const settings = useSettings();
	const { state } = workspace;

	const handleRun = async () => {
		const workflowPath = await workspace.ensureWorkflowSaved();
		if (!workflowPath) return;
		let inputs: Record<string, unknown> = {};
		try {
			inputs = JSON.parse(state.inputJson);
		} catch {
			workspace.setError("Invalid JSON input");
			return;
		}
		const req: RunWorkflowRequest = {
			workflow_ref: workflowPath,
			inputs,
			max_concurrency: 4,
			working_dir: "",
		};
		runs.startRun(req).catch(() => {
			// error already in state
		});
	};

	return (
		<div className="app-shell">
			<Sidebar
				state={state}
				onOpenPath={workspace.openWorkflow}
				onSetView={workspace.setView}
				onValidate={workspace.doValidate}
				onGraph={workspace.doGraph}
				onDryRun={workspace.doDryRun}
				onSave={workspace.saveWorkflow}
				onRun={handleRun}
				onShowRuns={() => workspace.setView("runs")}
				onShowSettings={() => workspace.setView("settings")}
				runsCount={runs.state.runs.length}
				runError={runs.state.error}
			/>
			<div className="main-area">
				<div className="main-content">
					{state.error && (
						<div className="alert alert-error">{state.error}</div>
					)}
					{runs.state.error && state.view !== "runs" && (
						<div className="alert alert-error">{runs.state.error}</div>
					)}
					{state.validation?.valid && (
						<div className="alert alert-success">
							Valid: {state.validation.name} ({state.validation.nodeCount}{" "}
							nodes)
						</div>
					)}
					{state.validation && !state.validation.valid && (
						<div className="alert alert-error">
							Validation failed for {state.validation.name}
							{state.validation.errors &&
								state.validation.errors.length > 0 && (
									<ul style={{ margin: "0.5rem 0 0", paddingLeft: "1.25rem" }}>
										{state.validation.errors.map((e, i) => (
											<li key={i}>
												<strong>[{e.code}]</strong> {e.message}
												{e.context && <span> — {e.context}</span>}
											</li>
										))}
									</ul>
								)}
						</div>
					)}
					{state.view === "editor" && (
						<EditorPane
							state={state}
							onChangeYaml={workspace.setWorkflowYaml}
							onChangeInput={workspace.setInputJson}
						/>
					)}
					{state.view === "graph" && <GraphPane state={state} />}
					{state.view === "dry-run" && <DryRunPane state={state} />}
					{state.view === "runs" && (
						<RunsPane
							state={runs.state}
							onSelectRun={runs.selectRun}
							onCancelRun={runs.cancelRun}
						/>
					)}
					{state.view === "settings" && (
						<SettingsView
							settings={settings.state.settings}
							isLoading={settings.state.isLoading}
							error={settings.state.error}
							onSave={settings.saveSettings}
						/>
					)}
				</div>
				<StatusBar state={state} runsState={runs.state} />
			</div>
		</div>
	);
}

export default App;
