import { Button } from "@/components/ui/button";
import { api } from "@/lib/api";
import { useStore } from "@/lib/store";
import { cn, formatTime } from "@/lib/utils";
import { useQuery } from "@tanstack/react-query";
import {
	Activity,
	BarChart3,
	Circle,
	PanelLeftClose,
	PanelLeftOpen,
	Settings,
	Workflow,
} from "lucide-react";
import { Link } from "react-router-dom";

export function StatusBar() {
	const { data: health } = useQuery({
		queryKey: ["health"],
		queryFn: api.health,
		refetchInterval: 30_000,
	});
	const { theme, setTheme, sidebarOpen, toggleSidebar } = useStore();

	return (
		<header className="pointer-events-none absolute left-4 right-4 top-3 z-20 flex h-9 items-center justify-between">
			<Button
				onClick={toggleSidebar}
				variant="ghost"
				size="icon"
				className="pointer-events-auto size-8 rounded-lg border border-border/60 bg-background/55 text-muted-foreground backdrop-blur hover:bg-accent"
				aria-label={sidebarOpen ? "Collapse sidebar" : "Expand sidebar"}
				title={sidebarOpen ? "Collapse sidebar" : "Expand sidebar"}
			>
				{sidebarOpen ? (
					<PanelLeftClose className="size-4" />
				) : (
					<PanelLeftOpen className="size-4" />
				)}
			</Button>
			<div className="pointer-events-auto flex items-center gap-1 rounded-xl border border-border/60 bg-background/55 p-1 text-muted-foreground shadow-lg shadow-black/10 backdrop-blur-xl">
				<Link
					to="/dashboard"
					className="inline-flex size-7 items-center justify-center rounded-lg hover:bg-accent hover:text-accent-foreground"
					title="Dashboard"
				>
					<BarChart3 className="size-3.5" />
				</Link>
				<Link
					to="/workflow"
					className="inline-flex size-7 items-center justify-center rounded-lg hover:bg-accent hover:text-accent-foreground"
					title="Workflow"
				>
					<Workflow className="size-3.5" />
				</Link>
				<Link
					to="/diagnostics"
					className="inline-flex size-7 items-center justify-center rounded-lg hover:bg-accent hover:text-accent-foreground"
					title="Diagnostics"
				>
					<Activity className="size-3.5" />
				</Link>
				<Link
					to="/settings"
					className="inline-flex size-7 items-center justify-center rounded-lg hover:bg-accent hover:text-accent-foreground"
					title="Settings"
				>
					<Settings className="size-3.5" />
				</Link>
				<span
					className={cn(
						"mx-1 inline-flex items-center gap-1 rounded-md px-2 text-[11px]",
						health?.status === "ok" ? "text-emerald-400" : "text-amber-400",
					)}
					title={
						health
							? `v${health.version} · ${health.daemon_mode} · started ${formatTime(health.started_at)}`
							: "Unknown"
					}
				>
					<Circle className="size-2 fill-current" />
					{health?.daemon_mode ?? "--"}
				</span>
				<select
					value={theme}
					onChange={(e) =>
						setTheme(e.target.value as "light" | "dark" | "system")
					}
					className="h-7 rounded-md border border-border/70 bg-transparent px-1 text-[11px] outline-none"
					aria-label="Theme"
				>
					<option value="system">System</option>
					<option value="light">Light</option>
					<option value="dark">Dark</option>
				</select>
			</div>
		</header>
	);
}
