import * as DesktopService from "../../bindings/github.com/diasYuri/agentflow/internal/desktop/binding/desktopservice.js";

export type {
	DesktopError,
	InputDocument,
	NodeSummary,
	LoadedWorkflow,
	ValidationResult,
	GraphResult,
	NodePlan,
	DryRunResult,
	WorkflowSummary,
	AppSettings,
	RunSummary,
	RunWorkflowRequest,
	ListRunsResponse,
	RunEvent,
	EventsResponse,
	ArtifactInfo,
	ArtifactsResponse,
	ArtifactResponse,
	LogsResponse,
} from "./types.js";

export { DesktopService };

export async function health() {
	return DesktopService.Health();
}

export async function loadWorkflow(path: string) {
	return DesktopService.LoadWorkflow(
		path,
	) as import("./types.js").LoadedWorkflow;
}

export async function listWorkflows() {
	return DesktopService.ListWorkflows() as import("./types.js").WorkflowSummary[];
}

export async function validateWorkflow(path: string) {
	return DesktopService.ValidateWorkflow(
		path,
	) as import("./types.js").ValidationResult;
}

export async function generateGraph(path: string) {
	return DesktopService.GenerateGraph(path) as import("./types.js").GraphResult;
}

export async function dryRunWorkflow(
	path: string,
	inputs: Record<string, unknown>,
	vars?: Record<string, unknown>,
	maxConcurrency?: number,
	workingDir?: string,
) {
	return DesktopService.DryRunWorkflow(
		path,
		inputs,
		vars ?? {},
		maxConcurrency ?? 4,
		workingDir ?? "",
	) as import("./types.js").DryRunResult;
}

export async function saveWorkflow(path: string, content: string) {
	return DesktopService.SaveWorkflow(path, content);
}

export async function saveInput(path: string, content: string) {
	return DesktopService.SaveInput(path, content);
}

export async function getAppSettings() {
	return DesktopService.GetAppSettings() as import("./types.js").AppSettings;
}

export async function updateAppSettings(
	settings: import("./types.js").AppSettings,
) {
	return DesktopService.UpdateAppSettings(settings as any);
}

export async function resolveInput(
	path: string,
	inputs: Record<string, unknown>,
) {
	return DesktopService.ResolveInput(path, inputs) as Promise<
		Record<string, unknown>
	>;
}

export async function runWorkflow(
	req: import("./types.js").RunWorkflowRequest,
) {
	return DesktopService.RunWorkflow(req) as import("./types.js").RunSummary;
}

export async function cancelRun(runID: string) {
	return DesktopService.CancelRun(runID) as import("./types.js").RunSummary;
}

export async function listRuns() {
	return DesktopService.ListRuns() as import("./types.js").ListRunsResponse;
}

export async function getRun(runID: string) {
	return DesktopService.GetRun(runID) as import("./types.js").RunSummary;
}

export async function getRunEvents(
	runID: string,
	cursor: number,
	limit: number,
) {
	return DesktopService.GetRunEvents(
		runID,
		cursor,
		limit,
	) as import("./types.js").EventsResponse;
}

export async function getRunArtifacts(runID: string) {
	return DesktopService.GetRunArtifacts(
		runID,
	) as import("./types.js").ArtifactsResponse;
}

export async function getRunArtifact(runID: string, artifactID: string) {
	return DesktopService.GetRunArtifact(
		runID,
		artifactID,
	) as import("./types.js").ArtifactResponse;
}

export async function getRunLogs(runID: string) {
	return DesktopService.GetRunLogs(runID) as import("./types.js").LogsResponse;
}

export async function openPath(path: string) {
	return DesktopService.OpenPath(path);
}
