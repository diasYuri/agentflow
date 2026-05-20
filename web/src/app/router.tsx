import { DashboardView } from "@/features/dashboard/dashboard-view";
import { DiagnosticsView } from "@/features/diagnostics/diagnostics-view";
import { ProjectView } from "@/features/project/project-view";
import { SessionView } from "@/features/session/session-view";
import { SettingsView } from "@/features/settings/settings-view";
import { WorkflowView } from "@/features/workflow/workflow-view";
import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { Layout } from "./layout";

export function Router() {
	return (
		<BrowserRouter>
			<Routes>
				<Route path="/" element={<Layout />}>
					<Route index element={<ProjectView />} />
					<Route path="projects/:name" element={<ProjectView />} />
					<Route path="sessions/:id" element={<SessionView />} />
					<Route path="dashboard" element={<DashboardView />} />
					<Route path="workflow" element={<WorkflowView />} />
					<Route path="diagnostics" element={<DiagnosticsView />} />
					<Route path="settings" element={<SettingsView />} />
					<Route path="*" element={<Navigate to="/" replace />} />
				</Route>
			</Routes>
		</BrowserRouter>
	);
}
