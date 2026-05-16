import type { WorkspaceState } from "../state/workspace.js";
import type { RunsState } from "../state/runs.js";

interface StatusBarProps {
	state: WorkspaceState;
	runsState?: RunsState;
}

export function StatusBar({ state, runsState }: StatusBarProps) {
	const items: string[] = [];
	if (state.currentPath) {
		items.push(state.currentPath);
	}
	if (state.dirty) {
		items.push("Modified");
	}
	if (state.validation) {
		items.push(
			state.validation.valid
				? "Valid"
				: `Invalid (${state.validation.errors?.length ?? 0} errors)`,
		);
	}
	if (state.isLoading) {
		items.push("Loading…");
	}

	const runItems: string[] = [];
	if (runsState) {
		const active = runsState.runs.filter(
			(r) =>
				r.status === "running" ||
				r.status === "validating" ||
				r.status === "planned",
		);
		if (active.length > 0) {
			runItems.push(
				`${active.length} active run${active.length > 1 ? "s" : ""}`,
			);
		}
		if (runsState.isLoading) {
			runItems.push("Loading runs…");
		}
	}

	return (
		<div className="status-bar">
			<div className="status-left">{items.join("  ·  ")}</div>
			<div className="status-right">
				{runItems.length > 0 && <span>{runItems.join("  ·  ")}</span>}
				{state.workflow && (
					<span>
						{state.workflow.nodes.length} nodes
						{state.workflow.inputs &&
						Object.keys(state.workflow.inputs).length > 0
							? ` · ${Object.keys(state.workflow.inputs).length} inputs`
							: ""}
					</span>
				)}
			</div>
		</div>
	);
}
