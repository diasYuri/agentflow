import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ApprovalPanel } from "@/features/conversation/approval-panel";
import { Composer } from "@/features/conversation/composer";
import { SessionDiagnostics } from "@/features/conversation/session-diagnostics";
import { Timeline } from "@/features/conversation/timeline";
import { ToolCallPanel } from "@/features/conversation/tool-call-panel";
import { api } from "@/lib/api";
import { sse } from "@/lib/sse";
import { useQuery } from "@tanstack/react-query";
import { Archive, Bot } from "lucide-react";
import { useEffect } from "react";
import { useParams } from "react-router-dom";

export function SessionView() {
	const { id } = useParams<{ id: string }>();
	const { data: session } = useQuery({
		queryKey: ["session", id],
		queryFn: () => api.sessions.get(id ?? ""),
		enabled: !!id,
	});

	useEffect(() => {
		if (!id) return;
		sse.connect(id);
		return () => sse.disconnect();
	}, [id]);

	if (!id)
		return (
			<div className="p-8 text-muted-foreground">No session selected.</div>
		);

	return (
		<div className="flex h-full flex-col">
			<div className="pointer-events-none absolute left-16 right-72 top-3 z-10 hidden items-center justify-center lg:flex">
				<div className="pointer-events-auto flex max-w-xl items-center gap-3 rounded-xl border border-border/60 bg-background/55 px-3 py-2 shadow-lg shadow-black/10 backdrop-blur-xl">
					<div className="flex size-7 items-center justify-center rounded-lg bg-primary text-primary-foreground">
						<Bot className="size-3.5" />
					</div>
					<div className="min-w-0">
						<h2 className="truncate text-sm font-medium">
							{session?.title || "Untitled session"}
						</h2>
						<p className="truncate text-[11px] text-muted-foreground">
							{session?.project_name} · {session?.provider || "default"}
							{session?.model ? ` · ${session.model}` : ""}
						</p>
					</div>
					<Badge variant="secondary" className="rounded-lg font-normal">
						{session?.status ?? "loading"}
					</Badge>
					{session?.status === "open" && (
						<Button
							onClick={() => api.sessions.patch(id, { status: "archived" })}
							variant="ghost"
							size="icon"
							className="size-7 rounded-lg"
							aria-label="Archive session"
						>
							<Archive className="size-3.5" />
						</Button>
					)}
				</div>
			</div>
			<div className="flex min-h-0 flex-1">
				<div className="flex min-w-0 flex-1 flex-col">
					<Timeline sessionId={id} />
					<Composer sessionId={id} />
				</div>
				<div className="hidden w-72 shrink-0 overflow-auto border-l border-border/60 bg-background/35 px-3 pb-4 pt-16 backdrop-blur-xl lg:block">
					<ToolCallPanel sessionId={id} />
					<ApprovalPanel sessionId={id} />
					<SessionDiagnostics sessionId={id} />
				</div>
			</div>
		</div>
	);
}
