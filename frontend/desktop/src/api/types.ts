export interface DesktopError {
	message: string;
	code: string;
	context?: string;
}

export interface InputDocument {
	name: string;
	type: string;
	required: boolean;
	default?: unknown;
	description?: string;
}

export interface NodeSummary {
	id: string;
	kind: string;
	dependsOn?: string[];
}

export interface LoadedWorkflow {
	name: string;
	description: string;
	version: string;
	inputs: Record<string, InputDocument>;
	nodes: NodeSummary[];
	sourcePath: string;
	rawYaml?: string;
}

export interface ValidationResult {
	valid: boolean;
	name: string;
	nodeCount: number;
	errors?: DesktopError[];
}

export interface GraphResult {
	mermaid: string;
	valid: boolean;
	errors?: DesktopError[];
}

export interface NodePlan {
	id: string;
	dependencies: string[];
	dependents: string[];
	kind: string;
}

export interface DryRunResult {
	workflow: string;
	inputs: Record<string, unknown>;
	order: string[];
	nodes: Record<string, NodePlan>;
	valid: boolean;
	errors?: DesktopError[];
}

export interface WorkflowSummary {
	name: string;
	description: string;
	path: string;
}

export interface AppSettings {
	workspacePath: string;
	recentFiles: string[];
	theme: string;
	codexPath: string;
	claudePath: string;
	piPath: string;
	logFormat: string;
}

export type RunStatus =
	| "created"
	| "validating"
	| "planned"
	| "running"
	| "paused"
	| "success"
	| "failed"
	| "cancelled"
	| "";

export interface RunSummary {
	id: string;
	workflow: string;
	status: RunStatus;
	started_at: string;
	finished_at?: string;
	error?: string;
	current_step?: string;
	completed_steps?: string[];
	pending_steps?: string[];
	total_steps?: number;
}

export interface RunWorkflowRequest {
	workflow_ref: string;
	inputs?: Record<string, unknown>;
	vars?: Record<string, unknown>;
	max_concurrency?: number;
	working_dir?: string;
}

export interface ListRunsResponse {
	runs: RunSummary[];
}

export interface RunEvent {
	cursor: number;
	timestamp: string;
	run_id: string;
	type: string;
	node_id?: string;
	instance_id?: string;
	path?: string[];
	attempt?: number;
	data?: Record<string, unknown>;
}

export interface EventsResponse {
	run_id: string;
	events: RunEvent[];
	next_cursor: number;
	has_more: boolean;
}

export interface ArtifactInfo {
	id: string;
	name: string;
	path: string;
	size: number;
	content_type?: string;
	modified_at?: string;
}

export interface ArtifactsResponse {
	run_id: string;
	artifacts: ArtifactInfo[];
}

export interface ArtifactResponse {
	id: string;
	name: string;
	path: string;
	size: number;
	content_type?: string;
	encoding?: string;
	content: string;
}

export interface LogsResponse {
	run_id: string;
	lines: string[];
}
