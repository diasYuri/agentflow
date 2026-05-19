import { api } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useQuery } from "@tanstack/react-query";

export function SessionDiagnostics({ sessionId }: { sessionId: string }) {
	const { data: diags = [] } = useQuery({
		queryKey: ["diagnostics", sessionId],
		queryFn: () => api.sessions.diagnostics(sessionId),
	});

	if (diags.length === 0) return null;

	const lastFive = diags.slice(-5);

	return (
		<div className="py-4">
			<p className="mb-3 px-1 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
				Diagnostics
			</p>
			<div className="space-y-1">
				{lastFive.map((d) => (
					<div
						key={d.id}
						className={cn(
							"rounded-xl px-2 py-1.5 text-[10px]",
							d.level === "error" &&
								"bg-red-50 text-red-700 dark:bg-red-950 dark:text-red-300",
							d.level === "warning" &&
								"bg-amber-50 text-amber-700 dark:bg-amber-950 dark:text-amber-300",
							d.level === "info" &&
								"bg-blue-50 text-blue-700 dark:bg-blue-950 dark:text-blue-300",
							d.level === "debug" &&
								"bg-neutral-50 text-neutral-500 dark:bg-neutral-900 dark:text-neutral-400",
						)}
					>
						<span className="font-semibold">{d.source}</span>: {d.message}
					</div>
				))}
			</div>
		</div>
	);
}
