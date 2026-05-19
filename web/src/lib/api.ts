import type {
	Approval,
	Diagnostic,
	HealthResponse,
	Message,
	Project,
	Session,
	SettingsResponse,
	ToolCall,
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
};
