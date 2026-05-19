import { useStore } from "@/lib/store";
import { yaml } from "@codemirror/lang-yaml";
import { githubDark, githubLight } from "@uiw/codemirror-theme-github";
import CodeMirror from "@uiw/react-codemirror";
import { useCallback } from "react";

interface Props {
	value: string;
	onChange: (v: string) => void;
}

export function YamlEditor({ value, onChange }: Props) {
	const { theme } = useStore();
	const isDark =
		theme === "dark" ||
		(theme === "system" &&
			window.matchMedia("(prefers-color-scheme: dark)").matches);

	const handleChange = useCallback(
		(v: string) => {
			onChange(v);
		},
		[onChange],
	);

	return (
		<div className="h-full">
			<CodeMirror
				value={value}
				height="100%"
				theme={isDark ? githubDark : githubLight}
				extensions={[yaml()]}
				onChange={handleChange}
				basicSetup={{
					lineNumbers: true,
					highlightActiveLineGutter: true,
					highlightActiveLine: true,
					foldGutter: true,
				}}
			/>
		</div>
	);
}
