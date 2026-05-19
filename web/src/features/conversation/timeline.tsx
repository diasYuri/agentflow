import { api } from "@/lib/api";
import { sse } from "@/lib/sse";
import type { Message, SSEEvent } from "@/types";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import { MessageCard } from "./message-card";

export function Timeline({ sessionId }: { sessionId: string }) {
	const { data: messages = [] } = useQuery({
		queryKey: ["messages", sessionId],
		queryFn: () => api.sessions.messages(sessionId),
	});
	const queryClient = useQueryClient();
	const bottomRef = useRef<HTMLDivElement>(null);
	const containerRef = useRef<HTMLDivElement>(null);

	useEffect(() => {
		const unsubscribe = sse.on("message", (ev: SSEEvent) => {
			const msg = ev.payload as Message;
			if (msg.session_id !== sessionId) return;
			queryClient.setQueryData<Message[]>(["messages", sessionId], (old) => {
				if (!old) return [msg];
				const exists = old.some((m) => m.id === msg.id);
				if (exists) {
					return old.map((m) => (m.id === msg.id ? msg : m));
				}
				return [...old, msg];
			});
		});
		return () => {
			unsubscribe();
		};
	}, [sessionId, queryClient]);

	useEffect(() => {
		if (containerRef.current) {
			const el = containerRef.current;
			const nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 100;
			if (nearBottom) {
				bottomRef.current?.scrollIntoView({ behavior: "smooth" });
			}
		}
	});

	return (
		<div ref={containerRef} className="flex-1 overflow-y-auto px-4 pb-4 pt-20">
			<div className="mx-auto max-w-3xl space-y-5">
				{messages.length === 0 && (
					<div className="pt-32 text-center text-sm text-muted-foreground">
						No messages yet. Start with the composer below.
					</div>
				)}
				{messages.map((m) => (
					<MessageCard key={m.id} message={m} />
				))}
			</div>
			<div ref={bottomRef} />
		</div>
	);
}
