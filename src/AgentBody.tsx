import { type FormEvent, useEffect, useMemo, useState } from "react";

import ActivityStream from "./ActivityStream";
import { initializeAgent } from "./agent-init";
import { useRuntimeStore } from "./store";

function ActivityCount() {
  const activityCount = useRuntimeStore((state) => state.activity.length);

  return (
    <div className="metric-card">
      <span>Events</span>

      <strong>{activityCount}</strong>
    </div>
  );
}

function AgentBody() {
  const models = useRuntimeStore((state) => state.models);

  const sessions = useRuntimeStore((state) => state.sessions);

  const activeSessionId = useRuntimeStore((state) => state.activeSessionId);

  const loading = useRuntimeStore((state) => state.loading);

  const error = useRuntimeStore((state) => state.error);

  const running = useRuntimeStore((state) => state.running);

  const createSession = useRuntimeStore((state) => state.createSession);

  const deleteSession = useRuntimeStore((state) => state.deleteSession);

  const run = useRuntimeStore((state) => state.run);

  const abort = useRuntimeStore((state) => state.abort);

  const connectActivity = useRuntimeStore((state) => state.connectActivity);

  const disconnectActivity = useRuntimeStore(
    (state) => state.disconnectActivity,
  );

  const [message, setMessage] = useState("");

  useEffect(() => {
    let cancelled = false;

    void initializeAgent()
      .then(() => {
        if (!cancelled) {
          connectActivity();
        }
      })
      .catch(() => {
        // The initialization routine
        // already records the error
        // in the runtime store.
      });

    return () => {
      cancelled = true;
      disconnectActivity();
    };
  }, [connectActivity, disconnectActivity]);

  const activeSession = useMemo(
    () => sessions.find((session) => session.id === activeSessionId) ?? null,
    [sessions, activeSessionId],
  );

  const loadedModels = models?.loaded ?? [];

  const localModels = models?.local ?? [];

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    const value = message.trim();

    if (!value || running) {
      return;
    }

    setMessage("");

    try {
      await run(value);
    } catch {
      // The store exposes the error state.
    }
  }

  async function handleNewSession() {
    try {
      await createSession();
    } catch {
      // The store exposes the error state.
    }
  }

  async function handleDeleteSession() {
    if (!activeSessionId) {
      return;
    }

    try {
      await deleteSession(activeSessionId);
    } catch {
      // The store exposes the error state.
    }
  }

  useEffect(() => {
    function handleNewSessionRequest() {
      void handleNewSession();
    }

    window.addEventListener("sheytan:new-session", handleNewSessionRequest);

    return () => {
      window.removeEventListener(
        "sheytan:new-session",
        handleNewSessionRequest,
      );
    };
  }, [createSession]);

  return (
    <>
      <section className="workspace-content">
        <ActivityStream />

        <aside className="runtime-panel">
          <div className="panel-heading">
            <div>
              <span className="eyebrow">RUNTIME</span>

              <strong>System</strong>
            </div>
          </div>

          <div className="runtime-grid">
            <div className="metric-card">
              <span>Sessions</span>

              <strong>{sessions.length}</strong>
            </div>

            <div className="metric-card">
              <span>Local models</span>

              <strong>{localModels.length}</strong>
            </div>

            <div className="metric-card">
              <span>Loaded</span>

              <strong>{loadedModels.length}</strong>
            </div>

            <ActivityCount />
          </div>

          <div className="runtime-section">
            <span className="eyebrow">ENGINE</span>

            <div className="engine-status">
              <span
                className={`status-dot ${models?.llamaRunning ? "ready" : ""}`}
              />

              <div>
                <strong>
                  {models?.llamaRunning
                    ? "Llama server active"
                    : "Llama server idle"}
                </strong>

                <span>
                  {models?.llamaRunning
                    ? "Inference backend available"
                    : "Start a local model when needed"}
                </span>
              </div>
            </div>
          </div>

          <div className="runtime-section">
            <span className="eyebrow">SESSION</span>

            <div className="session-detail">
              <span>ID</span>

              <strong>
                {activeSessionId ? activeSessionId.slice(0, 16) : "None"}
              </strong>
            </div>

            <div className="session-detail">
              <span>MODEL</span>

              <strong>{activeSession?.model || "Automatic"}</strong>
            </div>
          </div>

          <div className="runtime-section">
            <span className="eyebrow">CONTROL</span>

            <div className="header-actions">
              <button
                type="button"
                className="secondary-button"
                onClick={() => void handleNewSession()}
              >
                New session
              </button>

              <button
                type="button"
                className="secondary-button"
                onClick={() => void handleDeleteSession()}
                disabled={!activeSessionId}
              >
                Delete session
              </button>
            </div>
          </div>
        </aside>
      </section>

      <form className="composer" onSubmit={handleSubmit}>
        <div className="composer-shell">
          <textarea
            value={message}
            onChange={(event) => setMessage(event.target.value)}
            placeholder={
              activeSessionId
                ? "Describe what SHEYTAN should forge..."
                : "Create a session to begin..."
            }
            disabled={!activeSessionId || running}
            rows={3}
            onKeyDown={(event) => {
              if (event.key === "Enter" && (event.ctrlKey || event.metaKey)) {
                event.preventDefault();

                event.currentTarget.form?.requestSubmit();
              }
            }}
          />

          <div className="composer-footer">
            <span>
              {activeSessionId
                ? "Ctrl/Cmd + Enter to run"
                : "Create a session first"}
            </span>

            {running ? (
              <button
                type="button"
                className="stop-button"
                onClick={() => void abort()}
              >
                Stop
              </button>
            ) : (
              <button
                type="submit"
                className="send-button"
                disabled={!activeSessionId || !message.trim() || loading}
              >
                Forge →
              </button>
            )}
          </div>
        </div>
      </form>

      {error ? (
        <div className="error-banner" role="alert">
          <span>{error}</span>
        </div>
      ) : null}
    </>
  );
}

export default AgentBody;
