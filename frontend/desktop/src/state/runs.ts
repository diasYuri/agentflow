import { useCallback, useEffect, useRef, useState } from "react";
import * as api from "../api/bindings.js";
import type { RunStatus } from "../api/types.js";

export type RunTab = "timeline" | "logs" | "artifacts" | "settings";

export interface RunDetail {
	run: api.RunSummary;
	events: api.RunEvent[];
	logs: string[];
	artifacts: api.ArtifactInfo[];
	eventCursor: number;
	isStreaming: boolean;
	error: string | null;
}

export interface RunsState {
	runs: api.RunSummary[];
	selectedRunId: string | null;
	details: Record<string, RunDetail>;
	isLoading: boolean;
	error: string | null;
	activePollRunId: string | null;
}

export function useRuns() {
	const [state, setState] = useState<RunsState>({
		runs: [],
		selectedRunId: null,
		details: {},
		isLoading: false,
		error: null,
		activePollRunId: null,
	});
	const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);
	const abortRef = useRef(false);
	const stateRef = useRef(state);
	stateRef.current = state;

	const isTerminalStatus = (status?: RunStatus) =>
		status === "success" || status === "failed" || status === "cancelled";

	const ensureDetail = useCallback((runId: string) => {
		setState((s) => {
			if (s.details[runId]) return s;
			return {
				...s,
				details: {
					...s.details,
					[runId]: {
						run: s.runs.find((r) => r.id === runId) ?? ({} as api.RunSummary),
						events: [],
						logs: [],
						artifacts: [],
						eventCursor: 0,
						isStreaming: false,
						error: null,
					},
				},
			};
		});
	}, []);

	const loadRuns = useCallback(async () => {
		setState((s) => ({ ...s, isLoading: true, error: null }));
		try {
			const resp = await api.listRuns();
			setState((s) => {
				const updated = { ...s, runs: resp.runs ?? [], isLoading: false };
				if (s.selectedRunId) {
					const run = resp.runs.find((r) => r.id === s.selectedRunId);
					if (run && updated.details[s.selectedRunId]) {
						updated.details = {
							...updated.details,
							[s.selectedRunId]: {
								...updated.details[s.selectedRunId],
								run,
							},
						};
					}
				}
				return updated;
			});
		} catch (err) {
			setState((s) => ({
				...s,
				isLoading: false,
				error: err instanceof Error ? err.message : String(err),
			}));
		}
	}, []);

	const selectRun = useCallback(
		(runId: string | null) => {
			setState((s) => ({ ...s, selectedRunId: runId }));
			if (runId) ensureDetail(runId);
		},
		[ensureDetail],
	);

	const fetchEvents = useCallback(async (runId: string) => {
		const detail = stateRef.current.details[runId] ?? {
			run: {} as api.RunSummary,
			events: [],
			logs: [],
			artifacts: [],
			eventCursor: 0,
			isStreaming: false,
			error: null,
		};
		try {
			const resp = await api.getRunEvents(runId, detail.eventCursor, 100);
			if (abortRef.current) return;
			setState((s) => {
				const existing = s.details[runId];
				if (!existing) return s;
				const newEvents = resp.events.filter(
					(e) => !existing.events.some((ex) => ex.cursor === e.cursor),
				);
				if (newEvents.length === 0 && !resp.has_more) return s;
				return {
					...s,
					details: {
						...s.details,
						[runId]: {
							...existing,
							events: [...existing.events, ...newEvents],
							eventCursor: resp.next_cursor,
							isStreaming: true,
							error: null,
						},
					},
				};
			});
		} catch (err) {
			setState((s) => {
				const existing = s.details[runId];
				if (!existing) return s;
				return {
					...s,
					details: {
						...s.details,
						[runId]: {
							...existing,
							isStreaming: false,
							error: err instanceof Error ? err.message : String(err),
						},
					},
				};
			});
		}
	}, []);

	const fetchLogs = useCallback(async (runId: string) => {
		try {
			const resp = await api.getRunLogs(runId);
			setState((s) => {
				const existing = s.details[runId];
				if (!existing) return s;
				return {
					...s,
					details: {
						...s.details,
						[runId]: { ...existing, logs: resp.lines ?? [] },
					},
				};
			});
		} catch {
			// silently ignore logs errors
		}
	}, []);

	const fetchArtifacts = useCallback(async (runId: string) => {
		try {
			const resp = await api.getRunArtifacts(runId);
			setState((s) => {
				const existing = s.details[runId];
				if (!existing) return s;
				return {
					...s,
					details: {
						...s.details,
						[runId]: { ...existing, artifacts: resp.artifacts ?? [] },
					},
				};
			});
		} catch {
			// silently ignore artifacts errors
		}
	}, []);

	const fetchRun = useCallback(async (runId: string) => {
		try {
			const run = await api.getRun(runId);
			if (abortRef.current) return;
			setState((s) => {
				const idx = s.runs.findIndex((r) => r.id === runId);
				const nextRuns =
					idx >= 0
						? [...s.runs.slice(0, idx), run, ...s.runs.slice(idx + 1)]
						: [run, ...s.runs];
				const existing = s.details[runId];
				return {
					...s,
					runs: nextRuns,
					details: existing
						? {
								...s.details,
								[runId]: {
									...existing,
									run,
									isStreaming: !isTerminalStatus(run.status),
									error: null,
								},
							}
						: s.details,
					activePollRunId: isTerminalStatus(run.status)
						? null
						: s.activePollRunId,
				};
			});
		} catch (err) {
			setState((s) => {
				const existing = s.details[runId];
				if (!existing) return s;
				return {
					...s,
					details: {
						...s.details,
						[runId]: {
							...existing,
							error: err instanceof Error ? err.message : String(err),
						},
					},
				};
			});
		}
	}, []);

	const startRun = useCallback(async (req: api.RunWorkflowRequest) => {
		setState((s) => ({ ...s, isLoading: true, error: null }));
		try {
			const run = await api.runWorkflow(req);
			setState((s) => ({
				...s,
				runs: [run, ...s.runs],
				selectedRunId: run.id,
				isLoading: false,
				details: {
					...s.details,
					[run.id]: {
						run,
						events: [],
						logs: [],
						artifacts: [],
						eventCursor: 0,
						isStreaming: true,
						error: null,
					},
				},
				activePollRunId: run.id,
			}));
			return run;
		} catch (err) {
			setState((s) => ({
				...s,
				isLoading: false,
				error: err instanceof Error ? err.message : String(err),
			}));
			throw err;
		}
	}, []);

	const cancelRun = useCallback(async (runId: string) => {
		try {
			const run = await api.cancelRun(runId);
			setState((s) => {
				const idx = s.runs.findIndex((r) => r.id === runId);
				const nextRuns =
					idx >= 0
						? [...s.runs.slice(0, idx), run, ...s.runs.slice(idx + 1)]
						: s.runs;
				const existing = s.details[runId];
				return {
					...s,
					runs: nextRuns,
					details: existing
						? {
								...s.details,
								[runId]: { ...existing, run, isStreaming: false },
							}
						: s.details,
					activePollRunId: null,
				};
			});
		} catch (err) {
			setState((s) => ({
				...s,
				error: err instanceof Error ? err.message : String(err),
			}));
		}
	}, []);

	useEffect(() => {
		loadRuns();
	}, [loadRuns]);

	useEffect(() => {
		if (pollRef.current) {
			clearInterval(pollRef.current);
			pollRef.current = null;
		}
		const activeId = state.activePollRunId ?? state.selectedRunId;
		if (!activeId) return;
		const run = state.runs.find((r) => r.id === activeId);
		const isTerminal = !run || isTerminalStatus(run.status);
		if (isTerminal) return;

		abortRef.current = false;
		fetchRun(activeId);
		fetchEvents(activeId);
		fetchLogs(activeId);
		fetchArtifacts(activeId);
		pollRef.current = setInterval(() => {
			if (abortRef.current) return;
			fetchRun(activeId);
			fetchEvents(activeId);
			fetchLogs(activeId);
			fetchArtifacts(activeId);
		}, 1200);
		return () => {
			if (pollRef.current) {
				clearInterval(pollRef.current);
				pollRef.current = null;
			}
			abortRef.current = true;
		};
	}, [
		state.activePollRunId,
		state.selectedRunId,
		state.runs,
		fetchRun,
		fetchEvents,
		fetchLogs,
		fetchArtifacts,
	]);

	return {
		state,
		loadRuns,
		selectRun,
		startRun,
		cancelRun,
		fetchRun,
		fetchEvents,
		fetchLogs,
		fetchArtifacts,
	};
}
