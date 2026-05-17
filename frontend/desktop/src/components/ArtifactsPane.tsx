import { useCallback, useState } from "react";
import * as api from "../api/bindings.js";
import type { ArtifactInfo } from "../api/types.js";

interface ArtifactsPaneProps {
	artifacts: ArtifactInfo[];
	runId: string;
}

interface PreviewState {
	info: ArtifactInfo;
	content: string;
	isText: boolean;
	truncated?: boolean;
}

export function ArtifactsPane({ artifacts, runId }: ArtifactsPaneProps) {
	const [preview, setPreview] = useState<PreviewState | null>(null);
	const [busy, setBusy] = useState(false);

	const openArtifact = useCallback(
		async (info: ArtifactInfo) => {
			setBusy(true);
			try {
				const resp = await api.getRunArtifact(runId, info.id);
				if (resp.is_text) {
					setPreview({
						info,
						content: resp.text_content ?? "",
						isText: true,
						truncated: resp.truncated,
					});
				} else {
					setPreview({
						info,
						content: "",
						isText: false,
						truncated: resp.truncated,
					});
				}
			} catch (err) {
				setPreview({
					info,
					content: err instanceof Error ? err.message : String(err),
					isText: true,
				});
			} finally {
				setBusy(false);
			}
		},
		[runId],
	);

	const handleOpenExternally = useCallback(
		async (artifactID: string) => {
			try {
				const path = await api.getRunArtifactPath(runId, artifactID);
				await api.openPath(path);
			} catch (err) {
				// eslint-disable-next-line no-console
				console.error("Failed to open artifact:", err);
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
							{a.kind ?? "file"}
							{a.node_id ? ` · ${a.node_id}` : ""}
							{a.media_type ? ` · ${a.media_type}` : ""}
							{a.size_bytes !== undefined
								? ` · ${formatBytes(a.size_bytes)}`
								: a.size
									? ` · ${formatBytes(a.size)}`
									: ""}
							{a.created_at ? ` · ${formatDate(a.created_at)}` : ""}
						</span>
					</button>
				))}
			</div>
			{preview && (
				<div className="artifact-preview">
					<div className="artifact-preview-header">
						<span className="artifact-preview-title">
							{preview.info.name}
							{preview.truncated ? " (truncated)" : ""}
						</span>
						<div className="artifact-preview-actions">
							{!preview.isText && (
								<button
									className="artifact-preview-open"
									onClick={() => handleOpenExternally(preview.info.id)}
								>
									Open
								</button>
							)}
							<button
								className="artifact-preview-close"
								onClick={() => setPreview(null)}
							>
								Close
							</button>
						</div>
					</div>
					{preview.isText ? (
						<pre className="artifact-preview-content">{preview.content}</pre>
					) : (
						<div className="artifact-preview-binary">
							<p>
								Binary file — preview not available.
								{preview.info.media_type ? ` (${preview.info.media_type})` : ""}
							</p>
							<button
								className="artifact-preview-open"
								onClick={() => handleOpenExternally(preview.info.id)}
							>
								Open
							</button>
						</div>
					)}
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

function formatDate(iso: string): string {
	try {
		const d = new Date(iso);
		return d.toLocaleString();
	} catch {
		return iso;
	}
}
