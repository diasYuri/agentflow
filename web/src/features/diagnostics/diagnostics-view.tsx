import { api } from "@/lib/api";
import { cn, formatDate, formatTime } from "@/lib/utils";
import { useQuery } from "@tanstack/react-query";
import { Activity } from "lucide-react";
import { useState } from "react";

type LevelFilter = "all" | "debug" | "info" | "warning" | "error";

export function DiagnosticsView() {
	const { data: diags = [] } = useQuery({
		queryKey: ["diagnostics-recent"],
		queryFn: () => api.diagnostics.recent(200),
	});
	const [filter, setFilter] = useState<LevelFilter>("all");
	const [search, setSearch] = useState("");

	const filtered = diags.filter((d) => {
		if (filter !== "all" && d.level !== filter) return false;
		if (search && !d.message.toLowerCase().includes(search.toLowerCase()))
			return false;
		return true;
	});

	const counts: Record<LevelFilter, number> = {
		all: diags.length,
		debug: diags.filter((d) => d.level === "debug").length,
		info: diags.filter((d) => d.level === "info").length,
		warning: diags.filter((d) => d.level === "warning").length,
		error: diags.filter((d) => d.level === "error").length,
	};

	return (
		<div className="flex h-full flex-col overflow-hidden px-6 pb-6 pt-20">
			<div className="mx-auto flex min-h-0 w-full max-w-6xl flex-1 flex-col">
				<header className="mb-5 flex flex-wrap items-center gap-3">
					<div className="flex size-10 items-center justify-center rounded-2xl border border-border bg-background/70 shadow-xl shadow-black/10">
						<Activity className="size-5" />
					</div>
					<div className="min-w-0">
						<h1 className="text-xl font-medium tracking-tight">Diagnostics</h1>
						<p className="text-sm text-muted-foreground">
							Recent warnings, errors, and runtime events.
						</p>
					</div>
					<input
						type="search"
						placeholder="Search diagnostics..."
						value={search}
						onChange={(e) => setSearch(e.target.value)}
						className="ml-auto h-9 min-w-52 rounded-2xl border border-border bg-background/55 px-3 text-sm outline-none transition-colors placeholder:text-muted-foreground focus:border-ring"
					/>
				</header>
				<div className="mb-4 flex flex-wrap gap-2">
					{(["all", "error", "warning", "info", "debug"] as LevelFilter[]).map(
						(level) => (
							<button
								type="button"
								key={level}
								onClick={() => setFilter(level)}
								className={cn(
									"rounded-xl border px-3 py-1.5 text-xs capitalize transition-colors",
									filter === level
										? "border-primary bg-primary text-primary-foreground"
										: "border-border/70 bg-background/55 text-muted-foreground hover:bg-accent hover:text-foreground",
								)}
							>
								{level} ({counts[level]})
							</button>
						),
					)}
				</div>
				<div className="min-h-0 flex-1 overflow-hidden rounded-[24px] border border-border/80 bg-card/80 shadow-2xl shadow-black/10 backdrop-blur-xl">
					<table className="w-full text-xs">
						<thead className="sticky top-0 bg-card/95 backdrop-blur">
							<tr className="border-b border-border/70 text-left">
								<th className="px-4 py-3 font-medium">Time</th>
								<th className="px-4 py-3 font-medium">Level</th>
								<th className="px-4 py-3 font-medium">Source</th>
								<th className="px-4 py-3 font-medium">Message</th>
								<th className="px-4 py-3 font-medium">Session</th>
							</tr>
						</thead>
						<tbody className="divide-y divide-border/60">
							{filtered.map((d) => (
								<tr key={d.id} className="transition-colors hover:bg-accent/45">
									<td className="whitespace-nowrap px-4 py-3 text-muted-foreground">
										{formatDate(d.created_at)} {formatTime(d.created_at)}
									</td>
									<td className="px-4 py-3">
										<span
											className={cn(
												"rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase",
												d.level === "error" &&
													"bg-red-100 text-red-700 dark:bg-red-950 dark:text-red-300",
												d.level === "warning" &&
													"bg-amber-100 text-amber-700 dark:bg-amber-950 dark:text-amber-300",
												d.level === "info" &&
													"bg-blue-100 text-blue-700 dark:bg-blue-950 dark:text-blue-300",
												d.level === "debug" &&
													"bg-neutral-100 text-neutral-500 dark:bg-neutral-900 dark:text-neutral-400",
											)}
										>
											{d.level}
										</span>
									</td>
									<td className="px-4 py-3 font-mono">{d.source}</td>
									<td className="px-4 py-3">{d.message}</td>
									<td className="max-w-[120px] truncate px-4 py-3 font-mono text-muted-foreground">
										{d.session_id ?? "—"}
									</td>
								</tr>
							))}
							{filtered.length === 0 && (
								<tr>
									<td
										colSpan={5}
										className="px-4 py-10 text-center text-muted-foreground"
									>
										No diagnostics match the filter.
									</td>
								</tr>
							)}
						</tbody>
					</table>
				</div>
			</div>
		</div>
	);
}
