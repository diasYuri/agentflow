import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { api } from "@/lib/api";
import { invalidateProjectSessionLists } from "@/lib/query-utils";
import { useStore } from "@/lib/store";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
	ArrowUp,
	Bot,
	ChevronDown,
	GitBranch,
	MessageSquare,
	ShieldAlert,
	Sparkles,
} from "lucide-react";
import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";

export function ProjectView() {
	const params = useParams();
	const projectName = params.name ?? useStore.getState().selectedProject;
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const { data: project } = useQuery({
		queryKey: ["project", projectName],
		queryFn: () => (projectName ? api.projects.get(projectName) : null),
		enabled: !!projectName,
	});
	const { data: sessions = [] } = useQuery({
		queryKey: ["sessions", projectName],
		queryFn: () =>
			projectName ? api.projects.sessions(projectName) : api.sessions.list(),
		enabled: !!projectName,
	});
	const { data: projects = [] } = useQuery({
		queryKey: ["projects"],
		queryFn: api.projects.list,
	});
	const { data: diagnostics = [] } = useQuery({
		queryKey: ["diagnostics", "recent", 20],
		queryFn: () => api.diagnostics.recent(20),
	});
	const [isCreating, setIsCreating] = useState(false);
	const [title, setTitle] = useState("");
	const [prompt, setPrompt] = useState("");

	const handleCreate = async (e: React.FormEvent) => {
		e.preventDefault();
		if (!title.trim() || !projectName) return;
		setIsCreating(true);
			try {
				const session = await api.projects.createSession(projectName, {
					title: title.trim(),
				});
				await invalidateProjectSessionLists(queryClient, projectName);
				setTitle("");
				navigate(`/sessions/${session.id}`);
		} catch (err) {
			alert(String(err));
		} finally {
			setIsCreating(false);
		}
	};

	const startChat = async () => {
		const text = prompt.trim();
		if (!text || !projectName || isCreating) return;
		setIsCreating(true);
			try {
				const session = await api.projects.createSession(projectName, {
					title: text.slice(0, 80),
				});
				await api.sessions.appendMessage(session.id, {
					role: "user",
					content: text,
				});
				await invalidateProjectSessionLists(queryClient, projectName);
				setPrompt("");
				navigate(`/sessions/${session.id}`);
		} catch (err) {
			alert(String(err));
		} finally {
			setIsCreating(false);
		}
	};

	const handlePromptKeyDown = (e: React.KeyboardEvent) => {
		if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
			e.preventDefault();
			startChat();
		}
	};

	const pendingDiagnostics = diagnostics.filter(
		(d) => d.level === "error" || d.level === "warning",
	).length;

	return (
		<div className="flex h-full flex-col overflow-auto px-6 pb-10 pt-20">
			<div className="mx-auto flex min-h-full w-full max-w-[780px] flex-col justify-center">
				<div className="mb-10 text-center">
					<div className="mb-5 inline-flex size-11 items-center justify-center rounded-2xl border border-border bg-background/70 shadow-xl shadow-black/10">
						<Sparkles className="size-5 text-foreground" />
					</div>
					<h1 className="text-balance text-3xl font-medium tracking-tight text-foreground md:text-[28px]">
						What should we work on in AgentFlow?
					</h1>
					<p className="mt-3 text-sm text-muted-foreground">
						{projectName
							? `Working in ${project?.name ?? projectName}`
							: "Select a project to start a new agent session."}
					</p>
				</div>

				<div className="overflow-hidden rounded-[28px] border border-border/80 bg-card/80 shadow-2xl shadow-black/20 backdrop-blur-xl">
					{pendingDiagnostics > 0 && (
						<Link
							to="/diagnostics"
							className="flex items-center justify-between border-b border-border/70 px-5 py-3 text-xs text-muted-foreground transition-colors hover:bg-accent/60"
						>
							<span className="flex items-center gap-2">
								<ShieldAlert className="size-4 text-amber-400" />
								{pendingDiagnostics} diagnostic item
								{pendingDiagnostics === 1 ? "" : "s"} need review
							</span>
							<span className="font-medium text-foreground">Review</span>
						</Link>
					)}
					<Textarea
						value={prompt}
						onChange={(e) => setPrompt(e.target.value)}
						onKeyDown={handlePromptKeyDown}
						placeholder={
							projectName
								? "Ask AgentFlow anything. @ to mention files or context"
								: "Choose a project from the sidebar first"
						}
						disabled={!projectName || isCreating}
						className="min-h-[96px] resize-none border-0 bg-transparent px-5 py-4 text-[15px] shadow-none focus-visible:ring-0"
					/>
					<div className="flex items-center justify-between px-4 pb-4">
						<div className="flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
							<Button variant="ghost" size="icon" className="size-8 rounded-xl">
								<GitBranch className="size-4" />
							</Button>
							<button
								type="button"
								className="flex max-w-[260px] items-center gap-2 truncate rounded-xl px-2 py-1.5 hover:bg-accent"
							>
								<span className="truncate">
									{projectName ?? "No project selected"}
								</span>
								<ChevronDown className="size-3" />
							</button>
						</div>
						<div className="flex items-center gap-2">
							<Badge
								variant="secondary"
								className="rounded-lg border-border/70 bg-secondary/80 font-normal"
							>
								Default model
							</Badge>
							<Button
								onClick={startChat}
								disabled={!prompt.trim() || !projectName || isCreating}
								size="icon"
								className="size-9 rounded-full"
								aria-label="Start chat"
							>
								<ArrowUp className="size-4" />
							</Button>
						</div>
					</div>
				</div>

				{projectName && (
					<form onSubmit={handleCreate} className="mt-5 flex gap-2">
						<input
							type="text"
							placeholder="Create an empty session..."
							value={title}
							onChange={(e) => setTitle(e.target.value)}
							className="h-10 flex-1 rounded-2xl border border-border bg-background/55 px-4 text-sm outline-none transition-colors placeholder:text-muted-foreground focus:border-ring"
						/>
						<Button
							type="submit"
							disabled={isCreating || !title.trim()}
							className="rounded-2xl"
						>
							Create Session
						</Button>
					</form>
				)}

				<div className="mt-8 grid gap-2">
					{sessions.slice(0, 4).map((s) => (
						<Link
							key={s.id}
							to={`/sessions/${s.id}`}
							className="group flex items-center gap-3 border-t border-border/70 px-3 py-3 text-sm text-muted-foreground transition-colors hover:text-foreground"
						>
							<MessageSquare className="size-4 opacity-60" />
							<span className="min-w-0 flex-1 truncate">
								{s.title || "Untitled session"}
							</span>
							<span className="text-xs opacity-70">
								{new Date(s.updated_at).toLocaleDateString()}
							</span>
						</Link>
					))}
					{!projectName &&
						projects.slice(0, 4).map((p) => (
							<Link
								key={p.name}
								to={`/projects/${encodeURIComponent(p.name)}`}
								className="group flex items-center gap-3 border-t border-border/70 px-3 py-3 text-sm text-muted-foreground transition-colors hover:text-foreground"
							>
								<Bot className="size-4 opacity-60" />
								<span className="min-w-0 flex-1 truncate">Open {p.name}</span>
								<span className="text-xs opacity-70">Project</span>
							</Link>
						))}
				</div>
			</div>
		</div>
	);
}
