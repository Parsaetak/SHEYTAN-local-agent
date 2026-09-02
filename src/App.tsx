import {
	type FormEvent,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";

import LabPanel from "./LabPanel";
import type { ActivityEvent } from "./store";
import { useRuntimeStore } from "./store";

type AppView = "agent" | "lab";

function formatActivity(activity: ActivityEvent): string {
	const data = activity.data;

	if (typeof data.message === "string") {
		return data.message;
	}

	if (typeof data.content === "string") {
		return data.content;
	}

	if (typeof data.text === "string") {
		return data.text;
	}

	if (typeof data.error === "string") {
		return data.error;
	}

	return activity.type;
}

function App() {
	const {
		app,
		models,
		sessions,
		activeSessionId,
		connection,
		loading,
		error,
		activity,
		running,
		loadInitialState,
		createSession,
		selectSession,
		deleteSession,
		run,
		abort,
		connectActivity,
		disconnectActivity,
		clearActivity,
	} = useRuntimeStore();

	const [message, setMessage] = useState("");
	const [view, setView] = useState<AppView>("agent");
	const activityEndRef = useRef<HTMLDivElement | null>(null);

	useEffect(() => {
		let cancelled = false;

		void loadInitialState().then(() => {
			if (!cancelled) {
				connectActivity();
			}
		});

		return () => {
			cancelled = true;
			disconnectActivity();
		};
	}, [connectActivity, disconnectActivity, loadInitialState]);

	useEffect(() => {
		if (view !== "agent") {
			return;
		}

		activityEndRef.current?.scrollIntoView({
			behavior: "smooth",
			block: "nearest",
		});
	}, [activity, view]);

	const activeSession = useMemo(
		() =>
			sessions.find(
				(session) => session.id === activeSessionId,
			) ?? null,
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
			// The existing error state remains authoritative.
		}
	}

	async function handleDeleteSession() {
		if (!activeSessionId) {
			return;
		}

		try {
			await deleteSession(activeSessionId);
		} catch {
			// The existing error state remains authoritative.
		}
	}

	const statusLabel =
		connection === "connected"
			? "Connected"
			: connection === "connecting"
				? "Connecting"
				: connection === "error"
					? "Connection error"
					: "Offline";

	return (
		<div className="app-shell">
			<header className="topbar">
				<div className="brand">
					<div className="brand-mark">S</div>

					<div className="brand-copy">
						<strong>SHEYTAN</strong>
						<span>Local Agent</span>
					</div>
				</div>

				<div className="topbar-status">
					<span
						className={`status-dot ${
							connection === "connected" ? "ready" : ""
						}`}
					/>
					<span>{statusLabel}</span>
				</div>

				<div className="topbar-meta">
					<span>{app?.appVersion ?? "ZETA"}</span>
				</div>
			</header>

			<div className="app-body">
				<aside className="sidebar">
					<div className="sidebar-heading">
						<div>
							<span className="eyebrow">WORKSPACE</span>
							<strong>Sessions</strong>
						</div>

						<button
							type="button"
							className="icon-button"
							onClick={() => void handleNewSession()}
							title="New session"
							aria-label="New session"
						>
							+
						</button>
					</div>

					<div className="session-list">
						{sessions.length === 0 && !loading ? (
							<div className="empty-sidebar">
								No sessions yet.
							</div>
						) : (
							sessions.map((session) => (
								<button
									type="button"
									key={session.id}
									className={`session-item ${
										session.id === activeSessionId
											? "active"
											: ""
									}`}
									onClick={() =>
										selectSession(session.id)
									}
								>
									<span className="session-icon">◈</span>

									<span className="session-copy">
										<strong>
											{session.title ||
												"Untitled session"}
										</strong>
										<span>
											{session.id.slice(0, 8)}
										</span>
									</span>
								</button>
							))
						)}
					</div>

					<div className="sidebar-footer">
						<span>Native runtime</span>
						<span>Go + Wails</span>
					</div>
				</aside>

				<main className="workspace">
					<section className="workspace-header">
						<div>
							<span className="eyebrow">
								{view === "agent"
									? "AGENT"
									: "CODING LAB"}
							</span>

							<h1>
								{view === "agent"
									? activeSession?.title ||
										"Forge a new task"
									: "Autonomous engineering"}
							</h1>
						</div>

						<div className="header-actions">
							<button
								type="button"
								className="secondary-button"
								onClick={() => setView("agent")}
								aria-pressed={view === "agent"}
							>
								Agent
							</button>

							<button
								type="button"
								className="secondary-button"
								onClick={() => setView("lab")}
								aria-pressed={view === "lab"}
							>
								Coding Lab
							</button>

							{view === "agent" ? (
								<>
									<span className="runtime-pill">
										{running ? "RUNNING" : "READY"}
									</span>

									<button
										type="button"
										className="secondary-button"
										onClick={() =>
											void handleDeleteSession()
										}
										disabled={!activeSessionId}
									>
										Delete
									</button>
								</>
							) : null}
						</div>
					</section>

					{view === "lab" ? (
						<LabPanel />
					) : (
						<>
							<section className="workspace-content">
								<div className="activity-panel">
									<div className="panel-heading">
										<div>
											<span className="eyebrow">
												LIVE
											</span>
											<strong>Activity</strong>
										</div>

										<button
											type="button"
											className="text-button"
											onClick={clearActivity}
											disabled={
												activity.length === 0
											}
										>
											Clear
										</button>
									</div>

									<div className="activity-stream">
										{activity.length === 0 ? (
											<div className="activity-empty">
												<div className="activity-empty-mark">
													✦
												</div>
												<strong>
													Awaiting the first operation
												</strong>
												<span>
													Send a task below to begin a
													local agent run.
												</span>
											</div>
										) : (
											activity.map((item) => (
												<article
													className="activity-item"
													key={item.id}
												>
													<div className="activity-marker">
														<span />
													</div>

													<div className="activity-content">
														<div className="activity-meta">
															<span>
																{item.type}
															</span>

															<time>
																{new Date(
																	item.timestamp,
																).toLocaleTimeString(
																	[],
																	{
																		hour: "2-digit",
																		minute: "2-digit",
																		second: "2-digit",
																	},
																)}
															</time>
														</div>

														<p>
															{formatActivity(
																item,
															)}
														</p>
													</div>
												</article>
											))
										)}

										<div ref={activityEndRef} />
									</div>
								</div>

								<aside className="runtime-panel">
									<div className="panel-heading">
										<div>
											<span className="eyebrow">
												RUNTIME
											</span>
											<strong>System</strong>
										</div>
									</div>

									<div className="runtime-grid">
										<div className="metric-card">
											<span>Sessions</span>
											<strong>
												{sessions.length}
											</strong>
										</div>

										<div className="metric-card">
											<span>Local models</span>
											<strong>
												{localModels.length}
											</strong>
										</div>

										<div className="metric-card">
											<span>Loaded</span>
											<strong>
												{loadedModels.length}
											</strong>
										</div>

										<div className="metric-card">
											<span>Events</span>
											<strong>
												{activity.length}
											</strong>
										</div>
									</div>

									<div className="runtime-section">
										<span className="eyebrow">
											ENGINE
										</span>

										<div className="engine-status">
											<span
												className={`status-dot ${
													models?.llamaRunning
														? "ready"
														: ""
												}`}
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
										<span className="eyebrow">
											SESSION
										</span>

										<div className="session-detail">
											<span>ID</span>
											<strong>
												{activeSessionId
													? activeSessionId.slice(
															0,
															16,
														)
													: "None"}
											</strong>
										</div>

										<div className="session-detail">
											<span>MODEL</span>
											<strong>
												{activeSession?.model ||
													"Automatic"}
											</strong>
										</div>
									</div>
								</aside>
							</section>

							<form
								className="composer"
								onSubmit={handleSubmit}
							>
								<div className="composer-shell">
									<textarea
										value={message}
										onChange={(event) =>
											setMessage(
												event.target.value,
											)
										}
										placeholder={
											activeSessionId
												? "Describe what SHEYTAN should forge..."
												: "Create a session to begin..."
										}
										disabled={
											!activeSessionId || running
										}
										rows={3}
										onKeyDown={(event) => {
											if (
												event.key === "Enter" &&
												(event.ctrlKey ||
													event.metaKey)
											) {
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
												onClick={() =>
													void abort()
												}
											>
												Stop
											</button>
										) : (
											<button
												type="submit"
												className="send-button"
												disabled={
													!activeSessionId ||
													!message.trim() ||
													loading
												}
											>
												Forge →
											</button>
										)}
									</div>
								</div>
							</form>

							{error ? (
								<div
									className="error-banner"
									role="alert"
								>
									<span>{error}</span>
								</div>
							) : null}
						</>
					)}
				</main>
			</div>
		</div>
	);
}

export default App;
