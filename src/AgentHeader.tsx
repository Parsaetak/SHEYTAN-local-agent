import { memo } from "react";

import { useRuntimeStore } from "./store";

const AgentHeader = memo(
	function AgentHeader() {
		const activeSessionId =
			useRuntimeStore(
				(state) =>
					state.activeSessionId,
			);

		const sessions =
			useRuntimeStore(
				(state) => state.sessions,
			);

		const running =
			useRuntimeStore(
				(state) => state.running,
			);

		const activeSession =
			sessions.find(
				(session) =>
					session.id ===
					activeSessionId,
			) ?? null;

		return (
			<>
				<h1>
					{activeSession?.title ||
						"Forge a new task"}
				</h1>

				<div className="header-actions">
					<span className="runtime-pill">
						{running
							? "RUNNING"
							: "READY"}
					</span>
				</div>
			</>
		);
	},
);

export default AgentHeader;
