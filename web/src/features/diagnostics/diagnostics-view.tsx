import { api } from "@/lib/api";
import { cn, formatDate, formatTime } from "@/lib/utils";
import { useQuery } from "@tanstack/react-query";
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
		<div className="h-full flex flex-col">
			<div className="px-4 py-3 border-b border-neutral-100 dark:border-neutral-800 shrink-0">
				<h2 className="text-sm font-semibold">Diagnostics</h2>
				<div className="mt-2 flex flex-wrap gap-2">
					<input
						type="search"
						placeholder="Search..."
						value={search}
						onChange={(e) => setSearch(e.target.value)}
						className="text-xs px-2 py-1 rounded border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800 outline-none focus:border-neutral-400"
					/>
					{(["all", "error", "warning", "info", "debug"] as LevelFilter[]).map(
						(level) => (
							<button
								type="button"
								key={level}
								onClick={() => setFilter(level)}
								className={cn(
									"text-xs px-2 py-1 rounded border capitalize",
									filter === level
										? "bg-neutral-900 text-white border-neutral-900 dark:bg-neutral-100 dark:text-neutral-900 dark:border-neutral-100"
										: "border-neutral-200 dark:border-neutral-700 hover:bg-neutral-50 dark:hover:bg-neutral-800",
								)}
							>
								{level} ({counts[level]})
							</button>
						),
					)}
				</div>
			</div>
			<div className="flex-1 overflow-auto">
				<table className="w-full text-xs">
					<thead className="sticky top-0 bg-neutral-50 dark:bg-neutral-950">
						<tr className="text-left border-b border-neutral-100 dark:border-neutral-800">
							<th className="px-4 py-2 font-semibold">Time</th>
							<th className="px-4 py-2 font-semibold">Level</th>
							<th className="px-4 py-2 font-semibold">Source</th>
							<th className="px-4 py-2 font-semibold">Message</th>
							<th className="px-4 py-2 font-semibold">Session</th>
						</tr>
					</thead>
					<tbody className="divide-y divide-neutral-50 dark:divide-neutral-800">
						{filtered.map((d) => (
							<tr
								key={d.id}
								className="hover:bg-neutral-50 dark:hover:bg-neutral-800/50"
							>
								<td className="px-4 py-2 whitespace-nowrap text-neutral-400">
									{formatDate(d.created_at)} {formatTime(d.created_at)}
								</td>
								<td className="px-4 py-2">
									<span
										className={cn(
											"px-1 py-0.5 rounded text-[10px] uppercase font-semibold",
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
								<td className="px-4 py-2 font-mono">{d.source}</td>
								<td className="px-4 py-2">{d.message}</td>
								<td className="px-4 py-2 text-neutral-400 font-mono truncate max-w-[120px]">
									{d.session_id ?? "—"}
								</td>
							</tr>
						))}
						{filtered.length === 0 && (
							<tr>
								<td
									colSpan={5}
									className="px-4 py-8 text-center text-neutral-400"
								>
									No diagnostics match the filter.
								</td>
							</tr>
						)}
					</tbody>
				</table>
			</div>
		</div>
	);
}
