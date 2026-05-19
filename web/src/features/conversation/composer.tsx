import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { api } from "@/lib/api";
import { ArrowUp, AtSign, ChevronDown, Mic } from "lucide-react";
import { useRef, useState } from "react";

export function Composer({ sessionId }: { sessionId: string }) {
	const [content, setContent] = useState("");
	const [sending, setSending] = useState(false);
	const textareaRef = useRef<HTMLTextAreaElement>(null);

	const submit = async () => {
		const text = content.trim();
		if (!text || sending) return;
		setSending(true);
		try {
			await api.sessions.appendMessage(sessionId, {
				role: "user",
				content: text,
			});
			setContent("");
			textareaRef.current?.focus();
		} catch (err) {
			alert(String(err));
		} finally {
			setSending(false);
		}
	};

	const handleKeyDown = (e: React.KeyboardEvent) => {
		if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
			e.preventDefault();
			submit();
		}
	};

	return (
		<div className="shrink-0 px-4 pb-5 pt-2">
			<div className="mx-auto max-w-3xl overflow-hidden rounded-[24px] border border-border/80 bg-card/90 shadow-2xl shadow-black/20 backdrop-blur-xl">
				<Textarea
					ref={textareaRef}
					value={content}
					onChange={(e) => setContent(e.target.value)}
					onKeyDown={handleKeyDown}
					placeholder="Ask AgentFlow anything. @ to mention files or context"
					rows={2}
					className="min-h-[82px] resize-none border-0 bg-transparent px-5 py-4 text-[15px] shadow-none focus-visible:ring-0"
				/>
				<div className="flex items-center justify-between px-4 pb-4">
					<div className="flex items-center gap-1 text-xs text-muted-foreground">
						<Button variant="ghost" size="icon" className="size-8 rounded-xl">
							<AtSign className="size-4" />
						</Button>
						<button
							type="button"
							className="flex items-center gap-1 rounded-xl px-2 py-1.5 hover:bg-accent"
						>
							Default permissions
							<ChevronDown className="size-3" />
						</button>
					</div>
					<div className="flex items-center gap-2">
						<Button
							variant="ghost"
							size="icon"
							className="size-8 rounded-xl text-muted-foreground"
						>
							<Mic className="size-4" />
						</Button>
						<Button
							onClick={submit}
							disabled={sending || !content.trim()}
							size="icon"
							className="size-9 rounded-full"
							aria-label="Send message"
						>
							<ArrowUp className="size-4" />
						</Button>
					</div>
				</div>
			</div>
		</div>
	);
}
