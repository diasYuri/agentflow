import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { api } from "@/lib/api";
import { groupSessionsByProject, uniqueProjectName } from "@/lib/project-tree";
import { useStore } from "@/lib/store";
import { cn, formatDate } from "@/lib/utils";
import type { PickFolderResponse, Project, Session } from "@/types";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	Bot,
	ChevronDown,
	ChevronRight,
	Folder,
	MessageSquare,
	Plus,
	Search,
	Settings,
	Sparkles,
} from "lucide-react";
import { useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";

export function ProjectRail() {
	const queryClient = useQueryClient();
	const { data: projects = [] } = useQuery({
		queryKey: ["projects"],
		queryFn: api.projects.list,
	});
	const { data: sessions = [] } = useQuery({
		queryKey: ["sessions", "all"],
		queryFn: () => api.sessions.list(),
	});
	const {
		selectedProject,
		sidebarOpen,
		setSelectedProject,
		setSelectedSession,
	} = useStore();
	const params = useParams();
	const activeSession = sessions.find((s) => s.id === params.id) ?? null;
	const active =
		params.name ?? activeSession?.project_name ?? selectedProject ?? null;
	const [expanded, setExpanded] = useState<Record<string, boolean>>({});
	const [pendingProject, setPendingProject] =
		useState<PickFolderResponse | null>(null);
	const [projectName, setProjectName] = useState("");
	const [projectError, setProjectError] = useState<string | null>(null);

	const sessionsByProject = useMemo(
		() => groupSessionsByProject(projects, sessions),
		[projects, sessions],
	);

	const pickProject = useMutation({
		mutationFn: api.projects.pickFolder,
		onSuccess: async (folder) => {
			const name = uniqueProjectName(folder.name, projects);
			if (name === folder.name) {
				await createProject.mutateAsync({ name, path: folder.path });
				return;
			}
			setPendingProject(folder);
			setProjectName(name);
			setProjectError(null);
		},
		onError: (err) => setProjectError(String(err)),
	});

	const createProject = useMutation({
		mutationFn: api.projects.create,
		onSuccess: (project) => {
			queryClient.invalidateQueries({ queryKey: ["projects"] });
			setSelectedProject(project.name);
			setExpanded((state) => ({ ...state, [project.name]: true }));
			setPendingProject(null);
			setProjectName("");
			setProjectError(null);
		},
		onError: (err) => setProjectError(String(err)),
	});

	if (!sidebarOpen) return null;

	return (
		<aside className="flex w-[296px] shrink-0 flex-col bg-[#202326] text-neutral-300 dark:bg-[#202326]">
			<div className="space-y-1 px-3 py-4">
				<Link
					to="/"
					onClick={() => setSelectedProject(active)}
					className="flex items-center gap-3 rounded-xl px-3 py-2 text-[15px] text-neutral-100 transition-colors hover:bg-white/7"
				>
					<Sparkles className="size-4" />
					New chat
				</Link>
				<button
					type="button"
					className="flex w-full items-center gap-3 rounded-xl px-3 py-2 text-left text-[15px] transition-colors hover:bg-white/7"
				>
					<Search className="size-4" />
					Search
				</button>
				<Link
					to="/workflow"
					className="flex items-center gap-3 rounded-xl px-3 py-2 text-[15px] transition-colors hover:bg-white/7"
				>
					<Bot className="size-4" />
					Workflows
				</Link>
			</div>

			<nav className="min-h-0 flex-1 overflow-auto px-3 pb-3">
				<div className="mb-2 mt-3 flex items-center justify-between px-1 text-[13px] text-neutral-500">
					<span>Projects</span>
					<button
						type="button"
						onClick={() => pickProject.mutate()}
						disabled={pickProject.isPending || createProject.isPending}
						className="inline-flex size-6 items-center justify-center rounded-lg text-neutral-400 transition-colors hover:bg-white/7 hover:text-neutral-100 disabled:opacity-50"
						aria-label="Add project"
						title="Add project"
					>
						<Plus className="size-3.5" />
					</button>
				</div>
				<ul className="space-y-1">
					{projects.map((p) => (
						<li key={p.name} className="space-y-1">
							<div
								className={cn(
									"group flex items-center rounded-xl text-sm transition-colors",
									active === p.name
										? "bg-white/10 text-neutral-50"
										: "text-neutral-400 hover:bg-white/7 hover:text-neutral-100",
								)}
							>
								<button
									type="button"
									onClick={() =>
										setExpanded((state) => ({
											...state,
											[p.name]: !(state[p.name] ?? active === p.name),
										}))
									}
									className="ml-1 inline-flex size-7 shrink-0 items-center justify-center rounded-lg text-neutral-500 hover:text-neutral-100"
									aria-label={
										expanded[p.name] ? "Collapse project" : "Expand project"
									}
								>
									{(expanded[p.name] ?? active === p.name) ? (
										<ChevronDown className="size-3.5" />
									) : (
										<ChevronRight className="size-3.5" />
									)}
								</button>
								<Link
									to={`/projects/${encodeURIComponent(p.name)}`}
									onClick={() => {
										setSelectedProject(p.name);
										setExpanded((state) => ({ ...state, [p.name]: true }));
									}}
									className="flex min-w-0 flex-1 items-center gap-2 py-2 pr-3"
									title={p.path}
								>
									<Folder className="size-4 shrink-0" />
									<span className="truncate">{p.name}</span>
								</Link>
							</div>
							{(expanded[p.name] ?? active === p.name) && (
								<ProjectSessions
									project={p}
									sessions={sessionsByProject[p.name] ?? []}
									activeSessionId={params.id ?? null}
									onSelectSession={(session) => {
										setSelectedProject(session.project_name);
										setSelectedSession(session.id);
									}}
								/>
							)}
						</li>
					))}
					{projects.length === 0 && (
						<li className="px-3 py-4 text-center text-xs text-neutral-500">
							No projects yet.
							<br />
							Use + to add a folder.
						</li>
					)}
				</ul>

				<div className="mt-6 px-1 text-[13px] text-neutral-500">
					<Badge
						variant="secondary"
						className="border-white/10 bg-white/5 text-[10px] text-neutral-400"
					>
						{sessions.length} chats
					</Badge>
				</div>
			</nav>

			<div className="space-y-1 border-t border-white/8 p-3">
				<Button
					asChild
					variant="ghost"
					className="h-10 w-full justify-start gap-3 rounded-xl text-neutral-300 hover:bg-white/7 hover:text-neutral-50"
				>
					<Link to={active ? `/projects/${encodeURIComponent(active)}` : "/"}>
						<Plus className="size-4" />
						Start in project
					</Link>
				</Button>
				<Button
					asChild
					variant="ghost"
					className="h-10 w-full justify-start gap-3 rounded-xl text-neutral-300 hover:bg-white/7 hover:text-neutral-50"
				>
					<Link to="/settings">
						<Settings className="size-4" />
						Settings
					</Link>
				</Button>
			</div>
			{(pendingProject || projectError) && (
				<div className="border-t border-white/8 p-3">
					<form
						onSubmit={(e) => {
							e.preventDefault();
							if (!pendingProject) return;
							createProject.mutate({
								name: projectName.trim(),
								path: pendingProject.path,
							});
						}}
						className="space-y-2 rounded-xl border border-white/10 bg-white/5 p-3"
					>
						<div className="text-xs font-medium text-neutral-200">
							Add project
						</div>
						{pendingProject && (
							<>
								<input
									value={projectName}
									onChange={(e) => setProjectName(e.target.value)}
									className="h-8 w-full rounded-lg border border-white/10 bg-black/20 px-2 text-xs text-neutral-100 outline-none focus:border-white/25"
									placeholder="Project name"
								/>
								<div className="truncate text-[11px] text-neutral-500">
									{pendingProject.path}
								</div>
							</>
						)}
						{projectError && (
							<div className="text-[11px] text-amber-300">{projectError}</div>
						)}
						<div className="flex justify-end gap-2">
							<button
								type="button"
								onClick={() => {
									setPendingProject(null);
									setProjectError(null);
								}}
								className="rounded-lg px-2 py-1 text-xs text-neutral-400 hover:bg-white/7 hover:text-neutral-100"
							>
								Cancel
							</button>
							{pendingProject && (
								<button
									type="submit"
									disabled={!projectName.trim() || createProject.isPending}
									className="rounded-lg bg-neutral-100 px-2 py-1 text-xs text-neutral-900 disabled:opacity-50"
								>
									Add
								</button>
							)}
						</div>
					</form>
				</div>
			)}
		</aside>
	);
}

function ProjectSessions({
	project,
	sessions,
	activeSessionId,
	onSelectSession,
}: {
	project: Project;
	sessions: Session[];
	activeSessionId: string | null;
	onSelectSession: (session: Session) => void;
}) {
	return (
		<ul className="ml-9 space-y-1 border-l border-white/8 pl-2">
			{sessions.slice(0, 12).map((s) => (
				<li key={s.id}>
					<Link
						to={`/sessions/${s.id}`}
						onClick={() => onSelectSession(s)}
						className={cn(
							"group flex items-start gap-2 rounded-xl px-2 py-1.5 text-sm transition-colors hover:bg-white/7",
							activeSessionId === s.id
								? "bg-white/10 text-neutral-50"
								: "text-neutral-500 hover:text-neutral-100",
						)}
						title={`${s.title || "Untitled session"} · ${project.path}`}
					>
						<MessageSquare className="mt-0.5 size-3.5 shrink-0 opacity-70" />
						<span className="min-w-0 flex-1">
							<span className="block truncate">
								{s.title || "Untitled session"}
							</span>
							<span className="block truncate text-[11px] text-neutral-600">
								{formatDate(s.updated_at)}
							</span>
						</span>
					</Link>
				</li>
			))}
			{sessions.length === 0 && (
				<li className="px-2 py-2 text-xs text-neutral-600">No chats yet.</li>
			)}
		</ul>
	);
}
