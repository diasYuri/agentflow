import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { api } from "@/lib/api";
import { useStore } from "@/lib/store";
import { cn, formatDate } from "@/lib/utils";
import { useQuery } from "@tanstack/react-query";
import {
	Bot,
	Folder,
	MessageSquare,
	Plus,
	Search,
	Settings,
	Sparkles,
} from "lucide-react";
import { Link, useParams } from "react-router-dom";

export function ProjectRail() {
	const { data: projects = [] } = useQuery({
		queryKey: ["projects"],
		queryFn: api.projects.list,
	});
	const { selectedProject, sidebarOpen, setSelectedProject } = useStore();
	const params = useParams();
	const active = params.name ?? selectedProject ?? null;
	const { data: sessions = [] } = useQuery({
		queryKey: ["sessions", active],
		queryFn: () =>
			active ? api.projects.sessions(active) : api.sessions.list(),
	});

	if (!sidebarOpen) return null;

	return (
		<aside className="flex w-[296px] shrink-0 flex-col bg-[#202326] text-neutral-300 dark:bg-[#202326]">
			<div className="space-y-1 px-3 py-4">
				<Link
					to="/"
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
				<div className="mb-2 mt-3 px-1 text-[13px] text-neutral-500">
					Projects
				</div>
				<ul className="space-y-1">
					{projects.map((p) => (
						<li key={p.name}>
							<Link
								to={`/projects/${encodeURIComponent(p.name)}`}
								onClick={() => setSelectedProject(p.name)}
								className={cn(
									"flex items-center gap-2 rounded-xl px-3 py-2 text-sm transition-colors",
									active === p.name
										? "bg-white/10 text-neutral-50"
										: "text-neutral-400 hover:bg-white/7 hover:text-neutral-100",
								)}
								title={p.path}
							>
								<Folder className="size-4 shrink-0" />
								<span className="truncate">{p.name}</span>
							</Link>
						</li>
					))}
					{projects.length === 0 && (
						<li className="px-3 py-4 text-center text-xs text-neutral-500">
							No projects yet.
							<br />
							Use the CLI to add one.
						</li>
					)}
				</ul>

				<div className="mb-2 mt-6 flex items-center justify-between px-1 text-[13px] text-neutral-500">
					<span>Chats</span>
					<Badge
						variant="secondary"
						className="border-white/10 bg-white/5 text-[10px] text-neutral-400"
					>
						{sessions.length}
					</Badge>
				</div>
				<ul className="space-y-1">
					{sessions.slice(0, 12).map((s) => (
						<li key={s.id}>
							<Link
								to={`/sessions/${s.id}`}
								className={cn(
									"group flex items-start gap-2 rounded-xl px-3 py-2 text-sm transition-colors hover:bg-white/7",
									params.id === s.id
										? "bg-white/10 text-neutral-50"
										: "text-neutral-400 hover:text-neutral-100",
								)}
							>
								<MessageSquare className="mt-0.5 size-4 shrink-0 opacity-70" />
								<span className="min-w-0 flex-1">
									<span className="block truncate">
										{s.title || "Untitled session"}
									</span>
									<span className="block truncate text-[11px] text-neutral-500">
										{formatDate(s.updated_at)}
									</span>
								</span>
							</Link>
						</li>
					))}
					{sessions.length === 0 && (
						<li className="px-3 py-3 text-xs text-neutral-500">
							No chats yet.
						</li>
					)}
				</ul>
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
		</aside>
	);
}
