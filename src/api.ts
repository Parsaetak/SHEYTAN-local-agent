export type AppState = {
	appName: string;
	appVersion: string;
	state: string;
};

export type SysInfo = Record<string, unknown>;

export type Preset = Record<string, unknown>;

export type ModelsResponse = {
	local: unknown[];
	loaded: unknown[];
	llamaRunning: boolean;
};

export type Session = {
	id: string;
	title?: string;
	model?: string;
	context?: Record<string, unknown>;
	messages?: unknown[];
};

export type ToolInfo = {
	name: string;
	description: string;
};

export type RunRequest = {
	sessionId: string;
	message: string;
};

const API_PREFIX = "/api";

async function request<T>(
	path: string,
	init?: RequestInit,
): Promise<T> {
	const response = await fetch(`${API_PREFIX}${path}`, {
		...init,
		headers: {
			"Content-Type": "application/json",
			...(init?.headers ?? {}),
		},
	});

	if (!response.ok) {
		let message = `Request failed with HTTP ${response.status}`;

		try {
			const body = (await response.json()) as {
				error?: string;
				message?: string;
			};

			message = body.error ?? body.message ?? message;
		} catch {
			// Keep the HTTP fallback message.
		}

		throw new Error(message);
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

	sessions(): Promise<Session[]> {
		return request<Session[]>("/sessions");
	},

	session(id: string): Promise<Session> {
		return request<Session>(`/sessions/${encodeURIComponent(id)}`);
	},

	createSession(): Promise<Session> {
		return request<Session>("/sessions", {
			method: "POST",
			body: JSON.stringify({}),
		});
	},

	deleteSession(id: string): Promise<{ ok: boolean }> {
		return request<{ ok: boolean }>(
			`/sessions/${encodeURIComponent(id)}`,
			{
				method: "DELETE",
			},
		);
	},

	tools(): Promise<ToolInfo[]> {
		return request<ToolInfo[]>("/tools");
	},

	run(payload: RunRequest): Promise<unknown> {
		return request<unknown>("/run", {
			method: "POST",
			body: JSON.stringify(payload),
		});
	},

	abort(): Promise<unknown> {
		return request<unknown>("/abort", {
			method: "POST",
			body: JSON.stringify({}),
		});
	},
};

export function activityWebSocketURL(): string {
	const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
	return `${protocol}//${window.location.host}/ws/activity`;
}
