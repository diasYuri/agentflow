import { cn, formatTime } from "@/lib/utils";
import type { Message } from "@/types";

const roleStyles: Record<string, string> = {
	user: "ml-auto max-w-[78%] rounded-3xl bg-primary px-4 py-3 text-primary-foreground",
	assistant: "mr-auto max-w-full px-1 py-1 text-foreground",
	system:
		"mx-auto max-w-[82%] rounded-2xl border border-amber-500/20 bg-amber-500/10 px-3 py-2 text-xs text-amber-300",
	tool: "mr-auto max-w-[82%] rounded-2xl border border-sky-500/20 bg-sky-500/10 px-3 py-2 text-xs text-sky-300",
};

const roleLabels: Record<string, string> = {
	user: "You",
	assistant: "Assistant",
	system: "System",
	tool: "Tool",
};

export function MessageCard({ message }: { message: Message }) {
	return (
		<div
			className={cn("group", roleStyles[message.role] || roleStyles.assistant)}
		>
			<div className="mb-1 flex items-center gap-2">
				<span className="text-xs font-medium text-muted-foreground">
					{roleLabels[message.role] ?? message.role}
				</span>
				<span className="text-[10px] text-muted-foreground/70">
					{formatTime(message.created_at)}
				</span>
			</div>
			<div className="whitespace-pre-wrap text-[15px] leading-7">
				{message.content ?? "(content offloaded)"}
			</div>
			{message.correlation_id && (
				<div className="mt-2 font-mono text-[10px] text-muted-foreground/70">
					{message.correlation_id}
				</div>
			)}
		</div>
	);
}
