import { create } from "zustand";

import {
	activityWebSocketURL,
	api,
	type ActivityEvent as APIActivityEvent,
	type AppState,
	type LabListResponse,
	type LabTaskSessionSnapshot,
	type ModelsResponse,
	type Preset,
	type ResearchConfig,
	type ResearchResponse,
	type Session,
	type SysInfo,
	type ToolInfo,
} from "./api";

export type ConnectionState =
	| "idle"
	| "connecting"
	| "connected"
	| "disconnected"
	| "error";

export type ActivityEvent = {
	id: string;
	type: string;
	timestamp: number;
	data: Record<string, unknown>;
};

type RuntimeState = {
	app: AppState | null;
	sysinfo: SysInfo | null;
	models: ModelsResponse | null;
	presets: Preset[];
	tools: ToolInfo[];

	sessions: Session[];
	activeSessionId: string | null;

	connection: ConnectionState;
	loading: boolean;
	error: string | null;

	activity: ActivityEvent[];
	running: boolean;

	lab: LabListResponse | null;
	labLoading: boolean;
	labError: string | null;
	activeLabTaskId: string | null;
	activeLabTask: LabTaskSessionSnapshot | null;

	researchConfig: ResearchConfig | null;
	research: ResearchResponse | null;
	researchLoading: boolean;
	researchError: string | null;

	loadInitialState: () => Promise<void>;
	refreshSysinfo: () => Promise<void>;
	refreshModels: () => Promise<void>;
	refreshPresets: () => Promise<void>;
	refreshSessions: () => Promise<void>;
	refreshTools: () => Promise<void>;
	refreshAgentResources: () => Promise<void>;

	refreshLab: () => Promise<void>;
	loadLabTask: (id: string) => Promise<void>;

	createSession: () => Promise<Session>;
	selectSession: (id: string | null) => void;
	deleteSession: (id: string) => Promise<void>;

	run: (message: string) => Promise<void>;
	abort: () => Promise<void>;

	runLabAction: (
		payload: Record<string, unknown>,
	) => Promise<unknown>;

	loadResearchConfig: () => Promise<void>;
	searchResearch: (payload: {
		query: string;
		backend?: string;
		maxResults?: number;
		timeoutSec?: number;
	}) => Promise<ResearchResponse>;

	connectActivity: () => void;
	disconnectActivity: () => void;
	clearActivity: () => void;
};

let socket: WebSocket | null = null;
let activitySequence = 0;
let activitySessionId: string | null = null;

function createActivityID(): string {
	activitySequence += 1;

	return `${Date.now()}-${activitySequence}`;
}

function normalizeActivity(
	payload: unknown,
): ActivityEvent {
	if (payload && typeof payload === "object") {
		const value =
			payload as Record<string, unknown>;

		const rawTimestamp =
			value.timestamp;

		let timestamp = Date.now();

		if (typeof rawTimestamp === "number") {
			timestamp = rawTimestamp;
		} else if (
			typeof rawTimestamp === "string"
		) {
			const parsed = Date.parse(
				rawTimestamp,
			);

			if (!Number.isNaN(parsed)) {
				timestamp = parsed;
			}
		}

		return {
			id: createActivityID(),
			type:
				typeof value.type === "string"
					? value.type
					: "activity",
			timestamp,
			data: value,
		};
	}

	return {
		id: createActivityID(),
		type: "activity",
		timestamp: Date.now(),
		data: {
			value: payload,
		},
	};
}

function resolveActiveSessionID(
	sessions: Session[],
	currentID: string | null,
): string | null {
	if (
		currentID &&
		sessions.some(
			(session) =>
				session.id === currentID,
		)
	) {
		return currentID;
	}

	return sessions[0]?.id ?? null;
}

export const useRuntimeStore =
	create<RuntimeState>((set, get) => ({
		app: null,
		sysinfo: null,
		models: null,
		presets: [],
		tools: [],

		sessions: [],
		activeSessionId: null,

		connection: "idle",
		loading: false,
		error: null,

		activity: [],
		running: false,

		lab: null,
		labLoading: false,
		labError: null,
		activeLabTaskId: null,
		activeLabTask: null,

		researchConfig: null,
		research: null,
		researchLoading: false,
		researchError: null,

		loadInitialState: async () => {
			set({
				loading: true,
				error: null,
			});

			try {
				const [
					app,
					models,
					sessions,
				] = await Promise.all([
					api.state(),
					api.models(),
					api.sessions(),
				]);

				const current =
					get().activeSessionId;

				const activeSessionId =
					resolveActiveSessionID(
						sessions,
						current,
					);

				set({
					app,
					models,
					sessions,
					activeSessionId,
					loading: false,
				});
			} catch (error) {
				set({
					loading: false,
					error:
						error instanceof Error
							? error.message
							: "Failed to initialize runtime state.",
				});
			}
		},

		refreshSysinfo: async () => {
			try {
				const sysinfo =
					await api.sysinfo();

				set({ sysinfo });
			} catch (error) {
				set({
					error:
						error instanceof Error
							? error.message
							: "Failed to refresh system information.",
				});
			}
		},

		refreshModels: async () => {
			try {
				const models =
					await api.models();

				set({ models });
			} catch (error) {
				set({
					error:
						error instanceof Error
							? error.message
							: "Failed to refresh models.",
				});
			}
		},

		refreshPresets: async () => {
			try {
				const presets =
					await api.presets();

				set({ presets });
			} catch (error) {
				set({
					error:
						error instanceof Error
							? error.message
							: "Failed to refresh presets.",
				});
			}
		},

		refreshSessions: async () => {
			try {
				const sessions =
					await api.sessions();

				const current =
					get().activeSessionId;

				const activeSessionId =
					resolveActiveSessionID(
						sessions,
						current,
					);

				set({
					sessions,
					activeSessionId,
				});
			} catch (error) {
				set({
					error:
						error instanceof Error
							? error.message
							: "Failed to refresh sessions.",
				});
			}
		},

		refreshTools: async () => {
			try {
				const tools =
					await api.tools();

				set({ tools });
			} catch (error) {
				set({
					error:
						error instanceof Error
							? error.message
							: "Failed to refresh tools.",
				});
			}
		},

		refreshAgentResources: async () => {
			set({
				error: null,
			});

			try {
				const [
					sysinfo,
					presets,
					tools,
				] = await Promise.all([
					api.sysinfo(),
					api.presets(),
					api.tools(),
				]);

				set({
					sysinfo,
					presets,
					tools,
				});
			} catch (error) {
				set({
					error:
						error instanceof Error
							? error.message
							: "Failed to load Agent resources.",
				});
			}
		},

		refreshLab: async () => {
			set({
				labLoading: true,
				labError: null,
			});

			try {
				const lab =
					await api.lab();

				const activeLabTaskId =
					get().activeLabTaskId;

				const activeLabTask =
					activeLabTaskId
						? lab.tasks.find(
								(item) =>
									item.id ===
									activeLabTaskId,
							) ?? null
						: null;

				set({
					lab,
					labLoading: false,
					activeLabTask,
				});
			} catch (error) {
				set({
					labLoading: false,
					labError:
						error instanceof Error
							? error.message
							: "Failed to refresh Coding Lab.",
				});
			}
		},

		loadLabTask: async (id) => {
			const taskId = id.trim();

			if (!taskId) {
				set({
					activeLabTaskId: null,
					activeLabTask: null,
				});

				return;
			}

			set({
				activeLabTaskId: taskId,
				labLoading: true,
				labError: null,
			});

			try {
				const activeLabTask =
					await api.labTask(taskId);

				set({
					activeLabTask,
					labLoading: false,
				});
			} catch (error) {
				set({
					labLoading: false,
					labError:
						error instanceof Error
							? error.message
							: "Failed to load Coding Lab task.",
				});
			}
		},

		createSession: async () => {
			const session =
				await api.createSession();

			set((state) => ({
				sessions: [
					session,
					...state.sessions,
				],
				activeSessionId:
					session.id,
			}));

			return session;
		},

		selectSession: (id) => {
			if (
				get().activeSessionId ===
				id
			) {
				return;
			}

			get().disconnectActivity();

			set({
				activeSessionId: id,
				error: null,
				activity: [],
			});

			if (id) {
				get().connectActivity();
			}
		},

		deleteSession: async (id) => {
			await api.deleteSession(id);

			if (
				get().activeSessionId ===
				id
			) {
				get().disconnectActivity();
			}

			set((state) => {
				const sessions =
					state.sessions.filter(
						(session) =>
							session.id !== id,
					);

				const activeSessionId =
					resolveActiveSessionID(
						sessions,
						state.activeSessionId ===
							id
							? null
							: state.activeSessionId,
					);

				return {
					sessions,
					activeSessionId,
				};
			});

			if (get().activeSessionId) {
				get().connectActivity();
			}
		},

		run: async (message) => {
			const sessionId =
				get().activeSessionId;

			if (!sessionId) {
				throw new Error(
					"No active session.",
				);
			}

			if (!message.trim()) {
				return;
			}

			set({
				running: true,
				error: null,
			});

			try {
				await api.run({
					sessionId,
					message: message.trim(),
				});

				await get().refreshSessions();
			} catch (error) {
				set({
					error:
						error instanceof Error
							? error.message
							: "Agent run failed.",
					running: false,
				});

				throw error;
			}
		},

		abort: async () => {
			const sessionId =
				get().activeSessionId;

			if (!sessionId) {
				set({
					running: false,
				});

				return;
			}

			try {
				await api.abort(sessionId);
			} finally {
				set({
					running: false,
				});
			}
		},

		runLabAction: async (payload) => {
			set({
				labLoading: true,
				labError: null,
			});

			try {
				const response =
					await api.labAction(
						payload,
					);

				if (!response.ok) {
					throw new Error(
						response.error ||
							"Coding Lab action failed.",
					);
				}

				await get().refreshLab();

				const activeTaskId =
					get().activeLabTaskId;

				if (activeTaskId) {
					await get().loadLabTask(
						activeTaskId,
					);
				}

				return response.result;
			} catch (error) {
				const message =
					error instanceof Error
						? error.message
						: "Coding Lab action failed.";

				set({
					labLoading: false,
					labError: message,
				});

				throw error;
			}
		},

		loadResearchConfig: async () => {
			set({
				researchError: null,
			});

			try {
				const researchConfig =
					await api.researchConfig();

				set({
					researchConfig,
				});
			} catch (error) {
				set({
					researchError:
						error instanceof Error
							? error.message
							: "Failed to load research configuration.",
				});
			}
		},

		searchResearch: async (payload) => {
			const query =
				payload.query.trim();

			if (!query) {
				throw new Error(
					"Research query is required.",
				);
			}

			set({
				researchLoading: true,
				researchError: null,
			});

			try {
				const research =
					await api.research({
						...payload,
						query,
					});

				set({
					research,
					researchLoading: false,
				});

				return research;
			} catch (error) {
				const message =
					error instanceof Error
						? error.message
						: "Research request failed.";

				set({
					researchLoading: false,
					researchError: message,
				});

				throw error;
			}
		},

		connectActivity: () => {
			const sessionId =
				get().activeSessionId;

			if (!sessionId) {
				return;
			}

			if (
				socket &&
				activitySessionId ===
					sessionId &&
				(socket.readyState ===
					WebSocket.OPEN ||
					socket.readyState ===
						WebSocket.CONNECTING)
			) {
				return;
			}

			if (socket) {
				socket.close();
				socket = null;
			}

			activitySessionId =
				sessionId;

			set({
				connection: "connecting",
			});

			socket = new WebSocket(
				activityWebSocketURL(
					sessionId,
				),
			);

			socket.onopen = () => {
				if (
					activitySessionId !==
					get().activeSessionId
				) {
					return;
				}

				set({
					connection: "connected",
				});
			};

			socket.onmessage = (
				event,
			) => {
				try {
					const payload =
						JSON.parse(
							event.data,
						) as APIActivityEvent;

					const activity =
						normalizeActivity(
							payload,
						);

					set((state) => ({
						activity:
							state.activity
								.length >=
							500
								? [
										...state.activity.slice(
											-499,
										),
										activity,
									]
								: [
										...state.activity,
										activity,
									],
						running:
							typeof activity
								.data
								.running ===
							"boolean"
								? Boolean(
										activity
											.data
											.running,
									)
								: state.running,
					}));
				} catch {
					// Ignore malformed activity frames.
				}
			};

			socket.onerror = () => {
				if (
					activitySessionId ===
					get().activeSessionId
				) {
					set({
						connection: "error",
					});
				}
			};

			socket.onclose = () => {
				socket = null;

				if (
					activitySessionId !==
					get().activeSessionId
				) {
					return;
				}

				set({
					connection:
						"disconnected",
				});
			};
		},

		disconnectActivity: () => {
			activitySessionId = null;

			if (socket) {
				socket.close();
				socket = null;
			}

			set({
				connection: "idle",
			});
		},

		clearActivity: () => {
			set({
				activity: [],
			});
		},
	}));

export function getRuntimeState(): RuntimeState {
	return useRuntimeStore.getState();
}
