import type { Project, Session } from "@/types";

export function groupSessionsByProject(
	projects: Project[],
	sessions: Session[],
) {
	const out: Record<string, Session[]> = {};
	for (const project of projects) {
		out[project.name] = [];
	}
	for (const session of sessions) {
		if (!out[session.project_name]) out[session.project_name] = [];
		out[session.project_name].push(session);
	}
	for (const projectSessions of Object.values(out)) {
		projectSessions.sort(
			(a, b) =>
				new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime(),
		);
	}
	return out;
}

export function uniqueProjectName(baseName: string, projects: Project[]) {
	const base = (baseName || "project").trim();
	const existing = new Set(projects.map((p) => p.name));
	if (!existing.has(base)) return base;
	let i = 2;
	while (existing.has(`${base}-${i}`)) i += 1;
	return `${base}-${i}`;
}
