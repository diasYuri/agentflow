import { describe, expect, test } from "vitest";
import {
	availableRunActions,
	buildDashboardMetrics,
	formatDuration,
	runDurationMs,
} from "../features/dashboard/dashboard-logic";
import type { WorkflowRun } from "../types";

const now = new Date("2026-05-20T12:10:00Z");

function run(partial: Partial<WorkflowRun>): WorkflowRun {
	return {
		id: "run-1",
		workflow: "release",
		run_dir: "/tmp/run-1",
		status: "running",
		started_at: "2026-05-20T12:00:00Z",
		...partial,
	};
}

describe("dashboard logic", () => {
	test("derives executive metrics from workflow runs", () => {
		const metrics = buildDashboardMetrics(
			[
				run({
					id: "success",
					status: "success",
					finished_at: "2026-05-20T12:02:00Z",
					recent_events: ["artifact.created"],
				}),
				run({
					id: "failed",
					status: "failed",
					workflow: "security",
					finished_at: "2026-05-20T12:04:00Z",
					recent_events: ["node.retrying"],
				}),
				run({ id: "approval", status: "wait_approval" }),
			],
			now,
		);

		expect(metrics.total).toBe(3);
		expect(metrics.success).toBe(1);
		expect(metrics.failed).toBe(1);
		expect(metrics.waiting).toBe(1);
		expect(metrics.successRate).toBe(0.5);
		expect(metrics.avgDurationMs).toBe(180000);
		expect(metrics.artifacts).toBe(1);
		expect(metrics.retries).toBe(1);
		expect(metrics.workflowCounts[0]).toMatchObject({
			workflow: "release",
			count: 2,
		});
	});

	test("maps run status to safe actions", () => {
		expect(availableRunActions("running")).toEqual(["pause", "cancel"]);
		expect(availableRunActions("paused")).toEqual(["resume", "cancel"]);
		expect(availableRunActions("wait_approval")).toEqual([
			"approve",
			"reject",
			"cancel",
		]);
		expect(availableRunActions("success")).toEqual([]);
	});

	test("formats durations and clamps invalid ranges", () => {
		expect(
			runDurationMs(
				run({
					started_at: "2026-05-20T12:00:00Z",
					finished_at: "2026-05-20T12:01:30Z",
				}),
				now,
			),
		).toBe(90000);
		expect(formatDuration(90000)).toBe("1m 30s");
		expect(formatDuration(7200000)).toBe("2h");
	});
});
