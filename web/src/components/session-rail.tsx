import { api } from "@/lib/api";
import { useStore } from "@/lib/store";
import { cn, formatDate, formatTime } from "@/lib/utils";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Link, useParams } from "react-router-dom";

export function SessionRail() {
	const { selectedProject, sidebarOpen, setSelectedSession } = useStore();
	const params = useParams();
	const projectName = params.name ?? selectedProject;
	const activeSession = params.id ?? null;
	const [filter, setFilter] = useState("");

	const { data: sessions = [] } = useQuery({
		queryKey: ["sessions", projectName],
		queryFn: () =>
			projectName ? api.projects.sessions(projectName) : api.sessions.list(),
		enabled: !!projectName || selectedProject === null,
	});

	if (!sidebarOpen) return null;

	const filtered = sessions.filter((s) =>
		s.title.toLowerCase().includes(filter.toLowerCase()),
	);

	return (
		<aside className="flex w-60 shrink-0 flex-col border-r border-border bg-background">
			<div className="border-b border-border px-3 py-2">
				<div className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
					Sessions
				</div>
				<input
					type="search"
					placeholder="Filter sessions..."
					value={filter}
					onChange={(e) => setFilter(e.target.value)}
					className="w-full rounded border border-border bg-background px-2 py-1 text-xs outline-none focus:border-ring"
				/>
			</div>
			<nav className="flex-1 overflow-auto">
				<ul className="divide-y divide-border">
					{filtered.map((s) => (
						<li key={s.id}>
							<Link
								to={`/sessions/${s.id}`}
								onClick={() => setSelectedSession(s.id)}
								className={cn(
									"block border-l-2 px-3 py-2 text-sm transition-colors",
									activeSession === s.id
										? "border-l-foreground bg-accent"
										: "border-l-transparent hover:bg-accent/50",
								)}
							>
								<div className="font-medium truncate">
									{s.title || "Untitled session"}
								</div>
								<div className="mt-0.5 flex items-center gap-2 text-xs text-muted-foreground">
									<span
										className={cn(
											"px-1 rounded text-[10px] uppercase tracking-wide",
											s.status === "open"
												? "bg-emerald-500/10 text-emerald-600 dark:text-emerald-300"
												: "bg-muted text-muted-foreground",
										)}
									>
										{s.status}
									</span>
									<span>
										{formatDate(s.updated_at)} {formatTime(s.updated_at)}
									</span>
								</div>
							</Link>
						</li>
					))}
					{filtered.length === 0 && (
						<li className="px-3 py-4 text-center text-xs text-muted-foreground">
							No sessions.
						</li>
					)}
				</ul>
			</nav>
		</aside>
	);
}
