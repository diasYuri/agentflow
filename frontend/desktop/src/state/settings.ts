import { useCallback, useEffect, useState } from "react";
import * as api from "../api/bindings.js";

export interface SettingsState {
	settings: api.AppSettings | null;
	isLoading: boolean;
	error: string | null;
}

export function useSettings() {
	const [state, setState] = useState<SettingsState>({
		settings: null,
		isLoading: false,
		error: null,
	});

	const loadSettings = useCallback(async () => {
		setState((s) => ({ ...s, isLoading: true, error: null }));
		try {
			const settings = await api.getAppSettings();
			setState((s) => ({ ...s, settings, isLoading: false }));
		} catch (err) {
			setState((s) => ({
				...s,
				isLoading: false,
				error: err instanceof Error ? err.message : String(err),
			}));
		}
	}, []);

	const saveSettings = useCallback(
		async (next: Partial<api.AppSettings>) => {
			setState((s) => ({ ...s, isLoading: true, error: null }));
			try {
				const merged = {
					...(state.settings ?? ({} as api.AppSettings)),
					...next,
				};
				await api.updateAppSettings(merged);
				setState((s) => ({ ...s, settings: merged, isLoading: false }));
			} catch (err) {
				setState((s) => ({
					...s,
					isLoading: false,
					error: err instanceof Error ? err.message : String(err),
				}));
			}
		},
		[state.settings],
	);

	useEffect(() => {
		loadSettings();
	}, [loadSettings]);

	return {
		state,
		loadSettings,
		saveSettings,
	};
}
