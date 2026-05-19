import type { SSEEvent } from "@/types";
import { getToken } from "./utils";

export class SSEClient {
	private es: EventSource | null = null;
	private listeners = new Map<string, Set<(ev: SSEEvent) => void>>();

	connect(sessionId?: string, sinceSequence?: number) {
		this.disconnect();
		const token = getToken();
		const params = new URLSearchParams();
		if (token) params.set("token", token);
		if (sinceSequence) params.set("since_sequence", String(sinceSequence));
		const path = sessionId
			? `/api/v1/sessions/${sessionId}/stream?${params}`
			: `/api/v1/stream?${params}`;
		this.es = new EventSource(path);
		this.es.onmessage = (e) => {
			try {
				const data = JSON.parse(e.data) as SSEEvent;
				this.emit(data.kind, data);
				this.emit("*", data);
			} catch {
				// ignore malformed
			}
		};
		this.es.onerror = () => {
			// auto-reconnect is handled by EventSource
		};
	}

	disconnect() {
		if (this.es) {
			this.es.close();
			this.es = null;
		}
	}

	on(kind: string, handler: (ev: SSEEvent) => void) {
		if (!this.listeners.has(kind)) this.listeners.set(kind, new Set());
		this.listeners.get(kind)?.add(handler);
		return () => this.listeners.get(kind)?.delete(handler);
	}

	private emit(kind: string, ev: SSEEvent) {
		const handlers = this.listeners.get(kind);
		if (!handlers) return;
		for (const handler of handlers) {
			handler(ev);
		}
	}
}

export const sse = new SSEClient();
