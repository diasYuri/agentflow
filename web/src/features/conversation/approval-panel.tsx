import { api } from "@/lib/api";
import { sse } from "@/lib/sse";
import { cn } from "@/lib/utils";
import type { Approval, SSEEvent } from "@/types";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";

export function ApprovalPanel({ sessionId }: { sessionId: string }) {
	const { data: approvals = [] } = useQuery({
		queryKey: ["approvals", sessionId],
		queryFn: () => api.sessions.approvals(sessionId),
	});
	const queryClient = useQueryClient();
	const [deciding, setDeciding] = useState<string | null>(null);

	useEffect(() => {
		const unsub = sse.on("approval", (ev: SSEEvent) => {
			const a = ev.payload as Approval;
			if (a.session_id !== sessionId) return;
			queryClient.invalidateQueries({ queryKey: ["approvals", sessionId] });
		});
		return () => {
			unsub();
		};
	}, [sessionId, queryClient]);

	const pending = approvals.filter((a) => a.status === "pending");
	if (pending.length === 0 && approvals.length === 0) return null;

	const decide = async (id: string, status: "approved" | "rejected") => {
		setDeciding(id);
		try {
			await api.approvals.decide(id, { status, decided_by: "web-ui" });
			queryClient.invalidateQueries({ queryKey: ["approvals", sessionId] });
		} catch (err) {
			alert(String(err));
		} finally {
			setDeciding(null);
		}
	};

	return (
		<div className="border-b border-border/70 py-4">
			<p className="mb-3 px-1 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
				Approvals
			</p>
			<div className="space-y-2">
				{approvals.map((a) => (
					<div
						key={a.id}
						className="rounded-2xl border border-border/70 bg-card/50 p-3 text-xs"
					>
						<div
							className={cn(
								"font-medium",
								a.status === "approved" && "text-emerald-600",
								a.status === "rejected" && "text-red-600",
							)}
						>
							{a.status}
						</div>
						{a.reason && (
							<div className="truncate text-muted-foreground">{a.reason}</div>
						)}
						{a.status === "pending" && (
							<div className="mt-2 flex gap-1">
								<button
									type="button"
									onClick={() => decide(a.id, "approved")}
									disabled={deciding === a.id}
									className="rounded-lg bg-emerald-600 px-2 py-1 text-[10px] text-white disabled:opacity-50"
								>
									Approve
								</button>
								<button
									type="button"
									onClick={() => decide(a.id, "rejected")}
									disabled={deciding === a.id}
									className="rounded-lg bg-red-600 px-2 py-1 text-[10px] text-white disabled:opacity-50"
								>
									Reject
								</button>
							</div>
						)}
					</div>
				))}
			</div>
		</div>
	);
}
