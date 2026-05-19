import { Suspense, lazy, useState } from "react";
import { YamlEditor } from "./yaml-editor";

const GraphEditor = lazy(() => import("./graph-editor"));

export function WorkflowView() {
	const [view, setView] = useState<"yaml" | "graph">("yaml");
	const [yaml, setYaml] = useState(defaultWorkflow);

	return (
		<div className="h-full flex flex-col">
			<div className="px-4 py-2 border-b border-neutral-100 dark:border-neutral-800 flex items-center gap-2 shrink-0">
				<h2 className="text-sm font-semibold">Workflow Editor</h2>
				<div className="ml-auto flex rounded border border-neutral-200 dark:border-neutral-700 overflow-hidden">
					<button
						type="button"
						onClick={() => setView("yaml")}
						className={
							view === "yaml"
								? "px-3 py-1 text-xs bg-neutral-900 text-white"
								: "px-3 py-1 text-xs hover:bg-neutral-50 dark:hover:bg-neutral-800"
						}
					>
						YAML
					</button>
					<button
						type="button"
						onClick={() => setView("graph")}
						className={
							view === "graph"
								? "px-3 py-1 text-xs bg-neutral-900 text-white"
								: "px-3 py-1 text-xs hover:bg-neutral-50 dark:hover:bg-neutral-800"
						}
					>
						Graph
					</button>
				</div>
			</div>
			<div className="flex-1 overflow-hidden">
				{view === "yaml" ? (
					<YamlEditor value={yaml} onChange={setYaml} />
				) : (
					<Suspense
						fallback={
							<div className="p-8 text-sm text-neutral-400">
								Loading graph editor...
							</div>
						}
					>
						<GraphEditor yaml={yaml} />
					</Suspense>
				)}
			</div>
		</div>
	);
}

const defaultWorkflow = `name: example
inputs:
  query:
    type: string
    required: true

vars:
  model: gpt-4o

execution:
  max_concurrency: 4

nodes:
  - id: start
    kind: agent
    name: Start Agent
    provider: claude
    model: "\${vars.model}"
    prompt: "Process: \${inputs.query}"

  - id: transform
    kind: transform
    name: Transform Result
    input: "\${start.output}"
    script: |
      return { result: input.toUpperCase() };

  - id: end
    kind: bash
    name: Save Output
    command: echo "Result: \${transform.result}"
    depends_on:
      - transform
`;
