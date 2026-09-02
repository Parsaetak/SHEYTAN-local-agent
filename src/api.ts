import { API_BASE, WS_BASE } from "./config";

export type ConnectionState = "connected" | "connecting" | "disconnected";

export interface AppState {
	app_name: string;
	version: string;
	status: string;
	uptime_seconds: number;
}

export interface SystemInfo {
	os: string;
	arch: string;
	cpu: string;
	cpu_count: number;
	memory_total: number;
	memory_available: number;
	hostname: string;
}

export interface Preset {
	id: string;
	name: string;
	description?: string;
	model?: string;
	temperature?: number;
	max_tokens?: number;
}

export interface Model {
	id: string;
	name: string;
	provider?: string;
	path?: string;
	size_bytes?: number;
	loaded?: boolean;
}

export interface Tool {
	name: string;
	description?: string;
	enabled?: boolean;
}

export interface Session {
	id: string;
	title: string;
	created_at: string;
	updated_at: string;
}

export interface ActivityEvent {
	id?: string;
	type: string;
	message?: string;
	timestamp?: string;
	session_id?: string;
	data?: unknown;
}

export interface RunRequest {
	session_id: string;
	prompt: string;
	model?: string;
}

export interface RunResponse {
	run_id?: string;
	session_id?: string;
	status?: string;
}

export interface AbortResponse {
	status?: string;
}

async function request<T>(
	path: string,
	init?: RequestInit,
): Promise<T> {
	const response = await fetch(`${API_BASE}${path}`, {
		...init,
		headers: {
			Accept: "application/json",
			...(init?.body ? { "Content-Type": "application/json" } : {}),
			...init?.headers,
		},
	});

	if (!response.ok) {
		const body = await response.text();
		throw new Error(
			body || `Request failed with HTTP ${response.status}`,
		);
	}

	if (response.status === 204) {
		return undefined as T;
	}

	return (await response.json()) as T;
}

export function getState(): Promise<AppState> {
	return request<AppState>("/state");
}

export function getSystemInfo(): Promise<SystemInfo> {
	return request<SystemInfo>("/sysinfo");
}

export function getPresets(): Promise<Preset[]> {
	return request<Preset[]>("/presets");
}

export function getModels(): Promise<Model[]> {
	return request<Model[]>("/models");
}

export function getTools(): Promise<Tool[]> {
	return request<Tool[]>("/tools");
}

export function getSessions(): Promise<Session[]> {
	return request<Session[]>("/sessions");
}

export function createSession(title = "New Session"): Promise<Session> {
	return request<Session>("/sessions", {
		method: "POST",
		body: JSON.stringify({ title }),
	});
}

export function deleteSession(id: string): Promise<void> {
	return request<void>(`/sessions/${encodeURIComponent(id)}`, {
		method: "DELETE",
	});
}

export function runAgent(payload: RunRequest): Promise<RunResponse> {
	return request<RunResponse>("/run", {
		method: "POST",
		body: JSON.stringify(payload),
	});
}

export function abortAgent(runId?: string): Promise<AbortResponse> {
	return request<AbortResponse>("/abort", {
		method: "POST",
		body: JSON.stringify(runId ? { run_id: runId } : {}),
	});
}

export function connectActivity(
	onEvent: (event: ActivityEvent) => void,
	onStateChange?: (state: ConnectionState) => void,
): () => void {
	let socket: WebSocket | undefined;
	let stopped = false;
	let reconnectTimer: ReturnType<typeof setTimeout> | undefined;

	const connect = () => {
		if (stopped) {
			return;
		}

		onStateChange?.("connecting");

		socket = new WebSocket(`${WS_BASE}/activity`);

		socket.addEventListener("open", () => {
			onStateChange?.("connected");
		});

		socket.addEventListener("message", (message) => {
			try {
				const event = JSON.parse(message.data) as ActivityEvent;
				onEvent(event);
			} catch {
				onEvent({
					type: "message",
					message: String(message.data),
				});
			}
		});

		socket.addEventListener("close", () => {
			socket = undefined;

			if (stopped) {
				onStateChange?.("disconnected");
				return;
			}

			onStateChange?.("disconnected");

			reconnectTimer = setTimeout(connect, 1500);
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
