import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./styles/index.css";
import { Providers } from "./app/providers";
import { Router } from "./app/router";
import { persistToken, resolveBootstrapToken } from "./lib/utils";

// Shell ready marker for ingration tests
console.info("agentflow web shell ready");

const token = resolveBootstrapToken(
	window.location.search,
	localStorage.getItem("agentflow_token"),
);
if (token) {
	persistToken(token);
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
