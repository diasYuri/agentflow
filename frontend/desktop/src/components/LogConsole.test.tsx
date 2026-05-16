import "@testing-library/jest-dom";
import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { LogConsole } from "./LogConsole.js";

describe("LogConsole", () => {
	it("renders empty state when no logs", () => {
		render(<LogConsole lines={[]} />);
		expect(screen.getByText(/No logs yet/)).toBeInTheDocument();
	});

	it("renders log lines", () => {
		render(<LogConsole lines={["line one", "line two"]} />);
		expect(screen.getByText(/line one/)).toBeInTheDocument();
		expect(screen.getByText(/line two/)).toBeInTheDocument();
	});
});
