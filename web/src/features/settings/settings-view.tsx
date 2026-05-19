import { api } from "@/lib/api";
import { useStore } from "@/lib/store";
import { useQuery } from "@tanstack/react-query";

export function SettingsView() {
	const { data: settings } = useQuery({
		queryKey: ["settings"],
		queryFn: api.settings,
	});
	const { theme, reducedMotion, setTheme, setReducedMotion } = useStore();

	return (
		<div className="p-6 max-w-2xl">
			<h2 className="text-lg font-semibold">Settings</h2>

			<section className="mt-6">
				<h3 className="text-sm font-semibold text-neutral-500 uppercase tracking-wider">
					Appearance
				</h3>
				<div className="mt-3 space-y-3">
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
							className="text-sm px-2 py-1 rounded border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800"
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
							className={`w-10 h-5 rounded-full relative transition-colors ${
								reducedMotion
									? "bg-neutral-900 dark:bg-neutral-100"
									: "bg-neutral-300 dark:bg-neutral-700"
							}`}
							aria-pressed={reducedMotion}
							aria-label="Toggle reduced motion"
						>
							<span
								className={`absolute top-0.5 left-0.5 w-4 h-4 rounded-full bg-white transition-transform ${
									reducedMotion ? "translate-x-5" : ""
								}`}
							/>
						</button>
					</div>
				</div>
			</section>

			<section className="mt-8">
				<h3 className="text-sm font-semibold text-neutral-500 uppercase tracking-wider">
					Server
				</h3>
				<dl className="mt-3 grid grid-cols-[120px_1fr] gap-y-2 text-sm">
					<dt className="text-neutral-400">Host</dt>
					<dd>{settings?.web.host}</dd>
					<dt className="text-neutral-400">Port</dt>
					<dd>{settings?.web.port}</dd>
					<dt className="text-neutral-400">Daemon</dt>
					<dd className="capitalize">{settings?.web.daemon}</dd>
					<dt className="text-neutral-400">Root</dt>
					<dd className="font-mono text-xs">{settings?.paths.root}</dd>
					{settings?.paths.daemon_socket && (
						<>
							<dt className="text-neutral-400">Socket</dt>
							<dd className="font-mono text-xs">
								{settings.paths.daemon_socket}
							</dd>
						</>
					)}
				</dl>
			</section>

			<section className="mt-8">
				<h3 className="text-sm font-semibold text-neutral-500 uppercase tracking-wider">
					Session Token
				</h3>
				<p className="mt-2 text-xs text-neutral-400">
					The token is stored in localStorage and sent with every request.
				</p>
				<button
					type="button"
					onClick={() => {
						localStorage.removeItem("agentflow_token");
						window.location.reload();
					}}
					className="mt-2 text-xs px-3 py-1.5 rounded border border-red-200 dark:border-red-900 text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-950"
				>
					Clear Token & Reload
				</button>
			</section>
		</div>
	);
}
