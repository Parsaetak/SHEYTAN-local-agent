import { api, type Session } from "./api";
import { useRuntimeStore } from "./store";

const INITIALIZATION_TTL_MS = 30_000;

let initializationPromise: Promise<void> | null = null;

let initializedAt = 0;

function resolveActiveSessionID(
  sessions: Session[],
  currentID: string | null,
): string | null {
  if (currentID && sessions.some((session) => session.id === currentID)) {
    return currentID;
  }

  return sessions[0]?.id ?? null;
}

async function initializeAgentOnce(): Promise<void> {
  useRuntimeStore.setState({
    loading: true,
    error: null,
  });

  try {
    const [app, sessions] = await Promise.all([api.state(), api.sessions()]);

    const current = useRuntimeStore.getState().activeSessionId;

    const activeSessionId = resolveActiveSessionID(sessions, current);

    useRuntimeStore.setState({
      app,
      sessions,
      activeSessionId,
      loading: false,
    });

    void useRuntimeStore.getState().refreshModels();
  } catch (error) {
    useRuntimeStore.setState({
      loading: false,
      error:
        error instanceof Error ? error.message : "Failed to initialize Agent.",
    });

    throw error;
  }
}

export function initializeAgent(): Promise<void> {
  if (initializationPromise) {
    return initializationPromise;
  }

  const now = Date.now();

  if (initializedAt > 0 && now - initializedAt < INITIALIZATION_TTL_MS) {
    return Promise.resolve();
  }

  initializationPromise = initializeAgentOnce()
    .then(() => {
      initializedAt = Date.now();
    })
    .catch((error) => {
      initializedAt = 0;
      throw error;
    })
    .finally(() => {
      initializationPromise = null;
    });

  return initializationPromise;
}
