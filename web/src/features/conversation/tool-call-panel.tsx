import { api } from "@/lib/api";
import { sse } from "@/lib/sse";
import { cn, formatTime } from "@/lib/utils";
import type { SSEEvent, ToolCall } from "@/types";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";

export function ToolCallPanel({ sessionId }: { sessionId: string }) {
	const { data: calls = [] } = useQuery({
		queryKey: ["tool-calls", sessionId],
		queryFn: () => api.sessions.toolCalls(sessionId),
	});
	const queryClient = useQueryClient();

	useEffect(() => {
		const unsub = sse.on("tool_call", (ev: SSEEvent) => {
			const call = ev.payload as ToolCall;
			if (call.session_id !== sessionId) return;
			queryClient.invalidateQueries({ queryKey: ["tool-calls", sessionId] });
		});
		return () => {
			unsub();
		};
	}, [sessionId, queryClient]);

	if (calls.length === 0) return null;

	return (
		<div className="border-b border-border/70 py-4">
			<p className="mb-3 px-1 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
				Tool Calls
			</p>
			<div className="space-y-2">
				{calls.map((c) => (
					<div
						key={c.id}
						className="rounded-2xl border border-border/70 bg-card/50 p-3 text-xs"
					>
						<div className="font-medium truncate">{c.name}</div>
						<div
							className={cn(
								"mt-2 inline-block rounded-lg px-2 py-1",
								c.status === "succeeded" &&
									"bg-emerald-50 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300",
								c.status === "failed" &&
									"bg-red-50 text-red-700 dark:bg-red-950 dark:text-red-300",
								c.status === "pending" &&
									"bg-amber-50 text-amber-700 dark:bg-amber-950 dark:text-amber-300",
								c.status === "running" &&
									"bg-blue-50 text-blue-700 dark:bg-blue-950 dark:text-blue-300",
							)}
						>
							{c.status}
						</div>
						<div className="mt-2 text-[10px] text-muted-foreground">
							{formatTime(c.started_at)}
						</div>
						{c.error && (
							<div className="mt-1 truncate text-[10px] text-red-500">
								{c.error}
							</div>
						)}
					</div>
				))}
			</div>
		</div>
	);
}
