import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

const TOKEN_KEY = "agentflow_token";
const SESSION_COOKIE = "agentflow_session";
const TOKEN_COOKIE_MAX_AGE = 60 * 60 * 24 * 365;

export function cn(...inputs: ClassValue[]) {
	return twMerge(clsx(inputs));
}

export function formatTime(iso: string): string {
	const d = new Date(iso);
	return d.toLocaleTimeString(undefined, {
		hour: "2-digit",
		minute: "2-digit",
	});
}

export function formatDate(iso: string): string {
	const d = new Date(iso);
	return d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

export function getToken(): string | null {
	return localStorage.getItem(TOKEN_KEY);
}

export function resolveBootstrapToken(
	search: string,
	storedToken: string | null,
): string | null {
	const queryToken = new URLSearchParams(search).get("token");
	return queryToken ?? storedToken;
}

export function buildSessionCookie(token: string): string {
	return `${SESSION_COOKIE}=${encodeURIComponent(token)}; Path=/; Max-Age=${TOKEN_COOKIE_MAX_AGE}; SameSite=Lax`;
}

export function buildClearedSessionCookie(): string {
	return `${SESSION_COOKIE}=; Path=/; Max-Age=0; SameSite=Lax`;
}

export function persistToken(token: string): void {
	localStorage.setItem(TOKEN_KEY, token);
	document.cookie = buildSessionCookie(token);
}

export function clearPersistedToken(): void {
	localStorage.removeItem(TOKEN_KEY);
	document.cookie = buildClearedSessionCookie();
}
