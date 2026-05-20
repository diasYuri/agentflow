import type { WorkflowRun, WorkflowRunStatus } from "@/types";

export type WorkflowRunAction =
	| "pause"
	| "cancel"
	| "resume"
	| "approve"
	| "reject";

export interface DashboardMetrics {
	total: number;
	active: number;
	success: number;
	failed: number;
	waiting: number;
	terminal: number;
	successRate: number;
	avgDurationMs: number;
	artifacts: number;
	retries: number;
	statusCounts: Array<{ status: WorkflowRunStatus; count: number }>;
	workflowCounts: Array<{ workflow: string; count: number }>;
	trend: Array<{ day: string; runs: number; failures: number }>;
}

const activeStatuses = new Set<WorkflowRunStatus>([
	"created",
	"validating",
	"planned",
	"running",
]);
const terminalStatuses = new Set<WorkflowRunStatus>([
	"success",
	"failed",
	"cancelled",
]);

export function availableRunActions(
	status: WorkflowRunStatus,
): WorkflowRunAction[] {
	if (activeStatuses.has(status)) return ["pause", "cancel"];
	if (status === "paused") return ["resume", "cancel"];
	if (status === "wait_approval") return ["approve", "reject", "cancel"];
	return [];
}

export function isTerminalRun(status: WorkflowRunStatus): boolean {
	return terminalStatuses.has(status);
}

export function runDurationMs(run: WorkflowRun, now = new Date()): number {
	const started = Date.parse(run.started_at);
	if (!Number.isFinite(started)) return 0;
	const finished = run.finished_at
		? Date.parse(run.finished_at)
		: now.getTime();
	if (!Number.isFinite(finished) || finished < started) return 0;
	return finished - started;
}

export function buildDashboardMetrics(
	runs: WorkflowRun[],
	now = new Date(),
): DashboardMetrics {
	const statusMap = new Map<WorkflowRunStatus, number>();
	const workflowMap = new Map<string, number>();
	const trendMap = new Map<
		string,
		{ day: string; runs: number; failures: number }
	>();
	let active = 0;
	let success = 0;
	let failed = 0;
	let waiting = 0;
	let terminal = 0;
	let durationTotal = 0;
	let durationCount = 0;
	let artifacts = 0;
	let retries = 0;

	for (const run of runs) {
		statusMap.set(run.status, (statusMap.get(run.status) ?? 0) + 1);
		workflowMap.set(run.workflow, (workflowMap.get(run.workflow) ?? 0) + 1);
		if (activeStatuses.has(run.status)) active += 1;
		if (run.status === "success") success += 1;
		if (run.status === "failed") failed += 1;
		if (run.status === "wait_approval" || run.status === "paused") waiting += 1;
		if (terminalStatuses.has(run.status)) terminal += 1;

		if (run.finished_at) {
			durationTotal += runDurationMs(run, now);
			durationCount += 1;
		}
		artifacts += Number(
			run.recent_events?.filter((event) => event.includes("artifact")).length ??
				0,
		);
		retries += Number(
			run.recent_events?.filter((event) => event.includes("retry")).length ?? 0,
		);

		const day = formatDay(run.started_at);
		const bucket = trendMap.get(day) ?? { day, runs: 0, failures: 0 };
		bucket.runs += 1;
		if (run.status === "failed") bucket.failures += 1;
		trendMap.set(day, bucket);
	}

	return {
		total: runs.length,
		active,
		success,
		failed,
		waiting,
		terminal,
		successRate: terminal > 0 ? success / terminal : 0,
		avgDurationMs: durationCount > 0 ? durationTotal / durationCount : 0,
		artifacts,
		retries,
		statusCounts: Array.from(statusMap.entries()).map(([status, count]) => ({
			status,
			count,
		})),
		workflowCounts: Array.from(workflowMap.entries())
			.map(([workflow, count]) => ({ workflow, count }))
			.sort((a, b) => b.count - a.count)
			.slice(0, 8),
		trend: Array.from(trendMap.values()).sort((a, b) =>
			a.day.localeCompare(b.day),
		),
	};
}

export function formatDuration(ms: number): string {
	if (!Number.isFinite(ms) || ms <= 0) return "0s";
	const seconds = Math.round(ms / 1000);
	if (seconds < 60) return `${seconds}s`;
	const minutes = Math.floor(seconds / 60);
	const remainingSeconds = seconds % 60;
	if (minutes < 60)
		return remainingSeconds
			? `${minutes}m ${remainingSeconds}s`
			: `${minutes}m`;
	const hours = Math.floor(minutes / 60);
	const remainingMinutes = minutes % 60;
	return remainingMinutes ? `${hours}h ${remainingMinutes}m` : `${hours}h`;
}

function formatDay(iso: string): string {
	const date = new Date(iso);
	if (Number.isNaN(date.getTime())) return "unknown";
	return date.toISOString().slice(0, 10);
}
