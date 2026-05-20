import { describe, expect, test } from "vitest";
import { groupSessionsByProject, uniqueProjectName } from "../lib/project-tree";

function parseWorkflowNodes(yaml) {
	const nodes = [];
	const lines = yaml.split("\n");
	let inNodes = false;
	let current = {};
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
			if (current.id) nodes.push(current);
			const m = t.match(/id:\s*([\w-]+)/);
			current = { id: m?.[1] || "" };
		} else if (/^kind:/.test(t)) {
			current.kind = t.split(":")[1].trim();
		} else if (/^name:/.test(t)) {
			current.name = t.split(":")[1].trim();
		} else if (/^depends_on:/.test(t)) {
			current.depends_on = [];
		} else if (/^-\s+([\w-]+)/.test(t) && Array.isArray(current.depends_on)) {
			const m = t.match(/^-\s+([\w-]+)/);
			if (m) current.depends_on.push(m[1]);
		}
	}

	if (current.id) nodes.push(current);
	return nodes;
}

describe("parseWorkflowNodes", () => {
	test("extracts nodes and dependencies from workflow yaml", () => {
		const yaml =
			"nodes:\n  - id: start\n    kind: agent\n    name: Start Agent\n  - id: end\n    kind: bash\n    depends_on:\n      - start\n";

		const nodes = parseWorkflowNodes(yaml);

		expect(nodes).toHaveLength(2);
		expect(nodes[0]).toMatchObject({
			id: "start",
			kind: "agent",
			name: "Start Agent",
		});
		expect(nodes[1]).toMatchObject({
			id: "end",
			kind: "bash",
			depends_on: ["start"],
		});
	});

	test("returns no nodes when yaml has no nodes section", () => {
		expect(parseWorkflowNodes("name:test")).toHaveLength(0);
	});
});

describe("project rail helpers", () => {
	test("groups chat sessions under their projects", () => {
		const projects = [
			{ name: "alpha", path: "/alpha" },
			{ name: "beta", path: "/beta" },
		];
		const sessions = [
			{
				id: "s1",
				project_name: "alpha",
				updated_at: "2026-05-19T10:00:00Z",
			},
			{
				id: "s2",
				project_name: "beta",
				updated_at: "2026-05-19T11:00:00Z",
			},
			{
				id: "s3",
				project_name: "alpha",
				updated_at: "2026-05-19T12:00:00Z",
			},
		];

		const grouped = groupSessionsByProject(projects, sessions);

		expect(grouped.alpha.map((s) => s.id)).toEqual(["s3", "s1"]);
		expect(grouped.beta.map((s) => s.id)).toEqual(["s2"]);
	});

	test("derives a conflict-free project name", () => {
		const projects = [
			{ name: "agentflow", path: "/repo/agentflow" },
			{ name: "agentflow-2", path: "/repo/agentflow-2" },
		];

		expect(uniqueProjectName("agentflow", projects)).toBe("agentflow-3");
		expect(uniqueProjectName("new", projects)).toBe("new");
	});
});
