export interface Project {
	name: string;
	path: string;
}

export interface PickFolderResponse {
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

export interface WorkflowDefinitionSummary {
	id: string;
	name: string;
	version: string;
	description?: string;
	created_at: string;
	updated_at: string;
}

export interface WorkflowDefinition {
	id: string;
	name: string;
	spec: WorkflowSpec;
	created_at: string;
	updated_at: string;
}

export interface WorkflowSpec {
	version?: string;
	name: string;
	description?: string;
	inputs?: Record<string, unknown>;
	vars?: Record<string, unknown>;
	secrets?: Record<string, unknown>;
	defaults?: Record<string, unknown>;
	execution?: Record<string, unknown>;
	nodes: Array<Record<string, unknown>>;
	worktree?: Record<string, unknown>;
	imports?: Array<Record<string, unknown> | string>;
	outputs?: Record<string, unknown>;
	hooks?: Array<Record<string, unknown>>;
	steps?: Record<string, unknown>;
}

export type WorkflowRunStatus =
	| "created"
	| "validating"
	| "planned"
	| "running"
	| "paused"
	| "wait_approval"
	| "success"
	| "failed"
	| "cancelled";

export interface WorkflowRun {
	id: string;
	workflow: string;
	run_dir: string;
	status: WorkflowRunStatus;
	started_at: string;
	finished_at?: string;
	paused_at?: string;
	approval_at?: string;
	pause_reason?: string;
	approval_node_id?: string;
	approval_message?: string;
	resume_count?: number;
	current_step?: string;
	completed_steps?: string[];
	pending_steps?: string[];
	total_steps?: number;
	error?: string;
	terminal_error?: string;
	failure_reason?: string;
	recent_events?: string[];
	tag?: string;
}

export interface WorkflowInspect {
	run_id: string;
	workflow: string;
	status: WorkflowRunStatus;
	started_at: string;
	finished_at?: string;
	approval_at?: string;
	duration_ms: number;
	current_step?: string;
	completed_steps?: string[];
	pending_steps?: string[];
	total_steps: number;
	failed_nodes: number;
	retries: number;
	agent_calls: number;
	bash_calls: number;
	first_error?: string;
	error?: string;
	terminal_error?: string;
	failure_reason?: string;
	approval_node_id?: string;
	approval_message?: string;
	tag?: string;
	artifact_count: number;
	node_count: number;
	slowest_nodes?: SlowestNode[];
	agent_usage?: AgentUsage[];
}

export interface WorkflowNodeResult {
	node_id: string;
	instance_id?: string;
	path?: string[];
	index?: number;
	status: string;
	output?: unknown;
	outputs?: unknown[];
	stdout?: string;
	stderr?: string;
	error?: string;
	exit_code?: number;
	duration_ms?: number;
	attempts?: number;
}

export interface WorkflowTimelineEntry {
	ts: string;
	type: string;
	node_id?: string;
	instance_id?: string;
	attempt?: number;
	duration_ms?: number;
}

export interface WorkflowEvent {
	cursor: number;
	timestamp: string;
	run_id: string;
	type: string;
	node_id?: string;
	instance_id?: string;
	path?: string[];
	attempt?: number;
	data?: Record<string, unknown>;
	error?: string;
}

export interface WorkflowArtifact {
	id: string;
	name: string;
	path: string;
	size: number;
	content_type?: string;
	modified_at?: string;
	run_id?: string;
	node_id?: string;
	instance_id?: string;
	relative_path?: string;
	media_type?: string;
	size_bytes?: number;
	created_at?: string;
	kind?: string;
	description?: string;
}

export interface SlowestNode {
	node_id: string;
	duration_ms: number;
}

export interface AgentUsage {
	provider: string;
	model?: string;
	input_tokens?: number;
	output_tokens?: number;
	total_tokens?: number;
	cost_usd?: number;
}

export interface SSEEvent {
	id: number;
	session_id?: string;
	kind: string;
	correlation_id?: string;
	occurred_at: string;
	payload: unknown;
}
