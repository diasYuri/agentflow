import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./styles/index.css";
import { Providers } from "./app/providers";
import { Router } from "./app/router";

// Shell ready marker for ingration tests
console.info("agentflow web shell ready");

const token =
	new URLSearchParams(window.location.search).get("token") ??
	localStorage.getItem("agentflow_token");
if (token) {
	localStorage.setItem("agentflow_token", token);
	window.history.replaceState({}, "", window.location.pathname);
}

const root = document.getElementById("root");
if (!root) {
	throw new Error("Missing #root element");
}

createRoot(root).render(
	<StrictMode>
		<Providers>
			<Router />
		</Providers>
	</StrictMode>,
);
