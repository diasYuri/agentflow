import "@testing-library/jest-dom";
import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { SettingsView } from "./SettingsView.js";
import type { AppSettings } from "../api/types.js";

describe("SettingsView", () => {
	const baseSettings: AppSettings = {
		workspacePath: "/workspace",
		recentFiles: [],
		theme: "light",
		codexPath: "codex",
		claudePath: "claude",
		piPath: "pi",
		logFormat: "json",
	};

	it("renders settings form", () => {
		render(
			<SettingsView
				settings={baseSettings}
				isLoading={false}
				error={null}
				onSave={vi.fn()}
			/>,
		);
		expect(
			screen.getByRole("heading", { name: /Settings/ }),
		).toBeInTheDocument();
		expect(screen.getByDisplayValue("/workspace")).toBeInTheDocument();
	});

	it("calls onSave with updated values", () => {
		const onSave = vi.fn();
		render(
			<SettingsView
				settings={baseSettings}
				isLoading={false}
				error={null}
				onSave={onSave}
			/>,
		);
		const input = screen.getByDisplayValue("/workspace");
		fireEvent.change(input, { target: { value: "/new" } });
		fireEvent.click(screen.getByText(/Save Settings/));
		expect(onSave).toHaveBeenCalledWith(
			expect.objectContaining({ workspacePath: "/new" }),
		);
	});
});
