interface LogConsoleProps {
	lines: string[];
}

export function LogConsole({ lines }: LogConsoleProps) {
	if (lines.length === 0) {
		return (
			<div className="empty-state">
				<div className="empty-title">No logs yet</div>
				<div className="empty-subtitle">
					Logs will appear as the run progresses.
				</div>
			</div>
		);
	}

	return (
		<div className="log-console">
			{lines.map((line, i) => (
				<div key={i} className="log-line">
					<span className="log-index">{i + 1}</span>
					<span className="log-text">{line}</span>
				</div>
			))}
		</div>
	);
}
