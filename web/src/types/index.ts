export interface Project {
	name: string;
	path: string;
}

export interface Session {
	id: string;
	project_name: string;
	project_path: string;
	title: string;
	status: "open" | "archived";
	provider: string;
	model: string;
	created_at: string;
	updated_at: string;
	last_message_at?: string;
	metadata?: Record<string, unknown>;
}

export interface Message {
	id: string;
	session_id: string;
	sequence: number;
	role: "user" | "assistant" | "system" | "tool";
	content?: string;
	payload_ref?: string;
	correlation_id?: string;
	created_at: string;
	metadata?: Record<string, unknown>;
}

export interface ToolCall {
	id: string;
	session_id: string;
	message_id?: string;
	name: string;
	status: "pending" | "running" | "succeeded" | "failed" | "cancelled";
	request_ref?: string;
	response_ref?: string;
	error?: string;
	correlation_id?: string;
	started_at: string;
	finished_at?: string;
}

export interface Approval {
	id: string;
	session_id: string;
	tool_call_id?: string;
	status: "pending" | "approved" | "rejected";
	reason?: string;
	decided_by?: string;
	created_at: string;
	decided_at?: string;
}

export interface Diagnostic {
	id: string;
	session_id?: string;
	level: "debug" | "info" | "warning" | "error";
	source: string;
	code?: string;
	message: string;
	context?: Record<string, unknown>;
	correlation_id?: string;
	created_at: string;
}

export interface HealthResponse {
	status: string;
	version: string;
	commit?: string;
	started_at: string;
	daemon_mode: string;
}

export interface SettingsResponse {
	web: {
		host: string;
		port: number;
		open_browser: boolean;
		dev_assets?: string;
		daemon: string;
	};
	paths: {
		root: string;
		daemon_socket?: string;
	};
}

export interface SSEEvent {
	id: number;
	session_id?: string;
	kind: string;
	correlation_id?: string;
	occurred_at: string;
	payload: unknown;
}
