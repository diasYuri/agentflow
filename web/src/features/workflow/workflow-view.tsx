import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { api } from "@/lib/api";
import { cn, formatDate, formatTime } from "@/lib/utils";
import type {
	WorkflowDefinition,
	WorkflowDefinitionSummary,
	WorkflowInputSpec,
	WorkflowOutputSpec,
} from "@/types";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	Bot,
	FileCode2,
	Play,
	Plus,
	Save,
	Trash2,
	Workflow,
} from "lucide-react";
import {
	type ReactNode,
	Suspense,
	lazy,
	useEffect,
	useMemo,
	useState,
} from "react";
import { YamlEditor } from "./yaml-editor";

const GraphEditor = lazy(() => import("./graph-editor"));

type ViewMode = "list" | "create";
type EditorView = "yaml" | "graph";

export function WorkflowView() {
	const queryClient = useQueryClient();
	const [mode, setMode] = useState<ViewMode>("list");
	const [selectedId, setSelectedId] = useState<string | null>(null);
	const [editorView, setEditorView] = useState<EditorView>("yaml");
	const [yaml, setYaml] = useState(defaultWorkflow);
	const [error, setError] = useState<string | null>(null);

	const { data: workflows = [], isLoading } = useQuery({
		queryKey: ["workflow-definitions"],
		queryFn: api.workflows.list,
	});
	const selected = useMemo(
		() => workflows.find((workflow) => workflow.id === selectedId) ?? null,
		[workflows, selectedId],
	);
	const { data: selectedDetail, isLoading: isLoadingDetail } = useQuery({
		queryKey: ["workflow-definition", selectedId],
		queryFn: () => api.workflows.get(selectedId ?? ""),
		enabled: mode === "list" && selectedId !== null,
	});

	const createWorkflow = useMutation({
		mutationFn: api.workflows.createFromYaml,
		onSuccess: (workflow) => {
			queryClient.invalidateQueries({ queryKey: ["workflow-definitions"] });
			setSelectedId(workflow.id);
			setMode("list");
			setError(null);
		},
		onError: (err) => setError(String(err)),
	});

	const deleteWorkflow = useMutation({
		mutationFn: api.workflows.delete,
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["workflow-definitions"] });
			setSelectedId(null);
			setError(null);
		},
		onError: (err) => setError(String(err)),
	});

	const startCreate = () => {
		setMode("create");
		setSelectedId(null);
		setYaml(defaultWorkflow);
		setError(null);
	};

	return (
		<div className="flex h-full flex-col overflow-hidden px-6 pb-6 pt-20">
			<div className="mx-auto flex min-h-0 w-full max-w-6xl flex-1 flex-col">
				<header className="mb-5 flex flex-wrap items-center gap-3">
					<div className="flex size-10 items-center justify-center rounded-2xl border border-border bg-background/70 shadow-xl shadow-black/10">
						<Workflow className="size-5" />
					</div>
					<div className="min-w-0">
						<h1 className="text-xl font-medium tracking-tight">Workflows</h1>
						<p className="text-sm text-muted-foreground">
							Global workflow definitions available to AgentFlow.
						</p>
					</div>
					<Button onClick={startCreate} className="ml-auto rounded-2xl">
						<Plus className="size-4" />
						New workflow
					</Button>
				</header>

				<div className="grid min-h-0 flex-1 gap-4 lg:grid-cols-[320px_1fr]">
					<section className="min-h-0 overflow-hidden rounded-[24px] border border-border/80 bg-card/80 shadow-2xl shadow-black/10 backdrop-blur-xl">
						<div className="border-b border-border/70 px-4 py-3">
							<div className="text-sm font-medium">Available workflows</div>
							<div className="text-xs text-muted-foreground">
								{isLoading ? "Loading" : `${workflows.length} definitions`}
							</div>
						</div>
						<div className="h-full overflow-auto pb-16">
							{workflows.map((workflow) => (
								<button
									type="button"
									key={workflow.id}
									onClick={() => {
										setMode("list");
										setSelectedId(workflow.id);
										setError(null);
									}}
									className={cn(
										"flex w-full items-start gap-3 border-b border-border/60 px-4 py-3 text-left transition-colors hover:bg-accent/60",
										selectedId === workflow.id &&
											"bg-accent/80 text-foreground",
									)}
								>
									<FileCode2 className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
									<span className="min-w-0 flex-1">
										<span className="block truncate text-sm font-medium">
											{workflow.name}
										</span>
										<span className="block truncate text-xs text-muted-foreground">
											{workflow.description || "No description"}
										</span>
										<span className="mt-1 block text-[11px] text-muted-foreground">
											Updated {formatDate(workflow.updated_at)}{" "}
											{formatTime(workflow.updated_at)}
										</span>
									</span>
									<Badge variant="secondary" className="rounded-lg text-[10px]">
										v{workflow.version || "1"}
									</Badge>
								</button>
							))}
							{!isLoading && workflows.length === 0 && (
								<div className="px-5 py-10 text-center text-sm text-muted-foreground">
									No workflows yet.
								</div>
							)}
						</div>
					</section>

					<section className="min-h-0 overflow-hidden rounded-[24px] border border-border/80 bg-card/80 shadow-2xl shadow-black/10 backdrop-blur-xl">
						{mode === "create" ? (
							<div className="flex h-full min-h-0 flex-col">
								<div className="flex items-center gap-2 border-b border-border/70 px-4 py-3">
									<div className="min-w-0 flex-1">
										<div className="text-sm font-medium">New workflow</div>
										<div className="text-xs text-muted-foreground">
											Create a global definition from YAML.
										</div>
									</div>
									<SegmentedControl
										view={editorView}
										onChange={setEditorView}
									/>
									<Button
										onClick={() => createWorkflow.mutate(yaml)}
										disabled={createWorkflow.isPending}
										className="rounded-xl"
									>
										<Save className="size-4" />
										Save
									</Button>
								</div>
								{error && (
									<div className="border-b border-amber-500/20 bg-amber-500/10 px-4 py-2 text-xs text-amber-300">
										{error}
									</div>
								)}
								<div className="min-h-0 flex-1">
									{editorView === "yaml" ? (
										<YamlEditor value={yaml} onChange={setYaml} />
									) : (
										<Suspense
											fallback={
												<div className="p-8 text-sm text-muted-foreground">
													Loading graph editor...
												</div>
											}
										>
											<GraphEditor yaml={yaml} />
										</Suspense>
									)}
								</div>
							</div>
						) : (
							<WorkflowDetail
								selected={selected}
								detail={selectedDetail ?? null}
								isLoadingDetail={isLoadingDetail}
								onDelete={(id) => deleteWorkflow.mutate(id)}
								isDeleting={deleteWorkflow.isPending}
								error={error}
							/>
						)}
					</section>
				</div>
			</div>
		</div>
	);
}

function WorkflowDetail({
	selected,
	detail,
	isLoadingDetail,
	onDelete,
	isDeleting,
	error,
}: {
	selected: WorkflowDefinitionSummary | null;
	detail: WorkflowDefinition | null;
	isLoadingDetail: boolean;
	onDelete: (id: string) => void;
	isDeleting: boolean;
	error: string | null;
}) {
	const queryClient = useQueryClient();
	const [inputValues, setInputValues] = useState<Record<string, string>>({});
	const [runError, setRunError] = useState<string | null>(null);
	const [startedRunId, setStartedRunId] = useState<string | null>(null);
	const inputs = useMemo(
		() => (detail?.inputs ?? {}) as Record<string, WorkflowInputSpec>,
		[detail],
	);
	const outputs = useMemo(
		() => (detail?.outputs ?? {}) as Record<string, WorkflowOutputSpec>,
		[detail],
	);

	useEffect(() => {
		if (!detail) {
			setInputValues({});
			setRunError(null);
			setStartedRunId(null);
			return;
		}
		setInputValues(defaultInputValues(inputs));
		setRunError(null);
		setStartedRunId(null);
	}, [detail, inputs]);

	const runWorkflow = useMutation({
		mutationFn: () => {
			if (!detail) throw new Error("Workflow definition is not loaded");
			return api.workflowRuns.run({
				workflow_ref: detail.id,
				inputs: coerceInputs(inputs, inputValues),
			});
		},
		onSuccess: (run) => {
			queryClient.invalidateQueries({ queryKey: ["workflow-runs"] });
			setStartedRunId(run.id);
			setRunError(null);
		},
		onError: (err) => setRunError(String(err)),
	});

	if (!selected) {
		return (
			<div className="flex h-full items-center justify-center px-6 text-center text-sm text-muted-foreground">
				Select a workflow or create a new global definition.
			</div>
		);
	}
	return (
		<div className="flex h-full flex-col">
			<div className="flex items-center gap-3 border-b border-border/70 px-5 py-4">
				<div className="flex size-10 items-center justify-center rounded-2xl bg-primary text-primary-foreground">
					<Bot className="size-5" />
				</div>
				<div className="min-w-0 flex-1">
					<h2 className="truncate text-lg font-medium">{selected.name}</h2>
					<p className="truncate text-sm text-muted-foreground">
						{selected.description || "No description"}
					</p>
				</div>
				<Button
					onClick={() => onDelete(selected.id)}
					disabled={isDeleting}
					variant="ghost"
					size="icon"
					className="rounded-xl text-muted-foreground hover:text-destructive"
					aria-label="Delete workflow"
				>
					<Trash2 className="size-4" />
				</Button>
			</div>
			{error && (
				<div className="border-b border-amber-500/20 bg-amber-500/10 px-4 py-2 text-xs text-amber-300">
					{error}
				</div>
			)}
			<div className="min-h-0 flex-1 overflow-auto p-5">
				<div className="grid gap-4 text-sm md:grid-cols-2">
					<Info label="Version" value={selected.version || "1"} />
					<Info
						label="Updated"
						value={`${formatDate(selected.updated_at)} ${formatTime(selected.updated_at)}`}
					/>
					<Info
						label="Created"
						value={`${formatDate(selected.created_at)} ${formatTime(selected.created_at)}`}
					/>
					<Info label="ID" value={selected.id} mono />
				</div>

				{isLoadingDetail ? (
					<div className="mt-6 text-sm text-muted-foreground">
						Loading definition...
					</div>
				) : detail ? (
					<div className="mt-6 grid gap-5 xl:grid-cols-[minmax(0,1fr)_minmax(320px,420px)]">
						<div className="space-y-5">
							<DefinitionSection title="Inputs">
								{Object.keys(inputs).length === 0 ? (
									<p className="text-sm text-muted-foreground">
										This workflow does not declare inputs.
									</p>
								) : (
									<div className="space-y-3">
										{Object.entries(inputs).map(([name, spec]) => (
											<InputField
												key={name}
												name={name}
												spec={spec}
												value={inputValues[name] ?? ""}
												onChange={(value) =>
													setInputValues((current) => ({
														...current,
														[name]: value,
													}))
												}
											/>
										))}
									</div>
								)}
								<div className="mt-4 flex flex-wrap items-center gap-3">
									<Button
										onClick={() => runWorkflow.mutate()}
										disabled={runWorkflow.isPending}
										className="rounded-xl"
									>
										<Play className="size-4" />
										Run workflow
									</Button>
									{startedRunId && (
										<span className="text-xs text-muted-foreground">
											Started run {startedRunId}
										</span>
									)}
								</div>
								{runError && (
									<div className="mt-3 border-l-2 border-destructive pl-3 text-xs text-destructive">
										{runError}
									</div>
								)}
							</DefinitionSection>

							<DefinitionSection title="Outputs">
								{Object.keys(outputs).length === 0 ? (
									<p className="text-sm text-muted-foreground">
										This workflow does not declare outputs.
									</p>
								) : (
									<div className="space-y-2">
										{Object.entries(outputs).map(([name, spec]) => (
											<div
												key={name}
												className="border-t border-border/70 pt-2 text-sm"
											>
												<div className="font-medium">{name}</div>
												<div className="text-xs text-muted-foreground">
													{spec.type || "any"}
												</div>
											</div>
										))}
									</div>
								)}
							</DefinitionSection>
						</div>

						<DefinitionSection title="Graph">
							<pre className="max-h-[420px] overflow-auto whitespace-pre-wrap rounded-lg border border-border/70 bg-background/60 p-3 font-mono text-xs text-muted-foreground">
								{detail.graph || "graph TD"}
							</pre>
						</DefinitionSection>
					</div>
				) : null}
			</div>
		</div>
	);
}

function DefinitionSection({
	title,
	children,
}: {
	title: string;
	children: ReactNode;
}) {
	return (
		<section>
			<h3 className="mb-3 text-sm font-medium">{title}</h3>
			{children}
		</section>
	);
}

function InputField({
	name,
	spec,
	value,
	onChange,
}: {
	name: string;
	spec: WorkflowInputSpec;
	value: string;
	onChange: (value: string) => void;
}) {
	const type = spec.type || schemaType(spec.schema) || "string";
	const isJSON = type === "object" || type === "array";
	const inputId = `workflow-input-${name}`;
	return (
		<label htmlFor={inputId} className="block border-t border-border/70 pt-3">
			<div className="mb-1 flex items-center gap-2">
				<span className="text-sm font-medium">{name}</span>
				<Badge variant="secondary" className="rounded-lg text-[10px]">
					{type}
				</Badge>
				{spec.required && (
					<span className="text-[11px] text-muted-foreground">required</span>
				)}
			</div>
			{isJSON ? (
				<Textarea
					id={inputId}
					value={value}
					onChange={(event) => onChange(event.target.value)}
					className="min-h-24 font-mono text-xs"
					placeholder={type === "array" ? "[]" : "{}"}
				/>
			) : (
				<Input
					id={inputId}
					value={value}
					type={type === "number" || type === "integer" ? "number" : "text"}
					onChange={(event) => onChange(event.target.value)}
				/>
			)}
		</label>
	);
}

function defaultInputValues(
	inputs: Record<string, WorkflowInputSpec>,
): Record<string, string> {
	return Object.fromEntries(
		Object.entries(inputs).map(([name, spec]) => {
			if (spec.default === undefined) return [name, ""];
			if (typeof spec.default === "string") return [name, spec.default];
			return [name, JSON.stringify(spec.default, null, 2)];
		}),
	);
}

function coerceInputs(
	inputs: Record<string, WorkflowInputSpec>,
	values: Record<string, string>,
): Record<string, unknown> {
	return Object.fromEntries(
		Object.entries(inputs)
			.map(([name, spec]) => [name, coerceInputValue(values[name] ?? "", spec)])
			.filter(([, value]) => value !== undefined),
	);
}

function coerceInputValue(value: string, spec: WorkflowInputSpec): unknown {
	const type = spec.type || schemaType(spec.schema) || "string";
	if (value === "" && !spec.required) return undefined;
	if (type === "number" || type === "integer") return Number(value);
	if (type === "boolean") return value === "true";
	if (type === "object" || type === "array") return JSON.parse(value);
	return value;
}

function schemaType(
	schema: Record<string, unknown> | undefined,
): string | undefined {
	const type = schema?.type;
	return typeof type === "string" ? type : undefined;
}

function Info({
	label,
	value,
	mono,
}: {
	label: string;
	value: string;
	mono?: boolean;
}) {
	return (
		<div className="border-t border-border/70 pt-3">
			<div className="text-xs text-muted-foreground">{label}</div>
			<div className={cn("mt-1 truncate", mono && "font-mono text-xs")}>
				{value}
			</div>
		</div>
	);
}

function SegmentedControl({
	view,
	onChange,
}: {
	view: EditorView;
	onChange: (view: EditorView) => void;
}) {
	return (
		<div className="flex overflow-hidden rounded-xl border border-border/70 bg-background/55 p-1">
			{(["yaml", "graph"] as EditorView[]).map((item) => (
				<button
					type="button"
					key={item}
					onClick={() => onChange(item)}
					className={cn(
						"rounded-lg px-3 py-1 text-xs capitalize transition-colors",
						view === item
							? "bg-primary text-primary-foreground"
							: "text-muted-foreground hover:bg-accent hover:text-foreground",
					)}
				>
					{item}
				</button>
			))}
		</div>
	);
}

const defaultWorkflow = `version: "1"
name: new-workflow
description: A reusable AgentFlow workflow.

inputs:
  query:
    type: string
    required: true

execution:
  max_concurrency: 4

nodes:
  - id: start
    kind: agent
    name: Start Agent
    provider: codex
    model: default
    prompt: "Process: \${inputs.query}"
`;
