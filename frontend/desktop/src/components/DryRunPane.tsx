import type { WorkspaceState } from "../state/workspace.js";

interface DryRunPaneProps {
	state: WorkspaceState;
}

export function DryRunPane({ state }: DryRunPaneProps) {
	if (!state.workflow) {
		return (
			<div className="empty-state">
				<div className="empty-icon">🧪</div>
				<div className="empty-title">No workflow open</div>
				<div className="empty-subtitle">
					Open a workflow and run a dry-run to see the execution plan.
				</div>
			</div>
		);
	}

	if (!state.dryRun) {
		return (
			<div className="empty-state">
				<div className="empty-icon">🧪</div>
				<div className="empty-title">Dry-run not executed</div>
				<div className="empty-subtitle">
					Click "Dry-run" in the sidebar to generate the execution plan.
				</div>
			</div>
		);
	}

	const dr = state.dryRun;

	return (
		<div className="dryrun-pane">
			<div className="dryrun-header">
				<h2 className="dryrun-title">{dr.workflow}</h2>
				<span className={`dryrun-badge ${dr.valid ? "valid" : "invalid"}`}>
					{dr.valid ? "Valid" : "Invalid"}
				</span>
			</div>

			{dr.errors && dr.errors.length > 0 && (
				<div className="error-list">
					{dr.errors.map((e, i) => (
						<div key={i} className="error-item">
							<strong>[{e.code}]</strong> {e.message}
							{e.context && (
								<span className="error-context"> — {e.context}</span>
							)}
						</div>
					))}
				</div>
			)}

			<div className="dryrun-section">
				<h3 className="dryrun-section-title">Resolved Inputs</h3>
				<pre className="code-block">{JSON.stringify(dr.inputs, null, 2)}</pre>
			</div>

			<div className="dryrun-section">
				<h3 className="dryrun-section-title">Execution Order</h3>
				<ol className="dryrun-order">
					{dr.order.map((id) => (
						<li key={id} className="dryrun-order-item">
							<code>{id}</code>
							{dr.nodes[id] && (
								<span className="dryrun-kind">{dr.nodes[id].kind}</span>
							)}
						</li>
					))}
				</ol>
			</div>

			<div className="dryrun-section">
				<h3 className="dryrun-section-title">Node Plan</h3>
				<div className="dryrun-nodes">
					{Object.values(dr.nodes).map((n) => (
						<div key={n.id} className="dryrun-node">
							<div className="dryrun-node-header">
								<strong>{n.id}</strong>
								<span className="dryrun-kind">{n.kind}</span>
							</div>
							{n.dependencies.length > 0 && (
								<div className="dryrun-node-meta">
									depends on: {n.dependencies.join(", ")}
								</div>
							)}
							{n.dependents.length > 0 && (
								<div className="dryrun-node-meta">
									dependents: {n.dependents.join(", ")}
								</div>
							)}
						</div>
					))}
				</div>
			</div>
		</div>
	);
}
