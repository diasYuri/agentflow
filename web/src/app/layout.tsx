import { ProjectRail } from "@/components/project-rail";
import { StatusBar } from "@/components/status-bar";
import { useStore } from "@/lib/store";
import { useEffect } from "react";
import { Outlet } from "react-router-dom";

export function Layout() {
	const { theme } = useStore();

	useEffect(() => {
		const root = document.documentElement;
		if (
			theme === "dark" ||
			(theme === "system" &&
				window.matchMedia("(prefers-color-scheme: dark)").matches)
		) {
			root.classList.add("dark");
		} else {
			root.classList.remove("dark");
		}
	}, [theme]);

	return (
		<div className="h-screen overflow-hidden bg-background text-foreground">
			<div className="flex h-full">
				<ProjectRail />
				<main className="relative flex-1 overflow-hidden rounded-tl-[24px] border-l border-t border-border/70 bg-[radial-gradient(circle_at_50%_28%,color-mix(in_oklch,var(--muted),transparent_72%),transparent_32%),linear-gradient(180deg,var(--background),color-mix(in_oklch,var(--background),black_4%))] shadow-2xl shadow-black/20">
					<StatusBar />
					<Outlet />
				</main>
			</div>
		</div>
	);
}
