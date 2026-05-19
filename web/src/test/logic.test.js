import { describe, expect, test } from "vitest";

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
