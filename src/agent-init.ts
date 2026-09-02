import {
	api,
	type Session,
} from "./api";
import { useRuntimeStore } from "./store";

let initializationPromise:
	| Promise<void>
	| null = null;

function resolveActiveSessionID(
	sessions: Session[],
	currentID: string | null,
): string | null {
	if (
		currentID &&
		sessions.some(
			(session) =>
				session.id === currentID,
		)
	) {
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
		const [
			app,
			sessions,
		] = await Promise.all([
			api.state(),
			api.sessions(),
		]);

		const current =
			useRuntimeStore.getState()
				.activeSessionId;

		const activeSessionId =
			resolveActiveSessionID(
				sessions,
				current,
			);

		useRuntimeStore.setState({
			app,
			sessions,
			activeSessionId,
			loading: false,
		});

		void useRuntimeStore
			.getState()
			.refreshModels();
	} catch (error) {
		useRuntimeStore.setState({
			loading: false,
			error:
				error instanceof Error
					? error.message
					: "Failed to initialize Agent.",
		});

		throw error;
	}
}

export function initializeAgent(): Promise<void> {
	if (!initializationPromise) {
		initializationPromise =
			initializeAgentOnce().catch(
				(error) => {
					initializationPromise =
						null;

					throw error;
				},
			);
	}

	return initializationPromise;
}
