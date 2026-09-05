import { create } from "zustand";

import {
  api,
  type ActivityEvent as APIActivityEvent,
  type AppState,
  type Attachment,
  type ChatMessage,
  type EngineSnapshot,
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
import { activityWebSocketURL } from "./config";

export type ConnectionState =
  "idle" | "connecting" | "connected" | "disconnected" | "error";

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

  // v1.1.3Z: authoritative engine state (polled + WS-pushed).
  engine: EngineSnapshot | null;

  // v1.1.3Z: real conversation history for the active session plus the
  // streaming assistant bubble.
  messages: ChatMessage[];
  streaming: { content: string; reasoning: string } | null;

  // v1.1.3Z: staged attachments for the composer.
  pendingAttachments: Attachment[];
  attachmentsUploading: boolean;

  lab: LabListResponse | null;
  labLoading: boolean;
  labError: string | null;
  activeLabTaskId: string | null;
  activeLabTask: LabTaskSessionSnapshot | null;

  researchConfig: ResearchConfig | null;
  research: ResearchResponse | null;
  researchLoading: boolean;
  researchError: string | null;

  refreshSysinfo: () => Promise<void>;
  refreshModels: () => Promise<void>;
  refreshPresets: () => Promise<void>;
  refreshSessions: () => Promise<void>;
  refreshTools: () => Promise<void>;
  refreshAgentResources: () => Promise<void>;
  refreshEngine: () => Promise<void>;
  startEnginePolling: () => void;

  loadSession: (id: string) => Promise<void>;

  refreshLab: () => Promise<void>;
  loadLabTask: (id: string) => Promise<void>;

  createSession: () => Promise<Session>;
  selectSession: (id: string | null) => void;
  deleteSession: (id: string) => Promise<void>;

  run: (message: string) => Promise<void>;
  abort: () => Promise<void>;
  regenerate: () => Promise<void>;

  uploadFiles: (files: File[]) => Promise<void>;
  removePendingAttachment: (id: string) => Promise<void>;

  runLabAction: (payload: Record<string, unknown>) => Promise<unknown>;

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

const MAX_ACTIVITY_EVENTS = 500;

let enginePollTimer: number | null = null;

let socket: WebSocket | null = null;
let activitySequence = 0;
let activitySessionId: string | null = null;

let activityFlushFrame: number | null = null;
let pendingActivity: ActivityEvent[] = [];
let flushingActivity: ActivityEvent[] = [];
let pendingRunning: boolean | undefined;

function createActivityID(): string {
  activitySequence += 1;

  return `${Date.now()}-${activitySequence}`;
}

function normalizeActivity(payload: unknown): ActivityEvent {
  if (payload && typeof payload === "object") {
    const value = payload as Record<string, unknown>;

    const rawTimestamp = value.timestamp;

    let timestamp = Date.now();

    if (typeof rawTimestamp === "number") {
      timestamp = rawTimestamp;
    } else if (typeof rawTimestamp === "string") {
      const parsed = Date.parse(rawTimestamp);

      if (!Number.isNaN(parsed)) {
        timestamp = parsed;
      }
    }

    return {
      id: createActivityID(),
      type: typeof value.type === "string" ? value.type : "activity",
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
  if (currentID && sessions.some((session) => session.id === currentID)) {
    return currentID;
  }

  return sessions[0]?.id ?? null;
}

function resetPendingActivity(): void {
  if (activityFlushFrame !== null) {
    cancelAnimationFrame(activityFlushFrame);

    activityFlushFrame = null;
  }

  pendingActivity.length = 0;
  flushingActivity.length = 0;
  pendingRunning = undefined;
}

function flushActivity(): void {
  activityFlushFrame = null;

  if (pendingActivity.length === 0 && pendingRunning === undefined) {
    return;
  }

  const batch = pendingActivity;

  pendingActivity = flushingActivity;
  flushingActivity = batch;

  const running = pendingRunning;
  pendingRunning = undefined;

  if (
    !activitySessionId ||
    activitySessionId !== useRuntimeStore.getState().activeSessionId
  ) {
    flushingActivity.length = 0;

    return;
  }

  setActivityBatch(batch, running);

  batch.length = 0;
}

function scheduleActivityFlush(): void {
  if (activityFlushFrame !== null) {
    return;
  }

  activityFlushFrame = requestAnimationFrame(flushActivity);
}

function queueActivity(activity: ActivityEvent): void {
  pendingActivity.push(activity);

  const running = activity.data.running;

  if (typeof running === "boolean") {
    pendingRunning = running;
  }

  scheduleActivityFlush();
}

function setActivityBatch(
  batch: ActivityEvent[],
  running: boolean | undefined,
): void {
  useRuntimeStore.setState((state) => {
    let activity = state.activity;

    if (batch.length > 0) {
      if (batch.length >= MAX_ACTIVITY_EVENTS) {
        activity = batch.slice(-MAX_ACTIVITY_EVENTS);
      } else if (state.activity.length + batch.length <= MAX_ACTIVITY_EVENTS) {
        activity = [...state.activity, ...batch];
      } else {
        const keep = MAX_ACTIVITY_EVENTS - batch.length;

        activity = [...state.activity.slice(-keep), ...batch];
      }
    }

    if (running === undefined) {
      return {
        activity,
      };
    }

    return {
      activity,
      running,
    };
  });

  // v1.1.3Z: route conversation-relevant events into the message pipeline
  // (streaming bubbles + the run-end bookkeeping that used to leave the
  // composer permanently disabled after one message).
  for (const event of batch) {
    handleConversationEvent(event);
  }
}

// handleConversationEvent mirrors activity stream events into the real
// conversation view and repairs the run state machine.
function handleConversationEvent(event: ActivityEvent): void {
  const store = useRuntimeStore.getState();

  switch (event.type) {
    case "response": {
      const content =
        typeof event.data.caption === "string" ? event.data.caption : "";

      if (content) {
        useRuntimeStore.setState({
          streaming: {
            content,
            reasoning: store.streaming?.reasoning ?? "",
          },
        });
      }

      break;
    }

    case "reasoning": {
      const reasoning =
        typeof event.data.caption === "string" ? event.data.caption : "";

      if (reasoning) {
        useRuntimeStore.setState({
          streaming: {
            content: store.streaming?.content ?? "",
            reasoning,
          },
        });
      }

      break;
    }

    case "done":
    case "error": {
      // THE v1.1.2Z dead-composer fix: a finished or failed run must
      // always release the composer. The old code only reset `running`
      // on error paths, so a successful reply left it disabled forever.
      useRuntimeStore.setState({ running: false });

      // Reload the persisted conversation so the final assistant message
      // (written by the run goroutine after the done event) replaces the
      // optimistic streaming bubble with the authoritative history.
      const sessionId = useRuntimeStore.getState().activeSessionId;

      if (event.type === "done" && sessionId) {
        window.setTimeout(() => {
          const current = useRuntimeStore.getState();

          if (current.activeSessionId === sessionId) {
            void current.loadSession(sessionId);
          }
        }, 400);
      }

      useRuntimeStore.setState({ streaming: null });

      break;
    }

    default:
      break;
  }
}

export const useRuntimeStore = create<RuntimeState>((set, get) => ({
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

  engine: null,

  messages: [],
  streaming: null,

  pendingAttachments: [],
  attachmentsUploading: false,

  lab: null,
  labLoading: false,
  labError: null,
  activeLabTaskId: null,
  activeLabTask: null,

  researchConfig: null,
  research: null,
  researchLoading: false,
  researchError: null,

  refreshSysinfo: async () => {
    try {
      const sysinfo = await api.sysinfo();

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
      const models = await api.models();

      set({ models });
    } catch (error) {
      set({
        error:
          error instanceof Error ? error.message : "Failed to refresh models.",
      });
    }
  },

  refreshPresets: async () => {
    try {
      const presets = await api.presets();

      set({ presets });
    } catch (error) {
      set({
        error:
          error instanceof Error ? error.message : "Failed to refresh presets.",
      });
    }
  },

  refreshSessions: async () => {
    try {
      const sessions = await api.sessions();

      const current = get().activeSessionId;

      const activeSessionId = resolveActiveSessionID(sessions, current);

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
      const tools = await api.tools();

      set({ tools });
    } catch (error) {
      set({
        error:
          error instanceof Error ? error.message : "Failed to refresh tools.",
      });
    }
  },

  refreshAgentResources: async () => {
    set({
      error: null,
    });

    try {
      const [sysinfo, presets, tools] = await Promise.all([
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

  refreshEngine: async () => {
    try {
      const engine = await api.engine();

      set({ engine });
    } catch {
      // Engine endpoint unreachable — connection state already reflects
      // backend health; leave the last known engine snapshot in place.
    }
  },

  startEnginePolling: () => {
    if (enginePollTimer !== null) {
      return;
    }

    void get().refreshEngine();

    enginePollTimer = window.setInterval(() => {
      void useRuntimeStore.getState().refreshEngine();
    }, 2500);
  },

  loadSession: async (id) => {
    try {
      const detail = await api.sessionDetail(id);

      // Only apply if the session is still the active one.
      if (useRuntimeStore.getState().activeSessionId !== id) {
        return;
      }

      set({
        messages: detail.messages ?? [],
      });
    } catch {
      // Session detail unavailable (fresh session not yet persisted) —
      // an empty conversation is the correct view.
      if (useRuntimeStore.getState().activeSessionId === id) {
        set({ messages: [] });
      }
    }
  },

  refreshLab: async () => {
    set({
      labLoading: true,
      labError: null,
    });

    try {
      const lab = await api.lab();

      const activeLabTaskId = get().activeLabTaskId;

      const activeLabTask = activeLabTaskId
        ? (lab.tasks.find((item) => item.id === activeLabTaskId) ?? null)
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
      const activeLabTask = await api.labTask(taskId);

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
    const session = await api.createSession();

    set((state) => ({
      sessions: [session, ...state.sessions],
      activeSessionId: session.id,
    }));

    return session;
  },

  selectSession: (id) => {
    if (get().activeSessionId === id) {
      return;
    }

    get().disconnectActivity();

    set({
      activeSessionId: id,
      error: null,
      activity: [],
      messages: [],
      streaming: null,
      running: false,
      pendingAttachments: [],
    });

    if (id) {
      get().connectActivity();
      void get().loadSession(id);
    }
  },

  deleteSession: async (id) => {
    await api.deleteSession(id);

    if (get().activeSessionId === id) {
      get().disconnectActivity();
    }

    set((state) => {
      const sessions = state.sessions.filter((session) => session.id !== id);

      const activeSessionId = resolveActiveSessionID(
        sessions,
        state.activeSessionId === id ? null : state.activeSessionId,
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
    const sessionId = get().activeSessionId;

    if (!sessionId) {
      throw new Error("No active session.");
    }

    if (!message.trim()) {
      return;
    }

    set({
      running: true,
      error: null,
    });

    // v1.1.3Z: optimistic user bubble — the conversation shows the sent
    // message immediately, before any streaming event arrives.
    const attachmentNames = get().pendingAttachments.map((item) => item.name);

    set((state) => ({
      messages: [
        ...state.messages,
        {
          role: "user" as const,
          content: message.trim(),
          ...(attachmentNames.length > 0 ? { attachments: attachmentNames } : {}),
        },
      ],
      streaming: null,
    }));

    const attachmentIds = get().pendingAttachments.map((item) => item.id);

    set({ pendingAttachments: [] });

    try {
      await api.run({
        sessionId,
        message: message.trim(),
        ...(attachmentIds.length > 0 ? { attachmentIds } : {}),
      });

      await get().refreshSessions();
    } catch (error) {
      set({
        error: error instanceof Error ? error.message : "Agent run failed.",
        running: false,
      });

      throw error;
    }
  },

  regenerate: async () => {
    const sessionId = get().activeSessionId;

    if (!sessionId) {
      return;
    }

    if (get().running) {
      return;
    }

    set({
      running: true,
      error: null,
      streaming: null,
    });

    try {
      await api.run({ sessionId, message: "", regenerate: true });

      // Drop the trailing assistant bubble optimistically; the reload on
      // done restores the authoritative history.
      set((state) => {
        const messages = [...state.messages];

        for (;;) {
          const last = messages[messages.length - 1];

          if (
            messages.length > 0 &&
            last &&
            (last.role === "assistant" || last.role === "tool")
          ) {
            messages.pop();

            continue;
          }

          break;
        }

        return { messages };
      });

      await get().refreshSessions();
    } catch (error) {
      set({
        error: error instanceof Error ? error.message : "Regenerate failed.",
        running: false,
      });

      throw error;
    }
  },

  uploadFiles: async (files) => {
    const sessionId = get().activeSessionId;

    if (!sessionId || files.length === 0) {
      return;
    }

    set({ attachmentsUploading: true, error: null });

    try {
      const response = await api.uploadAttachments(sessionId, files);

      set((state) => ({
        pendingAttachments: [...state.pendingAttachments, ...response.attachments],
        attachmentsUploading: false,
      }));

      if (response.failed.length > 0) {
        set({
          error: response.failed
            .map((item) => `${item.name}: ${item.error}`)
            .join("; "),
        });
      }
    } catch (error) {
      set({
        attachmentsUploading: false,
        error: error instanceof Error ? error.message : "Upload failed.",
      });

      throw error;
    }
  },

  removePendingAttachment: async (id) => {
    set((state) => ({
      pendingAttachments: state.pendingAttachments.filter(
        (item) => item.id !== id,
      ),
    }));

    try {
      await api.deleteAttachment(id);
    } catch {
      // The staged file will be cleaned with the store; removing it from
      // the composer is the user-visible contract and must not fail.
    }
  },

  abort: async () => {
    const sessionId = get().activeSessionId;

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
      const response = await api.labAction(payload);

      if (!response.ok) {
        throw new Error(response.error || "Coding Lab action failed.");
      }

      await get().refreshLab();

      const activeTaskId = get().activeLabTaskId;

      if (activeTaskId) {
        await get().loadLabTask(activeTaskId);
      }

      return response.result;
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Coding Lab action failed.";

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
      const researchConfig = await api.researchConfig();

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
    const query = payload.query.trim();

    if (!query) {
      throw new Error("Research query is required.");
    }

    set({
      researchLoading: true,
      researchError: null,
    });

    try {
      const research = await api.research({
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
        error instanceof Error ? error.message : "Research request failed.";

      set({
        researchLoading: false,
        researchError: message,
      });

      throw error;
    }
  },

  connectActivity: () => {
    const sessionId = get().activeSessionId;

    if (!sessionId) {
      return;
    }

    if (
      socket &&
      activitySessionId === sessionId &&
      (socket.readyState === WebSocket.OPEN ||
        socket.readyState === WebSocket.CONNECTING)
    ) {
      return;
    }

    resetPendingActivity();

    if (socket) {
      socket.close();
      socket = null;
    }

    activitySessionId = sessionId;

    set({
      connection: "connecting",
    });

    const ws = new WebSocket(activityWebSocketURL(sessionId));

    socket = ws;

    ws.onopen = () => {
      if (socket !== ws || activitySessionId !== get().activeSessionId) {
        return;
      }

      set({
        connection: "connected",
      });
    };

    ws.onmessage = (event) => {
      if (socket !== ws || activitySessionId !== get().activeSessionId) {
        return;
      }

      try {
        const payload = JSON.parse(event.data) as APIActivityEvent;

        queueActivity(normalizeActivity(payload));
      } catch {
        // Ignore malformed activity frames.
      }
    };

    ws.onerror = () => {
      if (socket !== ws || activitySessionId !== get().activeSessionId) {
        return;
      }

      set({
        connection: "error",
      });
    };

    ws.onclose = () => {
      if (socket !== ws || activitySessionId !== get().activeSessionId) {
        return;
      }

      socket = null;

      set({
        connection: "disconnected",
      });
    };
  },

  disconnectActivity: () => {
    activitySessionId = null;

    resetPendingActivity();

    if (socket) {
      socket.close();
      socket = null;
    }

    set({
      connection: "idle",
    });
  },

  clearActivity: () => {
    resetPendingActivity();

    set({
      activity: [],
    });
  },
}));

export function getRuntimeState(): RuntimeState {
  return useRuntimeStore.getState();
}
