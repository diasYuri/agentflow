import "@testing-library/jest-dom";
import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ArtifactsPane } from "./ArtifactsPane.js";
import * as api from "../api/bindings.js";

vi.mock("../api/bindings.js", () => ({
	getRunArtifact: vi.fn(),
	getRunArtifactPath: vi.fn(),
	openPath: vi.fn(),
}));

const mockArtifactText = {
	id: "nodes/a/stdout.txt",
	name: "stdout.txt",
	path: "nodes/a/stdout.txt",
	size: 5,
	size_bytes: 5,
	media_type: "text/plain",
	kind: "stdout",
	node_id: "a",
	created_at: "2024-01-01T00:00:00Z",
};

const mockArtifactBinary = {
	id: "image.png",
	name: "image.png",
	path: "image.png",
	size: 1024,
	size_bytes: 1024,
	media_type: "image/png",
	kind: "file",
	created_at: "2024-01-01T00:00:00Z",
};

describe("ArtifactsPane", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("renders empty state when no artifacts", () => {
		render(<ArtifactsPane artifacts={[]} runId="run-1" />);
		expect(screen.getByText("No artifacts")).toBeInTheDocument();
	});

	it("renders artifact list with metadata", () => {
		render(<ArtifactsPane artifacts={[mockArtifactText]} runId="run-1" />);
		expect(screen.getByText("stdout.txt")).toBeInTheDocument();
		const meta = screen.getByText(/text\/plain/);
		expect(meta).toBeInTheDocument();
		expect(meta.textContent).toContain("stdout");
		expect(meta.textContent).toContain("a");
	});

	it("shows text preview when artifact is textual", async () => {
		vi.mocked(api.getRunArtifact).mockResolvedValue({
			id: "nodes/a/stdout.txt",
			name: "stdout.txt",
			path: "nodes/a/stdout.txt",
			size: 5,
			is_text: true,
			text_content: "hello",
			encoding: "text",
		});

		render(<ArtifactsPane artifacts={[mockArtifactText]} runId="run-1" />);
		fireEvent.click(screen.getByText("stdout.txt"));

		await waitFor(() => expect(screen.getByText("hello")).toBeInTheDocument());
	});

	it("shows binary state without text preview", async () => {
		vi.mocked(api.getRunArtifact).mockResolvedValue({
			id: "image.png",
			name: "image.png",
			path: "image.png",
			size: 1024,
			is_text: false,
			media_type: "image/png",
			encoding: "base64",
			truncated: true,
		});

		render(<ArtifactsPane artifacts={[mockArtifactBinary]} runId="run-1" />);
		fireEvent.click(screen.getByText("image.png"));

		await waitFor(() =>
			expect(
				screen.getByText(/Binary file — preview not available/),
			).toBeInTheDocument(),
		);
	});

	it("opens binary artifact externally", async () => {
		vi.mocked(api.getRunArtifact).mockResolvedValue({
			id: "image.png",
			name: "image.png",
			path: "image.png",
			size: 1024,
			is_text: false,
			media_type: "image/png",
			encoding: "base64",
			truncated: true,
		});
		vi.mocked(api.getRunArtifactPath).mockResolvedValue(
			"/tmp/run-1/artifacts/image.png",
		);
		vi.mocked(api.openPath).mockResolvedValue(undefined);

		render(<ArtifactsPane artifacts={[mockArtifactBinary]} runId="run-1" />);
		fireEvent.click(screen.getByText("image.png"));

		await waitFor(() =>
			expect(screen.getAllByText("Open").length).toBeGreaterThan(0),
		);

		fireEvent.click(screen.getAllByText("Open")[0]);

		await waitFor(() =>
			expect(api.getRunArtifactPath).toHaveBeenCalledWith("run-1", "image.png"),
		);
		await waitFor(() =>
			expect(api.openPath).toHaveBeenCalledWith(
				"/tmp/run-1/artifacts/image.png",
			),
		);
	});

	it("displays error message when artifact fetch fails", async () => {
		vi.mocked(api.getRunArtifact).mockRejectedValue(
			new Error("artifact not found"),
		);

		render(<ArtifactsPane artifacts={[mockArtifactText]} runId="run-1" />);
		fireEvent.click(screen.getByText("stdout.txt"));

		await waitFor(() =>
			expect(screen.getByText("artifact not found")).toBeInTheDocument(),
		);
	});
});
