import { useMemo } from "react";
import type { WorkspaceState } from "../state/workspace.js";

interface GraphPaneProps {
	state: WorkspaceState;
}

export function GraphPane({ state }: GraphPaneProps) {
	const svg = useMemo(() => {
		if (!state.workflow || state.workflow.nodes.length === 0) return null;

		const nodes = state.workflow.nodes;
		const nodeMap = new Map<string, number>();
		const levels: string[][] = [];
		const visited = new Set<string>();

		// simple level assignment via topological layering
		function levelOf(id: string): number {
			if (visited.has(id)) return nodeMap.get(id) ?? 0;
			visited.add(id);
			const node = nodes.find((n) => n.id === id);
			let max = -1;
			for (const dep of node?.dependsOn ?? []) {
				max = Math.max(max, levelOf(dep));
			}
			const lvl = max + 1;
			nodeMap.set(id, lvl);
			if (!levels[lvl]) levels[lvl] = [];
			levels[lvl].push(id);
			return lvl;
		}

		for (const n of nodes) levelOf(n.id);

		const colWidth = 180;
		const rowHeight = 64;
		const padX = 40;
		const padY = 40;
		const nodeW = 140;
		const nodeH = 40;

		const positions = new Map<string, { x: number; y: number }>();
		levels.forEach((col, ci) => {
			col.forEach((id, ri) => {
				positions.set(id, {
					x: padX + ci * colWidth,
					y: padY + ri * rowHeight + (levels.length === 1 ? 0 : 0),
				});
			});
		});

		const width = Math.max(400, levels.length * colWidth + padX * 2);
		const height = Math.max(
			300,
			Math.max(...levels.map((c) => c.length)) * rowHeight + padY * 2,
		);

		const lines: JSX.Element[] = [];
		for (const n of nodes) {
			const from = positions.get(n.id);
			if (!from) continue;
			for (const dep of n.dependsOn ?? []) {
				const to = positions.get(dep);
				if (!to) continue;
				lines.push(
					<line
						key={`${dep}-${n.id}`}
						x1={to.x + nodeW}
						y1={to.y + nodeH / 2}
						x2={from.x}
						y2={from.y + nodeH / 2}
						stroke="rgba(0,0,0,0.15)"
						strokeWidth={2}
					/>,
				);
			}
		}

		const rects: JSX.Element[] = [];
		for (const n of nodes) {
			const pos = positions.get(n.id);
			if (!pos) continue;
			rects.push(
				<g key={n.id}>
					<rect
						x={pos.x}
						y={pos.y}
						width={nodeW}
						height={nodeH}
						rx={8}
						fill="rgba(255,255,255,0.9)"
						stroke="rgba(0,0,0,0.08)"
						strokeWidth={1}
					/>
					<text
						x={pos.x + nodeW / 2}
						y={pos.y + nodeH / 2}
						dominantBaseline="middle"
						textAnchor="middle"
						fill="#1d1d1f"
						fontSize={12}
						fontWeight={500}
					>
						{n.id}
					</text>
					<text
						x={pos.x + nodeW / 2}
						y={pos.y + nodeH + 14}
						dominantBaseline="middle"
						textAnchor="middle"
						fill="#6e6e73"
						fontSize={10}
					>
						{n.kind}
					</text>
				</g>,
			);
		}

		return (
			<svg
				width={width}
				height={height}
				style={{ minWidth: width, minHeight: height }}
			>
				<rect width={width} height={height} fill="transparent" />
				{lines}
				{rects}
			</svg>
		);
	}, [state.workflow]);

	if (!state.workflow) {
		return (
			<div className="empty-state">
				<div className="empty-icon">🕸️</div>
				<div className="empty-title">No workflow open</div>
				<div className="empty-subtitle">
					Open a workflow to visualize its graph.
				</div>
			</div>
		);
	}

	return (
		<div className="graph-pane">
			{state.graph?.valid && (
				<div className="graph-mermaid">
					<pre className="mermaid-code">{state.graph.mermaid}</pre>
				</div>
			)}
			<div className="graph-visual">{svg}</div>
		</div>
	);
}
