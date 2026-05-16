import type { RunEvent } from "../api/types.js";

interface RunTimelineProps {
	events: RunEvent[];
}

export function RunTimeline({ events }: RunTimelineProps) {
	if (events.length === 0) {
		return (
			<div className="empty-state">
				<div className="empty-title">Waiting for events</div>
				<div className="empty-subtitle">
					Events will appear as the run progresses.
				</div>
			</div>
		);
	}

	return (
		<div className="timeline">
			{events.map((e) => (
				<div
					key={e.cursor}
					className={`timeline-item type-${e.type.replace(/\./g, "-")}`}
				>
					<div className="timeline-dot" />
					<div className="timeline-content">
						<div className="timeline-header">
							<span className="timeline-type">{e.type}</span>
							{e.node_id && <span className="timeline-node">{e.node_id}</span>}
							{e.attempt != null && (
								<span className="timeline-attempt">attempt {e.attempt}</span>
							)}
							<span className="timeline-time">
								{new Date(e.timestamp).toLocaleTimeString()}
							</span>
						</div>
						{e.data && Object.keys(e.data).length > 0 && (
							<pre className="timeline-data">
								{JSON.stringify(e.data, null, 2)}
							</pre>
						)}
					</div>
				</div>
			))}
		</div>
	);
}
