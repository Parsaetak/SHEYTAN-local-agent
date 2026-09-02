import { api } from "./api";
import { useRuntimeStore } from "./store";

export async function initializeAgent(): Promise<void> {
	const store = useRuntimeStore.getState();

	useRuntimeStore.setState({
		loading: true,
		error: null,
	});

	try {
		const [app] = await Promise.all([
			api.state(),
			store.refreshSessions(),
		]);

		useRuntimeStore.setState({
			app,
			loading: false,
		});

		void store.refreshModels();
	} catch (error) {
		useRuntimeStore.setState({
			loading: false,
			error:
				error instanceof Error
					? error.message
					: "Failed to initialize Agent.",
		});
	}
}
