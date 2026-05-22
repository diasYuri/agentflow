import { describe, expect, test } from "vitest";
import {
	buildClearedSessionCookie,
	buildSessionCookie,
	resolveBootstrapToken,
} from "../lib/utils";

describe("auth token helpers", () => {
	test("prefers the query token when present", () => {
		expect(resolveBootstrapToken("?token=query-token", "stored-token")).toBe(
			"query-token",
		);
	});

	test("falls back to stored token when query token is absent", () => {
		expect(resolveBootstrapToken("", "stored-token")).toBe("stored-token");
	});

	test("builds a persistent session cookie", () => {
		expect(buildSessionCookie("abc123")).toContain("agentflow_session=abc123");
		expect(buildSessionCookie("abc123")).toContain("Path=/");
		expect(buildSessionCookie("abc123")).toContain("SameSite=Lax");
		expect(buildSessionCookie("abc123")).toContain("Max-Age=31536000");
	});

	test("builds a cookie clearing directive", () => {
		expect(buildClearedSessionCookie()).toBe(
			"agentflow_session=; Path=/; Max-Age=0; SameSite=Lax",
		);
	});
});
