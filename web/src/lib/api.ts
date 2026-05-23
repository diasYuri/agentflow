import type {
	Approval,
	Diagnostic,
	HealthResponse,
	Message,
	PickFolderResponse,
	Project,
	RunWorkflowRequest,
	Session,
	SettingsResponse,
	ToolCall,
	WorkflowArtifact,
	WorkflowDefinition,
	WorkflowDefinitionSummary,
	WorkflowEvent,
	WorkflowInspect,
	WorkflowNodeResult,
	WorkflowRun,
	WorkflowTimelineEntry,
} from "@/types";
import { getToken } from "./utils";

function headers(): Record<string, string> {
	const t = getToken();
	return {
		"Content-Type": "application/json",
		...(t ? { Authorization: `Bearer ${t}` } : {}),
	};
}

async function fetchJSON<T>(
	input: RequestInfo,
	init?: RequestInit,
): Promise<T> {
	const res = await fetch(input, {
		...init,
		headers: { ...headers(), ...(init?.headers ?? {}) },
	});
	if (!res.ok) {
		const body = await res.text().catch(() => "");
		throw new Error(`${res.status} ${res.statusText}: ${body}`);
	}
	if (res.status === 204) return undefined as T;
	return res.json() as Promise<T>;
}

function arrayOrEmpty<T>(value: T[] | null | undefined): T[] {
	return Array.isArray(value) ? value : [];
}

export const api = {
	health: () => fetchJSON<HealthResponse>("/api/v1/health"),
	settings: () => fetchJSON<SettingsResponse>("/api/v1/settings"),

	projects: {
		list: () =>
			fetchJSON<{ projects: Project[] | null }>("/api/v1/projects").then((r) =>
				arrayOrEmpty(r.projects),
			),
		create: (body: { name: string; path: string }) =>
			fetchJSON<Project>("/api/v1/projects", {
				method: "POST",
				body: JSON.stringify(body),
			}),
		pickFolder: () =>
			fetchJSON<PickFolderResponse>("/api/v1/projects/pick-folder", {
				method: "POST",
			}),
		get: (name: string) =>
			fetchJSON<Project>(`/api/v1/projects/${encodeURIComponent(name)}`),
		sessions: (name: string) =>
			fetchJSON<{ sessions: Session[] | null }>(
				`/api/v1/projects/${encodeURIComponent(name)}/sessions`,
			).then((r) => arrayOrEmpty(r.sessions)),
		createSession: (
			name: string,
			body: { title?: string; provider?: string; model?: string },
		) =>
			fetchJSON<Session>(
				`/api/v1/projects/${encodeURIComponent(name)}/sessions`,
				{
					method: "POST",
					body: JSON.stringify(body),
				},
			),
	},

	sessions: {
		list: (project?: string) => {
			const q = project ? `?project=${encodeURIComponent(project)}` : "";
			return fetchJSON<{ sessions: Session[] | null }>(
				`/api/v1/sessions${q}`,
			).then((r) => arrayOrEmpty(r.sessions));
		},
		get: (id: string) => fetchJSON<Session>(`/api/v1/sessions/${id}`),
		patch: (id: string, body: { title?: string; status?: string }) =>
			fetchJSON<Session>(`/api/v1/sessions/${id}`, {
				method: "PATCH",
				body: JSON.stringify(body),
			}),
		delete: (id: string) =>
			fetchJSON<void>(`/api/v1/sessions/${id}`, { method: "DELETE" }),
		messages: (id: string, since?: number) => {
			const q = since ? `?since_sequence=${since}` : "";
			return fetchJSON<{ messages: Message[] | null }>(
				`/api/v1/sessions/${id}/messages${q}`,
			).then((r) => arrayOrEmpty(r.messages));
		},
		appendMessage: (
			id: string,
			body: { role: string; content: string; correlation_id?: string },
		) =>
			fetchJSON<Message>(`/api/v1/sessions/${id}/messages`, {
				method: "POST",
				body: JSON.stringify(body),
			}),
		toolCalls: (id: string) =>
			fetchJSON<{ tool_calls: ToolCall[] | null }>(
				`/api/v1/sessions/${id}/tool-calls`,
			).then((r) => arrayOrEmpty(r.tool_calls)),
		createToolCall: (
			id: string,
			body: { name: string; message_id?: string; status?: string },
		) =>
			fetchJSON<ToolCall>(`/api/v1/sessions/${id}/tool-calls`, {
				method: "POST",
				body: JSON.stringify(body),
			}),
		updateToolCall: (
			toolCallId: string,
			body: { status: string; response_ref?: string; error?: string },
		) =>
			fetchJSON<void>(`/api/v1/tool-calls/${toolCallId}`, {
				method: "PATCH",
				body: JSON.stringify(body),
			}),
		approvals: (id: string) =>
			fetchJSON<{ approvals: Approval[] | null }>(
				`/api/v1/sessions/${id}/approvals`,
			).then((r) => arrayOrEmpty(r.approvals)),
		createApproval: (
			id: string,
			body: { tool_call_id?: string; reason?: string },
		) =>
			fetchJSON<Approval>(`/api/v1/sessions/${id}/approvals`, {
				method: "POST",
				body: JSON.stringify(body),
			}),
		diagnostics: (id: string) =>
			fetchJSON<{ diagnostics: Diagnostic[] | null }>(
				`/api/v1/sessions/${id}/diagnostics`,
			).then((r) => arrayOrEmpty(r.diagnostics)),
	},

	approvals: {
		decide: (
			id: string,
			body: {
				status: "approved" | "rejected";
				reason?: string;
				decided_by?: string;
			},
		) =>
			fetchJSON<void>(`/api/v1/approvals/${id}/decide`, {
				method: "POST",
				body: JSON.stringify(body),
			}),
	},

	diagnostics: {
		recent: (limit = 100) =>
			fetchJSON<{ diagnostics: Diagnostic[] | null }>(
				`/api/v1/diagnostics?limit=${limit}`,
			).then((r) => arrayOrEmpty(r.diagnostics)),
	},

	workflows: {
		list: () =>
			fetchJSON<{ workflow_definitions: WorkflowDefinitionSummary[] | null }>(
				"/api/v1/workflow-definitions",
			).then((r) => arrayOrEmpty(r.workflow_definitions)),
		get: (id: string) =>
			fetchJSON<{ workflow_definition: WorkflowDefinition }>(
				`/api/v1/workflow-definitions/${encodeURIComponent(id)}`,
			).then((r) => r.workflow_definition),
		createFromYaml: (yaml: string) =>
			fetchJSON<{ workflow_definition: WorkflowDefinition }>(
				"/api/v1/workflow-definitions",
				{
					method: "POST",
					body: JSON.stringify({ yaml }),
				},
			).then((r) => r.workflow_definition),
		updateFromYaml: (id: string, yaml: string) =>
			fetchJSON<{ workflow_definition: WorkflowDefinition }>(
				`/api/v1/workflow-definitions/${encodeURIComponent(id)}`,
				{
					method: "PUT",
					body: JSON.stringify({ yaml }),
				},
			).then((r) => r.workflow_definition),
		delete: (id: string) =>
			fetchJSON<void>(
				`/api/v1/workflow-definitions/${encodeURIComponent(id)}`,
				{ method: "DELETE" },
			),
	},

	workflowRuns: {
		run: (body: RunWorkflowRequest) =>
			fetchJSON<{ run: WorkflowRun }>("/api/v1/workflows", {
				method: "POST",
				body: JSON.stringify(body),
			}).then((r) => r.run),
		list: () =>
			fetchJSON<{ runs: WorkflowRun[] | null }>("/api/v1/workflows").then((r) =>
				arrayOrEmpty(r.runs),
			),
		get: (runId: string) =>
			fetchJSON<{ run: WorkflowRun }>(
				`/api/v1/workflows/${encodeURIComponent(runId)}`,
			).then((r) => r.run),
		inspect: (runId: string) =>
			fetchJSON<WorkflowInspect>(
				`/api/v1/workflows/${encodeURIComponent(runId)}/inspect`,
			),
		nodes: (runId: string) =>
			fetchJSON<{ nodes: WorkflowNodeResult[] | null }>(
				`/api/v1/workflows/${encodeURIComponent(runId)}/nodes`,
			).then((r) => arrayOrEmpty(r.nodes)),
		timeline: (runId: string, cursor = 0, limit = 100) =>
			fetchJSON<{
				entries: WorkflowTimelineEntry[] | null;
				next_cursor: number;
				has_more: boolean;
			}>(
				`/api/v1/workflows/${encodeURIComponent(runId)}/timeline?cursor=${cursor}&limit=${limit}`,
			).then((r) => ({
				...r,
				entries: arrayOrEmpty(r.entries),
			})),
		events: (runId: string, cursor = 0, limit = 100) =>
			fetchJSON<{
				events: WorkflowEvent[] | null;
				next_cursor: number;
				has_more: boolean;
			}>(
				`/api/v1/workflows/${encodeURIComponent(runId)}/events?cursor=${cursor}&limit=${limit}`,
			).then((r) => ({
				...r,
				events: arrayOrEmpty(r.events),
			})),
		artifacts: (runId: string) =>
			fetchJSON<{ artifacts: WorkflowArtifact[] | null }>(
				`/api/v1/workflows/${encodeURIComponent(runId)}/artifacts`,
			).then((r) => arrayOrEmpty(r.artifacts)),
		action: (
			runId: string,
			action: "cancel" | "pause" | "resume" | "approve" | "reject",
		) =>
			fetchJSON<{ run: WorkflowRun }>(
				`/api/v1/workflows/${encodeURIComponent(runId)}/${action}`,
				{ method: "POST" },
			).then((r) => r.run),
	},
};
