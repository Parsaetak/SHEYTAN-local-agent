import {
	type FormEvent,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";

import type { ActivityEvent } from "./store";
import { useRuntimeStore } from "./store";

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

function AgentBody() {
	const {
		models,
		sessions,
		activeSessionId,
		loading,
		error,
		activity,
		running,
		loadInitialState,
		createSession,
		deleteSession,
		run,
		abort,
		connectActivity,
		disconnectActivity,
		clearActivity,
	} = useRuntimeStore();

	const [message, setMessage] = useState("");

	const activityEndRef =
		useRef<HTMLDivElement | null>(null);

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
	}, [
		loadInitialState,
		connectActivity,
		disconnectActivity,
	]);

	const activeSession = useMemo(
		() =>
			sessions.find(
				(session) =>
					session.id ===
					activeSessionId,
			) ?? null,
		[sessions, activeSessionId],
	);

	const loadedModels =
		models?.loaded ?? [];

	const localModels =
		models?.local ?? [];

	useEffect(() => {
		activityEndRef.current?.scrollIntoView({
			behavior: "smooth",
			block: "nearest",
		});
	}, [activity]);

	async function handleSubmit(
		event: FormEvent<HTMLFormElement>,
	) {
		event.preventDefault();

		const value =
			message.trim();

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
			await deleteSession(
				activeSessionId,
			);
		} catch {
			// The store exposes the error state.
		}
	}

	useEffect(() => {
		function handleNewSessionRequest() {
			void handleNewSession();
		}

		window.addEventListener(
			"sheytan:new-session",
			handleNewSessionRequest,
		);

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
				<div className="activity-panel">
					<div className="panel-heading">
						<div>
							<span className="eyebrow">
								LIVE
							</span>

							<strong>
								Activity
							</strong>
						</div>

						<button
							type="button"
							className="text-button"
							onClick={
								clearActivity
							}
							disabled={
								activity.length ===
								0
							}
						>
							Clear
						</button>
					</div>

					<div className="activity-stream">
						{activity.length ===
						0 ? (
							<div className="activity-empty">
								<div className="activity-empty-mark">
									✦
								</div>

								<strong>
									Awaiting the
									first
									operation
								</strong>

								<span>
									Send a task
									below to
									begin a
									local
									agent run.
								</span>
							</div>
						) : (
							activity.map(
								(item) => (
									<article
										className="activity-item"
										key={
											item.id
										}
									>
										<div className="activity-marker">
											<span />
										</div>

										<div className="activity-content">
											<div className="activity-meta">
												<span>
													{
														item.type
													}
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
								),
							)
						)}

						<div
							ref={
								activityEndRef
							}
						/>
					</div>
				</div>

				<aside className="runtime-panel">
					<div className="panel-heading">
						<div>
							<span className="eyebrow">
								RUNTIME
							</span>

							<strong>
								System
							</strong>
						</div>
					</div>

					<div className="runtime-grid">
						<div className="metric-card">
							<span>
								Sessions
							</span>

							<strong>
								{
									sessions.length
								}
							</strong>
						</div>

						<div className="metric-card">
							<span>
								Local models
							</span>

							<strong>
								{
									localModels.length
								}
							</strong>
						</div>

						<div className="metric-card">
							<span>
								Loaded
							</span>

							<strong>
								{
									loadedModels.length
								}
							</strong>
						</div>

						<div className="metric-card">
							<span>
								Events
							</span>

							<strong>
								{
									activity.length
								}
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

					<div className="runtime-section">
						<span className="eyebrow">
							CONTROL
						</span>

						<div className="header-actions">
							<button
								type="button"
								className="secondary-button"
								onClick={() =>
									void handleNewSession()
								}
							>
								New session
							</button>

							<button
								type="button"
								className="secondary-button"
								onClick={() =>
									void handleDeleteSession()
								}
								disabled={
									!activeSessionId
								}
							>
								Delete session
							</button>
						</div>
					</div>
				</aside>
			</section>

			<form
				className="composer"
				onSubmit={
					handleSubmit
				}
			>
				<div className="composer-shell">
					<textarea
						value={message}
						onChange={(event) =>
							setMessage(
								event.target
									.value,
							)
						}
						placeholder={
							activeSessionId
								? "Describe what SHEYTAN should forge..."
								: "Create a session to begin..."
						}
						disabled={
							!activeSessionId ||
							running
						}
						rows={3}
						onKeyDown={(event) => {
							if (
								event.key ===
									"Enter" &&
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
	);
}

export default AgentBody;
