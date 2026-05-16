import type { RunsState } from "../state/runs.js";
import { RunDetail } from "./RunDetail.js";

interface RunsPaneProps {
	state: RunsState;
	onSelectRun: (runId: string | null) => void;
	onCancelRun: (runId: string) => void;
}

export function RunsPane({ state, onSelectRun, onCancelRun }: RunsPaneProps) {
	const selected = state.selectedRunId
		? (state.details[state.selectedRunId] ?? null)
		: null;

	return (
		<div className="runs-pane">
			<div className="runs-list">
				<div className="runs-list-header">
					<span className="runs-count">{state.runs.length} runs</span>
				</div>
				{state.error && (
					<div className="alert alert-error" style={{ margin: "0.5rem" }}>
						{state.error}
					</div>
				)}
				{state.runs.length === 0 && (
					<div className="empty-state">
						<div className="empty-title">No runs yet</div>
						<div className="empty-subtitle">
							Open a workflow and press Run to start.
						</div>
					</div>
				)}
				{state.runs.map((run) => (
					<button
						key={run.id}
						className={`run-item ${state.selectedRunId === run.id ? "active" : ""}`}
						onClick={() => onSelectRun(run.id)}
					>
						<div className="run-item-row">
							<span className={`run-item-status status-${run.status}`}>
								{run.status || "unknown"}
							</span>
							<span className="run-item-workflow">{run.workflow}</span>
						</div>
						<div className="run-item-row secondary">
							<span className="run-item-id">{run.id}</span>
							<span className="run-item-time">
								{new Date(run.started_at).toLocaleString()}
							</span>
						</div>
					</button>
				))}
			</div>
			<div className="runs-detail">
				{selected ? (
					<RunDetail
						detail={selected}
						onCancel={() => onCancelRun(selected.run.id)}
					/>
				) : (
					<div className="empty-state">
						<div className="empty-title">Select a run</div>
						<div className="empty-subtitle">
							Choose a run from the list to inspect details.
						</div>
					</div>
				)}
			</div>
		</div>
	);
}
