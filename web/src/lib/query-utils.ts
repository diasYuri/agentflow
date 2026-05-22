import type { QueryClient } from "@tanstack/react-query";

export function invalidateProjectSessionLists(
	queryClient: QueryClient,
	projectName?: string | null,
) {
	const tasks = [
		queryClient.invalidateQueries({ queryKey: ["sessions", "all"] }),
	];
	if (projectName) {
		tasks.push(
			queryClient.invalidateQueries({ queryKey: ["sessions", projectName] }),
		);
	}
	return Promise.all(tasks);
}

export function invalidateConversationQueries(
	queryClient: QueryClient,
	sessionId: string,
	projectName?: string | null,
) {
	const tasks = [
		queryClient.invalidateQueries({ queryKey: ["session", sessionId] }),
		queryClient.invalidateQueries({ queryKey: ["tool-calls", sessionId] }),
		queryClient.invalidateQueries({ queryKey: ["sessions", "all"] }),
	];
	if (projectName) {
		tasks.push(
			queryClient.invalidateQueries({ queryKey: ["sessions", projectName] }),
		);
	}
	return Promise.all(tasks);
}
