import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { api } from "@/lib/api";
import { cn, formatDate, formatTime } from "@/lib/utils";
import type {
	WorkflowArtifact,
	WorkflowInspect,
	WorkflowNodeResult,
	WorkflowRun,
	WorkflowRunStatus,
	WorkflowTimelineEntry,
} from "@/types";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	type ColumnDef,
	flexRender,
	getCoreRowModel,
	getFilteredRowModel,
	getSortedRowModel,
	useReactTable,
} from "@tanstack/react-table";
import {
	Activity,
	AlertTriangle,
	BarChart3,
	Clock,
	FileArchive,
	Loader2,
	type LucideIcon,
	Pause,
	Play,
	ShieldCheck,
	Square,
	ThumbsDown,
	ThumbsUp,
	Timer,
	XCircle,
} from "lucide-react";
import type React from "react";
import { useEffect, useMemo, useState } from "react";
import {
	Area,
	AreaChart,
	Bar,
	BarChart,
	Cell,
	Pie,
	PieChart,
	ResponsiveContainer,
	Tooltip,
	XAxis,
	YAxis,
} from "recharts";
import {
	type WorkflowRunAction,
	availableRunActions,
	buildDashboardMetrics,
	formatDuration,
	runDurationMs,
} from "./dashboard-logic";

const statusColors: Record<string, string> = {
	success: "#16a34a",
	failed: "#dc2626",
	cancelled: "#737373",
	running: "#2563eb",
	planned: "#0891b2",
	validating: "#7c3aed",
	created: "#64748b",
	paused: "#d97706",
	wait_approval: "#ca8a04",
};

export function DashboardView() {
	const queryClient = useQueryClient();
	const [selectedRunId, setSelectedRunId] = useState<string | null>(null);
	const [globalFilter, setGlobalFilter] = useState("");
	const [statusFilter, setStatusFilter] = useState<WorkflowRunStatus | "all">(
		"all",
	);

	const {
		data: runs = [],
		isLoading,
		error,
	} = useQuery({
		queryKey: ["workflow-runs"],
		queryFn: api.workflowRuns.list,
		refetchInterval: 5_000,
	});

	useEffect(() => {
		if (!selectedRunId && runs.length > 0) setSelectedRunId(runs[0].id);
	}, [runs, selectedRunId]);

	const selectedRun = runs.find((run) => run.id === selectedRunId) ?? null;
	const filteredRuns = useMemo(
		() =>
			statusFilter === "all"
				? runs
				: runs.filter((run) => run.status === statusFilter),
		[runs, statusFilter],
	);
	const metrics = useMemo(() => buildDashboardMetrics(runs), [runs]);

	const runAction = useMutation({
		mutationFn: ({
			runId,
			action,
		}: {
			runId: string;
			action: WorkflowRunAction;
		}) => api.workflowRuns.action(runId, action),
		onSuccess: (run) => {
			queryClient.invalidateQueries({ queryKey: ["workflow-runs"] });
			queryClient.invalidateQueries({ queryKey: ["workflow-run", run.id] });
		},
	});

	const columns = useMemo<ColumnDef<WorkflowRun>[]>(
		() => [
			{
				accessorKey: "workflow",
				header: "Workflow",
				cell: ({ row }) => (
					<div className="min-w-0">
						<div className="truncate font-medium">{row.original.workflow}</div>
						<div className="truncate font-mono text-[11px] text-muted-foreground">
							{row.original.id}
						</div>
					</div>
				),
			},
			{
				accessorKey: "status",
				header: "Status",
				cell: ({ row }) => <StatusBadge status={row.original.status} />,
			},
			{
				accessorKey: "current_step",
				header: "Step",
				cell: ({ row }) => row.original.current_step || "-",
			},
			{
				accessorKey: "started_at",
				header: "Started",
				cell: ({ row }) =>
					`${formatDate(row.original.started_at)} ${formatTime(row.original.started_at)}`,
			},
			{
				id: "duration",
				header: "Duration",
				cell: ({ row }) => formatDuration(runDurationMs(row.original)),
			},
		],
		[],
	);

	const table = useReactTable({
		data: filteredRuns,
		columns,
		state: { globalFilter },
		onGlobalFilterChange: setGlobalFilter,
		getCoreRowModel: getCoreRowModel(),
		getFilteredRowModel: getFilteredRowModel(),
		getSortedRowModel: getSortedRowModel(),
	});

	return (
		<div className="flex h-full flex-col overflow-hidden px-6 pb-6 pt-20">
			<div className="mx-auto flex min-h-0 w-full max-w-7xl flex-1 flex-col gap-4">
				<header className="flex flex-wrap items-center gap-3">
					<div className="flex size-10 items-center justify-center rounded-2xl border border-border bg-background/70 shadow-xl shadow-black/10">
						<BarChart3 className="size-5" />
					</div>
					<div className="min-w-0">
						<h1 className="text-xl font-medium tracking-tight">
							Workflow dashboard
						</h1>
						<p className="text-sm text-muted-foreground">
							Executive view of workflow executions, health, and decisions.
						</p>
					</div>
					<div className="ml-auto flex items-center gap-2">
						<Badge variant="secondary" className="rounded-lg">
							{isLoading ? "Loading" : `${runs.length} runs`}
						</Badge>
					</div>
				</header>

				{error && (
					<div className="rounded-2xl border border-amber-500/20 bg-amber-500/10 px-4 py-3 text-sm text-amber-300">
						Workflow runs require agentflowd. Start it with{" "}
						<span className="font-mono">agentflow daemon start</span>.
					</div>
				)}

				<div className="grid gap-3 md:grid-cols-2 xl:grid-cols-6">
					<Kpi icon={Activity} label="Active" value={metrics.active} />
					<Kpi
						icon={ShieldCheck}
						label="Success rate"
						value={`${Math.round(metrics.successRate * 100)}%`}
					/>
					<Kpi
						icon={AlertTriangle}
						label="Needs action"
						value={metrics.waiting}
						tone="warning"
					/>
					<Kpi
						icon={XCircle}
						label="Failures"
						value={metrics.failed}
						tone="danger"
					/>
					<Kpi
						icon={Timer}
						label="Avg duration"
						value={formatDuration(metrics.avgDurationMs)}
					/>
					<Kpi icon={FileArchive} label="Artifacts" value={metrics.artifacts} />
				</div>

				<div className="grid min-h-0 flex-1 gap-4 xl:grid-cols-[minmax(0,1fr)_420px]">
					<section className="flex min-h-0 flex-col gap-4 overflow-hidden">
						<div className="grid gap-4 lg:grid-cols-3">
							<ChartPanel title="Run trend" className="lg:col-span-2">
								<ResponsiveContainer width="100%" height={190}>
									<AreaChart data={metrics.trend}>
										<XAxis dataKey="day" hide />
										<YAxis allowDecimals={false} width={24} />
										<Tooltip />
										<Area
											type="monotone"
											dataKey="runs"
											stroke="#2563eb"
											fill="#2563eb"
											fillOpacity={0.18}
										/>
										<Area
											type="monotone"
											dataKey="failures"
											stroke="#dc2626"
											fill="#dc2626"
											fillOpacity={0.12}
										/>
									</AreaChart>
								</ResponsiveContainer>
							</ChartPanel>
							<ChartPanel title="Status mix">
								<ResponsiveContainer width="100%" height={190}>
									<PieChart>
										<Pie
											data={metrics.statusCounts}
											dataKey="count"
											nameKey="status"
											innerRadius={46}
											outerRadius={74}
											paddingAngle={3}
										>
											{metrics.statusCounts.map((item) => (
												<Cell
													key={item.status}
													fill={statusColors[item.status] ?? "#737373"}
												/>
											))}
										</Pie>
										<Tooltip />
									</PieChart>
								</ResponsiveContainer>
							</ChartPanel>
						</div>

						<ChartPanel title="Top workflows">
							<ResponsiveContainer width="100%" height={170}>
								<BarChart data={metrics.workflowCounts}>
									<XAxis dataKey="workflow" hide />
									<YAxis allowDecimals={false} width={24} />
									<Tooltip />
									<Bar dataKey="count" fill="#2563eb" radius={[6, 6, 0, 0]} />
								</BarChart>
							</ResponsiveContainer>
						</ChartPanel>

						<section className="min-h-0 overflow-hidden rounded-[24px] border border-border/80 bg-card/80 shadow-2xl shadow-black/10 backdrop-blur-xl">
							<div className="flex flex-wrap items-center gap-2 border-b border-border/70 px-4 py-3">
								<div className="mr-auto">
									<div className="text-sm font-medium">Execution history</div>
									<div className="text-xs text-muted-foreground">
										Sorted by daemon history, refreshed every 5 seconds.
									</div>
								</div>
								<input
									value={globalFilter}
									onChange={(event) => setGlobalFilter(event.target.value)}
									placeholder="Filter runs"
									className="h-9 w-48 rounded-xl border border-border/70 bg-background/70 px-3 text-sm outline-none focus:border-ring"
								/>
								<select
									value={statusFilter}
									onChange={(event) =>
										setStatusFilter(
											event.target.value as WorkflowRunStatus | "all",
										)
									}
									className="h-9 rounded-xl border border-border/70 bg-background/70 px-2 text-sm outline-none focus:border-ring"
									aria-label="Status filter"
								>
									<option value="all">All status</option>
									{metrics.statusCounts.map((item) => (
										<option key={item.status} value={item.status}>
											{item.status}
										</option>
									))}
								</select>
							</div>
							<div className="min-h-0 overflow-auto">
								<table className="w-full text-sm">
									<thead className="sticky top-0 bg-card text-xs text-muted-foreground">
										{table.getHeaderGroups().map((headerGroup) => (
											<tr
												key={headerGroup.id}
												className="border-b border-border/70"
											>
												{headerGroup.headers.map((header) => (
													<th
														key={header.id}
														className="px-4 py-2 text-left font-medium"
													>
														{flexRender(
															header.column.columnDef.header,
															header.getContext(),
														)}
													</th>
												))}
											</tr>
										))}
									</thead>
									<tbody>
										{table.getRowModel().rows.map((row) => (
											<tr
												key={row.id}
												onClick={() => setSelectedRunId(row.original.id)}
												onKeyDown={(event) => {
													if (event.key === "Enter" || event.key === " ") {
														event.preventDefault();
														setSelectedRunId(row.original.id);
													}
												}}
												tabIndex={0}
												className={cn(
													"cursor-pointer border-b border-border/50 transition-colors hover:bg-accent/60 focus:bg-accent/60 focus:outline-none",
													selectedRunId === row.original.id && "bg-accent/70",
												)}
											>
												{row.getVisibleCells().map((cell) => (
													<td key={cell.id} className="px-4 py-3">
														{flexRender(
															cell.column.columnDef.cell,
															cell.getContext(),
														)}
													</td>
												))}
											</tr>
										))}
									</tbody>
								</table>
								{!isLoading && table.getRowModel().rows.length === 0 && (
									<div className="px-5 py-10 text-center text-sm text-muted-foreground">
										No workflow runs match the current filters.
									</div>
								)}
							</div>
						</section>
					</section>

					<RunDetail
						run={selectedRun}
						onAction={(action) => {
							if (selectedRun)
								runAction.mutate({ runId: selectedRun.id, action });
						}}
						isActing={runAction.isPending}
					/>
				</div>
			</div>
		</div>
	);
}

function RunDetail({
	run,
	onAction,
	isActing,
}: {
	run: WorkflowRun | null;
	onAction: (action: WorkflowRunAction) => void;
	isActing: boolean;
}) {
	const { data: inspect } = useQuery({
		queryKey: ["workflow-run", run?.id, "inspect"],
		queryFn: () => api.workflowRuns.inspect(run?.id ?? ""),
		enabled: Boolean(run?.id),
		refetchInterval: 5_000,
	});
	const { data: nodes = [] } = useQuery({
		queryKey: ["workflow-run", run?.id, "nodes"],
		queryFn: () => api.workflowRuns.nodes(run?.id ?? ""),
		enabled: Boolean(run?.id),
		refetchInterval: 5_000,
	});
	const { data: timeline } = useQuery({
		queryKey: ["workflow-run", run?.id, "timeline"],
		queryFn: () => api.workflowRuns.timeline(run?.id ?? "", 0, 20),
		enabled: Boolean(run?.id),
		refetchInterval: 5_000,
	});
	const { data: artifacts = [] } = useQuery({
		queryKey: ["workflow-run", run?.id, "artifacts"],
		queryFn: () => api.workflowRuns.artifacts(run?.id ?? ""),
		enabled: Boolean(run?.id),
		refetchInterval: 10_000,
	});

	if (!run) {
		return (
			<aside className="flex min-h-0 items-center justify-center rounded-[24px] border border-border/80 bg-card/80 px-6 text-center text-sm text-muted-foreground">
				Select a run to inspect progress, metrics, timeline, nodes, and
				artifacts.
			</aside>
		);
	}

	const actions = availableRunActions(run.status);

	return (
		<aside className="flex min-h-0 flex-col overflow-hidden rounded-[24px] border border-border/80 bg-card/80 shadow-2xl shadow-black/10 backdrop-blur-xl">
			<div className="border-b border-border/70 px-5 py-4">
				<div className="flex items-start gap-3">
					<div className="flex size-10 shrink-0 items-center justify-center rounded-2xl bg-primary text-primary-foreground">
						<Clock className="size-5" />
					</div>
					<div className="min-w-0 flex-1">
						<h2 className="truncate text-lg font-medium">{run.workflow}</h2>
						<div className="mt-1 flex flex-wrap items-center gap-2">
							<StatusBadge status={run.status} />
							<span className="truncate font-mono text-[11px] text-muted-foreground">
								{run.id}
							</span>
						</div>
					</div>
				</div>
				{actions.length > 0 && (
					<div className="mt-4 flex flex-wrap gap-2">
						{actions.map((action) => (
							<Button
								key={action}
								onClick={() => onAction(action)}
								disabled={isActing}
								variant={
									action === "cancel" || action === "reject"
										? "ghost"
										: "default"
								}
								className="h-9 rounded-xl"
							>
								<ActionIcon action={action} />
								{actionLabel(action)}
							</Button>
						))}
					</div>
				)}
			</div>
			<div className="min-h-0 flex-1 overflow-auto px-5 py-4">
				<RunMetrics inspect={inspect} run={run} />
				{(run.approval_message || inspect?.approval_message) && (
					<div className="mt-4 rounded-2xl border border-amber-500/20 bg-amber-500/10 p-3 text-sm">
						<div className="font-medium text-amber-300">Approval required</div>
						<p className="mt-1 text-muted-foreground">
							{inspect?.approval_message ?? run.approval_message}
						</p>
					</div>
				)}
				<DetailSection title="Nodes">
					<NodeList nodes={nodes} />
				</DetailSection>
				<DetailSection title="Timeline">
					<Timeline entries={timeline?.entries ?? []} />
				</DetailSection>
				<DetailSection title="Artifacts">
					<ArtifactList artifacts={artifacts} />
				</DetailSection>
			</div>
		</aside>
	);
}

function RunMetrics({
	inspect,
	run,
}: {
	inspect?: WorkflowInspect;
	run: WorkflowRun;
}) {
	const completed =
		inspect?.completed_steps?.length ?? run.completed_steps?.length ?? 0;
	const total = inspect?.total_steps ?? run.total_steps ?? 0;
	const progress = total > 0 ? Math.round((completed / total) * 100) : 0;
	return (
		<div className="space-y-4">
			<div>
				<div className="mb-2 flex items-center justify-between text-xs text-muted-foreground">
					<span>
						{inspect?.current_step ?? run.current_step ?? "No active step"}
					</span>
					<span>{progress}%</span>
				</div>
				<div className="h-2 overflow-hidden rounded-full bg-muted">
					<div
						className="h-full rounded-full bg-primary transition-all"
						style={{ width: `${progress}%` }}
					/>
				</div>
			</div>
			<div className="grid grid-cols-2 gap-3 text-sm">
				<MiniMetric
					label="Duration"
					value={formatDuration(inspect?.duration_ms ?? runDurationMs(run))}
				/>
				<MiniMetric label="Nodes" value={`${completed}/${total || "-"}`} />
				<MiniMetric label="Failures" value={inspect?.failed_nodes ?? 0} />
				<MiniMetric label="Retries" value={inspect?.retries ?? 0} />
				<MiniMetric label="Agent calls" value={inspect?.agent_calls ?? 0} />
				<MiniMetric label="Artifacts" value={inspect?.artifact_count ?? 0} />
			</div>
			<ErrorSummary
				value={inspect?.first_error || inspect?.error || run.error}
			/>
		</div>
	);
}

function NodeList({ nodes }: { nodes: WorkflowNodeResult[] }) {
	if (nodes.length === 0) return <EmptyLine text="No node results yet." />;
	return (
		<div className="space-y-2">
			{nodes.map((node) => (
				<div
					key={`${node.node_id}:${node.instance_id ?? ""}`}
					className="rounded-2xl border border-border/70 p-3"
				>
					<div className="flex items-center gap-2">
						<StatusDot status={node.status} />
						<div className="min-w-0 flex-1 truncate text-sm font-medium">
							{node.node_id}
						</div>
						<div className="text-xs text-muted-foreground">
							{formatDuration(node.duration_ms ?? 0)}
						</div>
					</div>
					<ErrorSummary value={node.error} compact />
				</div>
			))}
		</div>
	);
}

function ErrorSummary({
	value,
	compact,
}: {
	value?: string;
	compact?: boolean;
}) {
	if (!value) return null;
	return (
		<div
			className={cn(
				"rounded-2xl border border-destructive/20 bg-destructive/10 text-destructive",
				compact ? "mt-2 p-2" : "p-3",
			)}
		>
			<div className="mb-1 text-[11px] font-medium uppercase tracking-wide">
				Error
			</div>
			<pre
				className={cn(
					"overflow-auto whitespace-pre-wrap break-all font-mono text-[11px] leading-relaxed",
					compact ? "max-h-24" : "max-h-36",
				)}
			>
				{value}
			</pre>
		</div>
	);
}

function Timeline({ entries }: { entries: WorkflowTimelineEntry[] }) {
	if (entries.length === 0)
		return <EmptyLine text="No timeline entries yet." />;
	return (
		<div className="space-y-3">
			{entries.map((entry, index) => (
				<div key={`${entry.ts}:${entry.type}:${index}`} className="flex gap-3">
					<div className="mt-1 size-2 rounded-full bg-muted-foreground" />
					<div className="min-w-0 flex-1">
						<div className="truncate text-sm">{entry.type}</div>
						<div className="text-xs text-muted-foreground">
							{formatDate(entry.ts)} {formatTime(entry.ts)}
							{entry.node_id ? ` · ${entry.node_id}` : ""}
						</div>
					</div>
				</div>
			))}
		</div>
	);
}

function ArtifactList({ artifacts }: { artifacts: WorkflowArtifact[] }) {
	if (artifacts.length === 0)
		return <EmptyLine text="No artifacts for this run." />;
	return (
		<div className="space-y-2">
			{artifacts.map((artifact) => (
				<div
					key={artifact.id}
					className="rounded-2xl border border-border/70 p-3"
				>
					<div className="truncate text-sm font-medium">{artifact.name}</div>
					<div className="mt-1 truncate text-xs text-muted-foreground">
						{artifact.kind || artifact.media_type || "artifact"} ·{" "}
						{artifact.node_id || "run"} ·{" "}
						{formatBytes(artifact.size_bytes || artifact.size)}
					</div>
				</div>
			))}
		</div>
	);
}

function Kpi({
	icon: Icon,
	label,
	value,
	tone,
}: {
	icon: LucideIcon;
	label: string;
	value: string | number;
	tone?: "warning" | "danger";
}) {
	return (
		<div className="rounded-[20px] border border-border/80 bg-card/80 p-4 shadow-xl shadow-black/5 backdrop-blur-xl">
			<div className="flex items-center justify-between">
				<div className="text-xs text-muted-foreground">{label}</div>
				<Icon
					className={cn(
						"size-4 text-muted-foreground",
						tone === "warning" && "text-amber-400",
						tone === "danger" && "text-destructive",
					)}
				/>
			</div>
			<div className="mt-2 text-2xl font-semibold tracking-tight">{value}</div>
		</div>
	);
}

function ChartPanel({
	title,
	children,
	className,
}: {
	title: string;
	children: React.ReactNode;
	className?: string;
}) {
	return (
		<section
			className={cn(
				"rounded-[24px] border border-border/80 bg-card/80 p-4 shadow-xl shadow-black/5 backdrop-blur-xl",
				className,
			)}
		>
			<div className="mb-3 text-sm font-medium">{title}</div>
			{children}
		</section>
	);
}

function DetailSection({
	title,
	children,
}: {
	title: string;
	children: React.ReactNode;
}) {
	return (
		<section className="mt-5">
			<h3 className="mb-2 text-sm font-medium">{title}</h3>
			{children}
		</section>
	);
}

function MiniMetric({
	label,
	value,
}: { label: string; value: string | number }) {
	return (
		<div className="rounded-2xl border border-border/70 p-3">
			<div className="text-xs text-muted-foreground">{label}</div>
			<div className="mt-1 font-medium">{value}</div>
		</div>
	);
}

function StatusBadge({ status }: { status: WorkflowRunStatus }) {
	return (
		<Badge
			variant="secondary"
			className="rounded-lg border"
			style={{
				borderColor: `${statusColors[status] ?? "#737373"}55`,
				color: statusColors[status] ?? undefined,
			}}
		>
			{status}
		</Badge>
	);
}

function StatusDot({ status }: { status: string }) {
	return (
		<span
			className="size-2 rounded-full"
			style={{ backgroundColor: statusColors[status] ?? "#737373" }}
		/>
	);
}

function ActionIcon({ action }: { action: WorkflowRunAction }) {
	if (action === "pause") return <Pause className="size-4" />;
	if (action === "resume") return <Play className="size-4" />;
	if (action === "approve") return <ThumbsUp className="size-4" />;
	if (action === "reject") return <ThumbsDown className="size-4" />;
	if (action === "cancel") return <Square className="size-4" />;
	return <Loader2 className="size-4" />;
}

function actionLabel(action: WorkflowRunAction): string {
	const labels: Record<WorkflowRunAction, string> = {
		pause: "Pause",
		cancel: "Cancel",
		resume: "Resume",
		approve: "Approve",
		reject: "Reject",
	};
	return labels[action];
}

function EmptyLine({ text }: { text: string }) {
	return <div className="py-3 text-sm text-muted-foreground">{text}</div>;
}

function formatBytes(bytes: number): string {
	if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
	if (bytes < 1024) return `${bytes} B`;
	const kb = bytes / 1024;
	if (kb < 1024) return `${kb.toFixed(1)} KB`;
	return `${(kb / 1024).toFixed(1)} MB`;
}
