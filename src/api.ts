import {
	API_BASE,
	WS_BASE,
	activityWebSocketURL,
} from "./config";

export interface AppState {
	appName: string;
	appVersion: string;
	state: string;
}

export interface SysInfoCPU {
	name: string;
	physicalCores: number;
	logicalCores: number;
	frequencyMHz: number;
}

export interface SysInfoRAM {
	totalBytes: number;
	freeBytes: number;
	availableBytes: number;
}

export interface SysInfoDisk {
	totalBytes: number;
	freeBytes: number;
	path: string;
}

export interface SysInfoGPU {
	vendor: string;
	name: string;
	vramBytes: number;
	driverVersion: string;
}

export interface SysInfoRecommended {
	numThread: number;
	numGPU: number;
	numCtx: number;
	numBatch: number;
	maxTokens: number;
	canRunCPU: boolean;
	canRunGPU: boolean;
	warnings: string[];
}

export interface SysInfo {
	os: string;
	arch: string;
	hostname: string;
	cpu: SysInfoCPU;
	ram: SysInfoRAM;
	disk: SysInfoDisk;
	gpus: SysInfoGPU[];
	wsl2: boolean;
	docker: boolean;
	recommended: SysInfoRecommended;
}

export interface Preset {
	id: string;
	name: string;
	description?: string;
	model?: string;
	temperature?: number;
	maxTokens?: number;
}

export interface Model {
	id: string;
	name: string;
	provider?: string;
	path?: string;
	sizeBytes?: number;
	loaded?: boolean;
}

export interface ModelsResponse {
	local: Model[];
	loaded: Model[];
	llamaRunning: boolean;
}

export interface ToolInfo {
	name: string;
	description?: string;
	enabled?: boolean;
}

export interface Session {
	id: string;
	title: string;
	model?: string;
	preset?: string;
	createdAt: string;
	updatedAt: string;
	threadId?: string;
	parentId?: string;
	chapter?: number;
	msgCount?: number;
}

export interface RunRequest {
	sessionId: string;
	message: string;
}

export interface RunResponse {
	ok: boolean;
	sessionId: string;
	runId?: string;
	status?: string;
}

export interface AbortResponse {
	ok: boolean;
	status?: string;
}

export type ActivityEvent = {
	id?: string;
	type: string;
	caption?: string;
	timestamp?: string | number;
	sessionId?: string;
	data?: unknown;
	[key: string]: unknown;
};

export type ConnectionState =
	| "connected"
	| "connecting"
	| "disconnected";

async function request<T>(
	path: string,
	init?: RequestInit,
): Promise<T> {
	const response = await fetch(`${API_BASE}${path}`, {
		...init,
		headers: {
			Accept: "application/json",
			...(init?.body
				? {
						"Content-Type":
							"application/json",
					}
				: {}),
			...init?.headers,
		},
	});

	if (!response.ok) {
		const body = await response.text();

		let message = body;

		try {
			const parsed = JSON.parse(body) as {
				error?: string;
			};

			if (
				typeof parsed.error === "string" &&
				parsed.error.trim()
			) {
				message = parsed.error;
			}
		} catch {
			// Preserve the raw response body.
		}

		throw new Error(
			message ||
				`Request failed with HTTP ${response.status}`,
		);
	}

	if (response.status === 204) {
		return undefined as T;
	}

	return (await response.json()) as T;
}

export const api = {
	state(): Promise<AppState> {
		return request<AppState>("/state");
	},

	sysinfo(): Promise<SysInfo> {
		return request<SysInfo>("/sysinfo");
	},

	presets(): Promise<Preset[]> {
		return request<Preset[]>("/presets");
	},

	models(): Promise<ModelsResponse> {
		return request<ModelsResponse>("/models");
	},

	tools(): Promise<ToolInfo[]> {
		return request<ToolInfo[]>("/tools");
	},

	sessions(): Promise<Session[]> {
		return request<Session[]>("/sessions");
	},

	session(id: string): Promise<Session> {
		return request<Session>(
			`/sessions/${encodeURIComponent(id)}`,
		);
	},

	createSession(): Promise<Session> {
		return request<Session>("/sessions", {
			method: "POST",
		});
	},

	deleteSession(id: string): Promise<void> {
		return request<void>(
			`/sessions/${encodeURIComponent(id)}`,
			{
				method: "DELETE",
			},
		);
	},

	run(payload: RunRequest): Promise<RunResponse> {
		return request<RunResponse>("/run", {
			method: "POST",
			body: JSON.stringify(payload),
		});
	},

	abort(sessionId: string): Promise<AbortResponse> {
		return request<AbortResponse>("/abort", {
			method: "POST",
			body: JSON.stringify({
				sessionId,
			}),
		);
	},

	updateSession(
		id: string,
		payload: {
			title?: string;
			model?: string;
			context?: {
				systemPrompt?: string;
				attachedFiles?: string[];
				maxIterations?: number;
			};
		},
	): Promise<Session> {
		return request<Session>(
			`/sessions/${encodeURIComponent(id)}`,
			{
				method: "PUT",
				body: JSON.stringify(payload),
			},
		);
	},
};

export function getState(): Promise<AppState> {
	return api.state();
}

export function getSystemInfo(): Promise<SysInfo> {
	return api.sysinfo();
}

export function getPresets(): Promise<Preset[]> {
	return api.presets();
}

export function getModels(): Promise<ModelsResponse> {
	return api.models();
}

export function getTools(): Promise<ToolInfo[]> {
	return api.tools();
}

export function getSessions(): Promise<Session[]> {
	return api.sessions();
}

export function createSession(): Promise<Session> {
	return api.createSession();
}

export function deleteSession(
	id: string,
): Promise<void> {
	return api.deleteSession(id);
}

export function runAgent(
	payload: RunRequest,
): Promise<RunResponse> {
	return api.run(payload);
}

export function abortAgent(
	sessionId: string,
): Promise<AbortResponse> {
	return api.abort(sessionId);
}

export function connectActivity(
	onEvent: (event: ActivityEvent) => void,
	onStateChange?: (
		state: ConnectionState,
	) => void,
	sessionId?: string | null,
): () => void {
	let socket: WebSocket | undefined;
	let stopped = false;
	let reconnectTimer:
		| ReturnType<typeof setTimeout>
		| undefined;

	const connect = () => {
		if (stopped) {
			return;
		}

		onStateChange?.("connecting");

		socket = new WebSocket(
			activityWebSocketURL(sessionId),
		);

		socket.addEventListener("open", () => {
			onStateChange?.("connected");
		});

		socket.addEventListener(
			"message",
			(message) => {
				try {
					const event =
						JSON.parse(
							message.data,
						) as ActivityEvent;

					onEvent(event);
				} catch {
					onEvent({
						type: "message",
						message: String(
							message.data,
						),
					});
				}
			},
		);

		socket.addEventListener("close", () => {
			socket = undefined;

			if (stopped) {
				onStateChange?.(
					"disconnected",
				);
				return;
			}

			onStateChange?.(
				"disconnected",
			);

			reconnectTimer = setTimeout(
				connect,
				1500,
			);
		});

		socket.addEventListener("error", () => {
			onStateChange?.("disconnected");
		});
	};

	connect();

	return () => {
		stopped = true;

		if (reconnectTimer !== undefined) {
			clearTimeout(reconnectTimer);
		}

		socket?.close();
		socket = undefined;
	};
}

export { API_BASE, WS_BASE };
