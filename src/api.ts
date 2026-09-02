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

export interface LabWorkspace {
	id: string;
	source: string;
	path: string;
	createdAt: string;
}

export interface LabCommand {
	command: string;
	workingDir?: string;
	timeout?: number;
	maxOutputBytes?: number;
}

export interface LabCommandResult {
	command: string;
	workingDir: string;
	stdout: string;
	stderr: string;
	output: string;
	exitCode: number;
	duration: number;
	startedAt: string;
	finishedAt: string;
	timedOut: boolean;
	canceled: boolean;
	outputLimit: boolean;
	success: boolean;
}

export type LabVerificationStatus =
	| "passed"
	| "failed"
	| "canceled"
	| "skipped";

export interface LabVerificationResult {
	name: string;
	status: LabVerificationStatus;
	result: LabCommandResult;
	error?: string;
	startedAt: string;
	finishedAt: string;
}

export interface LabVerificationSummary {
	passed: boolean;
	requiredTotal: number;
	requiredPassed: number;
	requiredFailed: number;
	optionalTotal: number;
	optionalPassed: number;
	optionalFailed: number;
	results: LabVerificationResult[];
	duration: number;
	startedAt: string;
	finishedAt: string;
	error?: string;
}

export type LabTaskStatus =
	| "pending"
	| "running"
	| "succeeded"
	| "failed"
	| "canceled"
	| "blocked";

export interface LabTaskSnapshot {
	id: string;
	title: string;
	description: string;
	status: LabTaskStatus;
	workspace?: LabWorkspace;
	commands?: LabCommand[];
	results?: LabCommandResult[];
	metadata?: Record<string, string>;
	lastVerification?: LabVerificationSummary;
	verificationPassed: boolean;
	verifiedAt?: string;
	createdAt: string;
	startedAt?: string;
	finishedAt?: string;
	error?: string;
}

export interface LabTaskSessionSnapshot {
	id: string;
	createdAt: string;
	updatedAt: string;
	task: LabTaskSnapshot;
}

export interface LabListResponse {
	tasks: LabTaskSessionSnapshot[];
}

export interface LabActionResponse {
	ok: boolean;
	result?: unknown;
	error?: string;
}

export interface ResearchResult {
	title: string;
	url: string;
	snippet?: string;
	source: string;
	provider: string;
	publishedAt?: string;
	authority: string;
	matchScore: number;
	contentHash?: string;
	metadata?: Record<string, unknown>;
}

export interface ResearchResponse {
	ok: boolean;
	provider: string;
	query: string;
	duration: number;
	results: ResearchResult[];
	error?: string;
	providers?: string[];
	backend?: string;
}

export interface ResearchConfig {
	backend: string;
	providers: string[];
}

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

	lab(): Promise<LabListResponse> {
		return request<LabListResponse>("/lab");
	},

	labTask(id: string): Promise<LabTaskSessionSnapshot> {
		return request<LabTaskSessionSnapshot>(
			`/lab/${encodeURIComponent(id)}`,
		);
	},

	labAction(
		payload: Record<string, unknown>,
	): Promise<LabActionResponse> {
		return request<LabActionResponse>("/lab", {
			method: "POST",
			body: JSON.stringify(payload),
		});
	},

	researchConfig(): Promise<ResearchConfig> {
		return request<ResearchConfig>("/research");
	},

	research(
		payload: {
			query: string;
			backend?: string;
			maxResults?: number;
			timeoutSec?: number;
		},
	): Promise<ResearchResponse> {
		return request<ResearchResponse>("/research", {
			method: "POST",
			body: JSON.stringify(payload),
		});
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
