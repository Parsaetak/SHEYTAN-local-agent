import { create } from "zustand";

import {
	activityWebSocketURL,
	api,
	type AppState,
	type ModelsResponse,
	type Preset,
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

	loadInitialState: () => Promise<void>;
	refreshModels: () => Promise<void>;
	refreshSessions: () => Promise<void>;
	refreshTools: () => Promise<void>;

	createSession: () => Promise<Session>;
	selectSession: (id: string | null) => void;
	deleteSession: (id: string) => Promise<void>;

	run: (message: string) => Promise<void>;
	abort: () => Promise<void>;

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

		const rawTimestamp = value.timestamp;

		let timestamp = Date.now();

		if (typeof rawTimestamp === "number") {
			timestamp = rawTimestamp;
		} else if (typeof rawTimestamp === "string") {
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

		loadInitialState: async () => {
			set({
				loading: true,
				error: null,
			});

			try {
				const [
					app,
					sysinfo,
					presets,
					models,
					sessions,
					tools,
				] = await Promise.all([
					api.state(),
					api.sysinfo(),
					api.presets(),
					api.models(),
					api.sessions(),
					api.tools(),
				]);

				const current =
					get().activeSessionId;

				const activeSessionId =
					current &&
					sessions.some(
						(session) =>
							session.id ===
							current,
					)
						? current
						: (sessions[0]?.id ??
							null);

				set({
					app,
					sysinfo,
					presets,
					models,
					sessions,
					tools,
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

		refreshSessions: async () => {
			try {
				const sessions =
					await api.sessions();

				const current =
					get().activeSessionId;

				const activeSessionId =
					current &&
					sessions.some(
						(session) =>
							session.id ===
							current,
					)
						? current
						: (sessions[0]?.id ??
							null);

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
							session.id !==
							id,
					);

				const activeSessionId =
					state.activeSessionId ===
					id
						? (sessions[0]?.id ??
							null)
						: state.activeSessionId;

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
					connection:
						"connected",
				});
			};

			socket.onmessage = (
				event,
			) => {
				try {
					const payload =
						JSON.parse(
							event.data,
						) as unknown;

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
						connection:
							"error",
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
			if (socket) {
				socket.close();
				socket = null;
			}

			activitySessionId = null;

			set({
				connection:
					"disconnected",
			});
		},

		clearActivity: () => {
			set({
				activity: [],
			});
		},
	}));
