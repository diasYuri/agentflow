import { useState } from "react";
import type { AppSettings } from "../api/types.js";

interface SettingsViewProps {
	settings: AppSettings | null;
	isLoading: boolean;
	error: string | null;
	onSave: (next: Partial<AppSettings>) => Promise<void>;
}

export function SettingsView({
	settings,
	isLoading,
	error,
	onSave,
}: SettingsViewProps) {
	const [form, setForm] = useState<Partial<AppSettings>>(() => ({
		workspacePath: settings?.workspacePath ?? "",
		theme: settings?.theme ?? "light",
		codexPath: settings?.codexPath ?? "",
		claudePath: settings?.claudePath ?? "",
		piPath: settings?.piPath ?? "",
		logFormat: settings?.logFormat ?? "json",
	}));

	const update = (key: keyof AppSettings, value: string) => {
		setForm((f) => ({ ...f, [key]: value }));
	};

	return (
		<div className="settings-pane">
			<h2 className="settings-title">Settings</h2>
			{error && <div className="alert alert-error">{error}</div>}
			<div className="settings-section">
				<h3 className="settings-section-title">Workspace</h3>
				<label className="settings-label">
					Workspace path
					<input
						className="settings-input"
						value={form.workspacePath ?? ""}
						onChange={(e) => update("workspacePath", e.target.value)}
						placeholder="/path/to/workspace"
					/>
				</label>
				<label className="settings-label">
					Theme
					<select
						className="settings-select"
						value={form.theme ?? "light"}
						onChange={(e) => update("theme", e.target.value)}
					>
						<option value="light">Light</option>
						<option value="dark">Dark</option>
						<option value="system">System</option>
					</select>
				</label>
			</div>
			<div className="settings-section">
				<h3 className="settings-section-title">External Tools</h3>
				<label className="settings-label">
					Codex CLI path
					<input
						className="settings-input"
						value={form.codexPath ?? ""}
						onChange={(e) => update("codexPath", e.target.value)}
						placeholder="codex"
					/>
				</label>
				<label className="settings-label">
					Claude CLI path
					<input
						className="settings-input"
						value={form.claudePath ?? ""}
						onChange={(e) => update("claudePath", e.target.value)}
						placeholder="claude"
					/>
				</label>
				<label className="settings-label">
					Pi CLI path
					<input
						className="settings-input"
						value={form.piPath ?? ""}
						onChange={(e) => update("piPath", e.target.value)}
						placeholder="pi"
					/>
				</label>
			</div>
			<div className="settings-section">
				<h3 className="settings-section-title">Logging</h3>
				<label className="settings-label">
					Log format
					<select
						className="settings-select"
						value={form.logFormat ?? "json"}
						onChange={(e) => update("logFormat", e.target.value)}
					>
						<option value="json">JSON</option>
						<option value="text">Text</option>
					</select>
				</label>
			</div>
			<div className="settings-actions">
				<button
					className="action-btn"
					onClick={() => onSave(form)}
					disabled={isLoading}
				>
					{isLoading ? "Saving…" : "Save Settings"}
				</button>
			</div>
		</div>
	);
}
