import {
	Background,
	Controls,
	type Edge,
	type Node,
	ReactFlow,
} from "@xyflow/react";
import { useMemo } from "react";
import "@xyflow/react/dist/style.css";

interface Props {
	yaml: string;
}

interface WNode {
	id: string;
	kind: string;
	name?: string;
	depends_on?: string[];
}

function parseNodes(yaml: string): WNode[] {
	const nodes: WNode[] = [];
	const lines = yaml.split("\n");
	let inNodes = false;
	let current: Partial<WNode> = {};
	let indent = 0;

	for (const raw of lines) {
		const line = raw.replace(/#.*$/, "");
		if (!inNodes) {
			if (/^nodes:/.test(line)) {
				inNodes = true;
				indent = line.search(/\S/);
			}
			continue;
		}
		const li = line.search(/\S/);
		if (li <= indent && line.trim() !== "") {
			inNodes = false;
			continue;
		}
		const t = line.trim();
		if (/^-\s+id:/.test(t)) {
			if (current.id) nodes.push(current as WNode);
			const m = t.match(/id:\\s*([\w-]+)/);
			current = { id: m?.[1] ?? "" };
		} else if (/^kind:/.test(t)) {
			current.kind = t.split(":")[1].trim();
		} else if (/^name:/.test(t)) {
			current.name = t
				.split(":")[1]
				.trim()
				.replace(/^["']|["']$/g, "");
		} else if (/^depends_on:/.test(t)) {
			current.depends_on = [];
		} else if (/^-\s+([\w-]+)/.test(t) && Array.isArray(current.depends_on)) {
			const m = t.match(/^-\s*([\w-]+)/);
			if (m) current.depends_on.push(m[1]);
		}
	}
	if (current.id) nodes.push(current as WNode);
	return nodes;
}

export default function GraphEditor({ yaml }: Props) {
	const nodes = useMemo(() => parseNodes(yaml), [yaml]);

	const flowNodes: Node[] = useMemo(() => {
		return nodes.map((n, i) => ({
			id: n.id,
			position: { x: (i % 3) * 250 + 50, y: Math.floor(i / 3) * 120 + 50 },
			data: { label: n.name || n.id },
			style: {
				borderRadius: 6,
				border: "1px solid #e5e5e5",
				padding: "8px 12px",
				fontSize: 12,
				background:
					n.kind === "agent"
						? "#f0fdf4"
						: n.kind === "bash"
							? "#eff6ff"
							: "#fafafa",
			},
		}));
	}, [nodes]);

	const flowEdges: Edge[] = useMemo(() => {
		const edges: Edge[] = [];
		for (const n of nodes) {
			for (const dep of n.depends_on ?? []) {
				edges.push({ id: `${dep}-${n.id}`, source: dep, target: n.id });
			}
		}
		for (let i = 1; i < nodes.length; i++) {
			const prev = nodes[i - 1];
			const cur = nodes[i];
			if (!cur.depends_on || cur.depends_on.length === 0) {
				if (!edges.some((e) => e.target === cur.id)) {
					edges.push({
						id: `${prev.id}-${cur.id}`,
						source: prev.id,
						target: cur.id,
					});
				}
			}
		}
		return edges;
	}, [nodes]);

	if (nodes.length === 0) {
		return (
			<div className="p-8 text-sm text-neutral-400">
				No nodes found in workflow.
			</div>
		);
	}

	return (
		<div className="h-full">
			<ReactFlow nodes={flowNodes} edges={flowEdges} fitView>
				<Background />
				<Controls />
			</ReactFlow>
		</div>
	);
}
