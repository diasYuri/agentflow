import "@testing-library/jest-dom";
import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { RunTimeline } from "./RunTimeline.js";
import type { RunEvent } from "../api/types.js";

describe("RunTimeline", () => {
	it("renders empty state when no events", () => {
		render(<RunTimeline events={[]} />);
		expect(screen.getByText(/Waiting for events/)).toBeInTheDocument();
	});

	it("renders events", () => {
		const events: RunEvent[] = [
			{
				cursor: 0,
				timestamp: new Date().toISOString(),
				run_id: "r1",
				type: "node.started",
				node_id: "n1",
				attempt: 1,
				data: { msg: "hello" },
			},
		];
		render(<RunTimeline events={events} />);
		expect(screen.getByText(/node\.started/)).toBeInTheDocument();
		expect(screen.getByText(/n1/)).toBeInTheDocument();
		expect(screen.getByText(/attempt 1/)).toBeInTheDocument();
	});
});
