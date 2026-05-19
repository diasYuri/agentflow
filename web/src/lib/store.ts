import { create } from "zustand";

interface UIState {
	selectedProject: string | null;
	selectedSession: string | null;
	sidebarOpen: boolean;
	theme: "light" | "dark" | "system";
	reducedMotion: boolean;
	setSelectedProject: (name: string | null) => void;
	setSelectedSession: (id: string | null) => void;
	toggleSidebar: () => void;
	setTheme: (t: "light" | "dark" | "system") => void;
	setReducedMotion: (v: boolean) => void;
}

export const useStore = create<UIState>((set) => ({
	selectedProject: null,
	selectedSession: null,
	sidebarOpen: true,
	theme:
		(localStorage.getItem("agentflow_theme") as "light" | "dark" | "system") ??
		"dark",
	reducedMotion: window.matchMedia("(prefers-reduced-motion: reduce)").matches,
	setSelectedProject: (name) =>
		set({ selectedProject: name, selectedSession: null }),
	setSelectedSession: (id) => set({ selectedSession: id }),
	toggleSidebar: () => set((s) => ({ sidebarOpen: !s.sidebarOpen })),
	setTheme: (t) => {
		localStorage.setItem("agentflow_theme", t);
		set({ theme: t });
	},
	setReducedMotion: (v) => set({ reducedMotion: v }),
}));
