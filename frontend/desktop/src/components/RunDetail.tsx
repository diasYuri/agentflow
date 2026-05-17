import { useState } from "react";
import type { RunDetail as RunDetailState, RunTab } from "../state/runs.js";
import { RunTimeline } from "./RunTimeline.js";
import { LogConsole } from "./LogConsole.js";
import { ArtifactsPane } from "./ArtifactsPane.js";

interface RunDetailProps {
	detail: RunDetailState;
	onCancel: () => void;
}

const tabs: { key: RunTab; label: string }[] = [
	{ key: "timeline", label: "Timeline" },
	{ key: "logs", label: "Logs" },
	{ key: "artifacts", label: "Artifacts" },
];

export function RunDetail({ detail, onCancel }: RunDetailProps) {
	const [tab, setTab] = useState<RunTab>("timeline");
	const [confirming, setConfirming] = useState(false);
	const run = detail.run;

	const isActive =
		run.status === "created" ||
		run.status === "running" ||
		run.status === "validating" ||
		run.status === "planned";

	return (
		<div className="run-detail">
			<div className="run-detail-header">
				<div className="run-detail-meta">
					<span className={`run-status status-${run.status}`}>
						{run.status || "unknown"}
					</span>
					<span className="run-id">{run.id}</span>
					<span className="run-workflow">{run.workflow}</span>
					{run.tag && <span className="run-tag">{run.tag}</span>}
					{run.total_steps != null && (
						<span className="run-progress">
							{run.completed_steps?.length ?? 0} / {run.total_steps} steps
						</span>
					)}
				</div>
				<div className="run-detail-actions">
					{isActive && (
						<>
							<button
								className="action-btn danger"
								onClick={() => {
									if (!confirming) {
										setConfirming(true);
										return;
									}
									setConfirming(false);
									onCancel();
								}}
							>
								{confirming ? "Confirm Cancel" : "Cancel Run"}
							</button>
							{confirming && (
								<button
									className="action-btn secondary"
									onClick={() => setConfirming(false)}
								>
									Back
								</button>
							)}
						</>
					)}
				</div>
			</div>
			{run.error && <div className="alert alert-error">{run.error}</div>}
			<div className="run-detail-tabs">
				{tabs.map((t) => (
					<button
						key={t.key}
						className={`tab-btn ${tab === t.key ? "active" : ""}`}
						onClick={() => setTab(t.key)}
					>
						{t.label}
					</button>
				))}
			</div>
			<div className="run-detail-body">
				{tab === "timeline" && <RunTimeline events={detail.events} />}
				{tab === "logs" && <LogConsole lines={detail.logs} />}
				{tab === "artifacts" && (
					<ArtifactsPane artifacts={detail.artifacts} runId={run.id} />
				)}
			</div>
		</div>
	);
}
