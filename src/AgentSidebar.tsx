import { useRuntimeStore } from "./store";

function AgentSidebar() {
	const {
		sessions,
		activeSessionId,
		loading,
		selectSession,
	} = useRuntimeStore();

	function requestNewSession() {
		window.dispatchEvent(
			new CustomEvent(
				"sheytan:new-session",
			),
		);
	}

	return (
		<>
			<div className="sidebar-heading sidebar-heading-secondary">
				<div>
					<span className="eyebrow">
						AGENT
					</span>

					<strong>
						Sessions
					</strong>
				</div>

				<button
					type="button"
					className="icon-button"
					onClick={
						requestNewSession
					}
					title="New session"
					aria-label="New session"
				>
					+
				</button>
			</div>

			<div className="session-list">
				{sessions.length === 0 &&
				!loading ? (
					<div className="empty-sidebar">
						No sessions yet.
					</div>
				) : (
					sessions.map(
						(session) => (
							<button
								type="button"
								key={
									session.id
								}
								className={`session-item ${
									session.id ===
									activeSessionId
										? "active"
										: ""
								}`}
								onClick={() =>
									selectSession(
										session.id,
									)
								}
							>
								<span className="session-icon">
									◈
								</span>

								<span className="session-copy">
									<strong>
										{session.title ||
											"Untitled session"}
									</strong>

									<span>
										{session.id.slice(
											0,
											8,
										)}
									</span>
								</span>
							</button>
						),
					)
				)}
			</div>
		</>
	);
}

export default AgentSidebar;
