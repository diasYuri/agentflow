import type { WorkspaceState } from "../state/workspace.js";

interface EditorPaneProps {
	state: WorkspaceState;
	onChangeYaml: (value: string) => void;
	onChangeInput: (value: string) => void;
}

export function EditorPane({
	state,
	onChangeYaml,
	onChangeInput,
}: EditorPaneProps) {
	if (!state.workflow) {
		return (
			<div className="empty-state">
				<div className="empty-icon">📄</div>
				<div className="empty-title">No workflow open</div>
				<div className="empty-subtitle">
					Open a workflow from the sidebar to start editing.
				</div>
			</div>
		);
	}

	return (
		<div className="editor-pane">
			<div className="editor-col">
				<div className="editor-header">
					<span className="editor-title">Workflow YAML</span>
					{state.dirty && <span className="dirty-badge">unsaved</span>}
				</div>
				<textarea
					className="code-editor"
					value={state.workflowYaml}
					onChange={(e) => onChangeYaml(e.target.value)}
					spellCheck={false}
				/>
			</div>
			<div className="editor-col">
				<div className="editor-header">
					<span className="editor-title">Input JSON</span>
				</div>
				<textarea
					className="code-editor"
					value={state.inputJson}
					onChange={(e) => onChangeInput(e.target.value)}
					spellCheck={false}
				/>
				{state.workflow.inputs &&
					Object.keys(state.workflow.inputs).length > 0 && (
						<div className="input-hints">
							{Object.entries(state.workflow.inputs).map(([key, doc]) => (
								<div key={key} className="input-hint">
									<code>{key}</code>
									<span className="input-type">{doc?.type}</span>
									{doc?.required && (
										<span className="input-required">required</span>
									)}
								</div>
							))}
						</div>
					)}
			</div>
		</div>
	);
}
