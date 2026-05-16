import { useState } from "react";
import type { WorkspaceState, View } from "../state/workspace.js";

interface SidebarProps {
	state: WorkspaceState;
	onOpenPath: (path: string) => void;
	onSetView: (view: View) => void;
	onValidate: () => void;
	onGraph: () => void;
	onDryRun: () => void;
	onSave: () => void;
	onRun: () => void;
	onShowRuns: () => void;
	onShowSettings: () => void;
	runsCount: number;
	runError: string | null;
}

export function Sidebar({
	state,
	onOpenPath,
	onSetView,
	onValidate,
	onGraph,
	onDryRun,
	onSave,
	onRun,
	onShowRuns,
	onShowSettings,
	runsCount,
}: SidebarProps) {
	const [pathInput, setPathInput] = useState("");

	const handleOpen = () => {
		const p = pathInput.trim();
		if (p) onOpenPath(p);
	};

	const views: { key: View; label: string }[] = [
		{ key: "editor", label: "Editor" },
		{ key: "graph", label: "Graph" },
		{ key: "dry-run", label: "Dry-run" },
		{ key: "runs", label: `Runs${runsCount > 0 ? ` (${runsCount})` : ""}` },
		{ key: "settings", label: "Settings" },
	];

	return (
		<aside className="sidebar">
			<div className="sidebar-section">
				<h3 className="sidebar-heading">Open Workflow</h3>
				<div className="sidebar-row">
					<input
						className="sidebar-input"
						value={pathInput}
						onChange={(e) => setPathInput(e.target.value)}
						placeholder="Path to .yaml"
						onKeyDown={(e) => e.key === "Enter" && handleOpen()}
					/>
					<button
						className="sidebar-btn"
						onClick={handleOpen}
						disabled={state.isLoading}
					>
						Open
					</button>
				</div>
			</div>

			<div className="sidebar-section">
				<h3 className="sidebar-heading">Views</h3>
				<div className="sidebar-list">
					{views.map((v) => (
						<button
							key={v.key}
							className={`sidebar-item ${state.view === v.key ? "active" : ""}`}
							onClick={() => onSetView(v.key)}
						>
							{v.label}
						</button>
					))}
				</div>
			</div>

			<div className="sidebar-section">
				<h3 className="sidebar-heading">Actions</h3>
				<div className="sidebar-actions">
					<button
						className="action-btn"
						onClick={onSave}
						disabled={!state.dirty || state.isLoading}
					>
						Save
					</button>
					<button
						className="action-btn"
						onClick={onValidate}
						disabled={!state.currentPath || state.isLoading}
					>
						Validate
					</button>
					<button
						className="action-btn"
						onClick={onGraph}
						disabled={!state.currentPath || state.isLoading}
					>
						Graph
					</button>
					<button
						className="action-btn"
						onClick={onDryRun}
						disabled={!state.currentPath || state.isLoading}
					>
						Dry-run
					</button>
					<button
						className="action-btn run-btn"
						onClick={onRun}
						disabled={!state.currentPath || state.isLoading}
					>
						Run
					</button>
				</div>
			</div>

			<div className="sidebar-section">
				<h3 className="sidebar-heading">Recent</h3>
				<div className="sidebar-list">
					{state.recentFiles.length === 0 && (
						<div className="sidebar-empty">No recent workflows</div>
					)}
					{state.recentFiles.map((path) => (
						<button
							key={path}
							className={`sidebar-item ${state.currentPath === path ? "active" : ""}`}
							title={path}
							onClick={() => onOpenPath(path)}
						>
							<span className="sidebar-item-name">
								{path.split("/").pop() ?? path}
							</span>
							<span className="sidebar-item-path">{path}</span>
						</button>
					))}
				</div>
			</div>
		</aside>
	);
}
