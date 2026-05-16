import { useCallback, useState } from "react";
import * as api from "../api/bindings.js";
import type { ArtifactInfo } from "../api/types.js";

interface ArtifactsPaneProps {
	artifacts: ArtifactInfo[];
	runId: string;
}

export function ArtifactsPane({ artifacts, runId }: ArtifactsPaneProps) {
	const [preview, setPreview] = useState<{
		info: ArtifactInfo;
		content: string;
	} | null>(null);
	const [busy, setBusy] = useState(false);

	const openArtifact = useCallback(
		async (info: ArtifactInfo) => {
			setBusy(true);
			try {
				const resp = await api.getRunArtifact(runId, info.id);
				if (resp.encoding === "base64") {
					const isText =
						(resp.content_type ?? "").startsWith("text/") ||
						resp.size < 128 * 1024;
					if (isText) {
						try {
							const decoded = atob(resp.content);
							setPreview({ info, content: decoded });
						} catch {
							setPreview({
								info,
								content: "[Unable to decode base64 content]",
							});
						}
					} else {
						setPreview({
							info,
							content: "[Binary file — preview not available]",
						});
					}
				} else {
					setPreview({ info, content: resp.content });
				}
			} catch (err) {
				setPreview({
					info,
					content: err instanceof Error ? err.message : String(err),
				});
			} finally {
				setBusy(false);
			}
		},
		[runId],
	);

	if (artifacts.length === 0) {
		return (
			<div className="empty-state">
				<div className="empty-title">No artifacts</div>
				<div className="empty-subtitle">
					Artifacts will appear as nodes complete.
				</div>
			</div>
		);
	}

	return (
		<div className="artifacts-pane">
			<div className="artifacts-list">
				{artifacts.map((a) => (
					<button
						key={a.id}
						className="artifact-item"
						onClick={() => openArtifact(a)}
						disabled={busy}
					>
						<span className="artifact-name">{a.name}</span>
						<span className="artifact-meta">
							{a.content_type} · {formatBytes(a.size)}
						</span>
					</button>
				))}
			</div>
			{preview && (
				<div className="artifact-preview">
					<div className="artifact-preview-header">
						<span className="artifact-preview-title">{preview.info.name}</span>
						<button
							className="artifact-preview-close"
							onClick={() => setPreview(null)}
						>
							Close
						</button>
					</div>
					<pre className="artifact-preview-content">{preview.content}</pre>
				</div>
			)}
		</div>
	);
}

function formatBytes(n: number): string {
	if (n < 1024) return `${n} B`;
	if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
	return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}
