import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
	plugins: [tailwindcss(), react()],
	resolve: {
		alias: {
			"@": new URL("./src", (import.meta as ImportMeta & { url: string }).url)
				.pathname,
		},
	},
	build: {
		outDir: "../internal/web/assets",
		emptyOutDir: false,
		rollupOptions: {
			output: {
				entryFileNames: "static/shell.js",
				chunkFileNames: "static/[name]-[hash].js",
				assetFileNames: (info) => {
					if (info.names?.some((n) => n.endsWith(".css"))) {
						return "static/shell.css";
					}
					return "static/[name]-[hash][extname]";
				},
			},
		},
	},
	server: {
		host: "127.0.0.1",
		port: 5173,
		proxy: {
			"/api": {
				target: "http://127.0.0.1:38080",
				changeOrigin: true,
			},
			"/assets": {
				target: "http://127.0.0.1:38080",
				changeOrigin: true,
			},
		},
	},
});
