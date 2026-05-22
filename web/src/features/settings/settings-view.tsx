import { api } from "@/lib/api";
import { clearPersistedToken } from "@/lib/utils";
import { useStore } from "@/lib/store";
import { useQuery } from "@tanstack/react-query";
import { Settings } from "lucide-react";

export function SettingsView() {
	const { data: settings } = useQuery({
		queryKey: ["settings"],
		queryFn: api.settings,
	});
	const { theme, reducedMotion, setTheme, setReducedMotion } = useStore();

	return (
		<div className="flex h-full flex-col overflow-auto px-6 pb-10 pt-20">
			<div className="mx-auto w-full max-w-3xl">
				<header className="mb-6 flex items-center gap-3">
					<div className="flex size-10 items-center justify-center rounded-2xl border border-border bg-background/70 shadow-xl shadow-black/10">
						<Settings className="size-5" />
					</div>
					<div>
						<h1 className="text-xl font-medium tracking-tight">Settings</h1>
						<p className="text-sm text-muted-foreground">
							Local web server and interface preferences.
						</p>
					</div>
				</header>

				<section className="rounded-[24px] border border-border/80 bg-card/80 p-5 shadow-2xl shadow-black/10 backdrop-blur-xl">
					<h2 className="text-sm font-medium text-muted-foreground">
						Appearance
					</h2>
					<div className="mt-4 space-y-4">
						<div className="flex items-center justify-between">
							<label className="text-sm" htmlFor="theme-select">
								Theme
							</label>
							<select
								id="theme-select"
								value={theme}
								onChange={(e) =>
									setTheme(e.target.value as "light" | "dark" | "system")
								}
								className="h-9 rounded-xl border border-border bg-background/55 px-3 text-sm outline-none"
							>
								<option value="system">System</option>
								<option value="light">Light</option>
								<option value="dark">Dark</option>
							</select>
						</div>
						<div className="flex items-center justify-between">
							<span className="text-sm">Reduced Motion</span>
							<button
								type="button"
								onClick={() => setReducedMotion(!reducedMotion)}
								className={`relative h-5 w-10 rounded-full transition-colors ${
									reducedMotion ? "bg-primary" : "bg-muted-foreground/35"
								}`}
								aria-pressed={reducedMotion}
								aria-label="Toggle reduced motion"
							>
								<span
									className={`absolute left-0.5 top-0.5 h-4 w-4 rounded-full bg-background transition-transform ${
										reducedMotion ? "translate-x-5" : ""
									}`}
								/>
							</button>
						</div>
					</div>
				</section>

				<section className="mt-4 rounded-[24px] border border-border/80 bg-card/80 p-5 shadow-2xl shadow-black/10 backdrop-blur-xl">
					<h2 className="text-sm font-medium text-muted-foreground">Server</h2>
					<dl className="mt-3 grid grid-cols-[120px_1fr] gap-y-2 text-sm">
						<dt className="text-muted-foreground">Host</dt>
						<dd>{settings?.web.host}</dd>
						<dt className="text-muted-foreground">Port</dt>
						<dd>{settings?.web.port}</dd>
						<dt className="text-muted-foreground">Daemon</dt>
						<dd className="capitalize">{settings?.web.daemon}</dd>
						<dt className="text-muted-foreground">Root</dt>
						<dd className="font-mono text-xs">{settings?.paths.root}</dd>
						{settings?.paths.daemon_socket && (
							<>
								<dt className="text-muted-foreground">Socket</dt>
								<dd className="font-mono text-xs">
									{settings.paths.daemon_socket}
								</dd>
							</>
						)}
					</dl>
				</section>

				<section className="mt-4 rounded-[24px] border border-border/80 bg-card/80 p-5 shadow-2xl shadow-black/10 backdrop-blur-xl">
					<h2 className="text-sm font-medium text-muted-foreground">
						Session Token
					</h2>
					<p className="mt-2 text-xs text-muted-foreground">
						The token is stored in localStorage and mirrored into a cookie so
						deep links keep working on reload.
					</p>
					<button
						type="button"
						onClick={() => {
							clearPersistedToken();
							window.location.reload();
						}}
						className="mt-3 rounded-xl border border-red-500/30 px-3 py-1.5 text-xs text-red-600 transition-colors hover:bg-red-500/10 dark:text-red-400"
					>
						Clear Token & Reload
					</button>
				</section>
			</div>
		</div>
	);
}
