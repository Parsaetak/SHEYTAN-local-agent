export type WorkspaceView =
	| "agent"
	| "lab"
	| "research";

export type WorkspaceLayer = {
	id: WorkspaceView;
	label: string;
	eyebrow: string;
	title: string;
	description: string;
	icon: string;
};

export const WORKSPACE_LAYERS: readonly WorkspaceLayer[] = [
	{
		id: "agent",
		label: "Agent",
		eyebrow: "AGENT",
		title: "Forge a new task",
		description:
			"Interactive local intelligence",
		icon: "◈",
	},
	{
		id: "lab",
		label: "Coding Lab",
		eyebrow: "CODING LAB",
		title: "Autonomous engineering",
		description:
			"Execute, verify, and repair",
		icon: "◆",
	},
	{
		id: "research",
		label: "Research",
		eyebrow: "RESEARCH",
		title: "External intelligence",
		description:
			"External evidence and sources",
		icon: "⌕",
	},
] as const;

export function isWorkspaceView(
	value: string,
): value is WorkspaceView {
	return WORKSPACE_LAYERS.some(
		(layer) => layer.id === value,
	);
}

export function getWorkspaceLayer(
	view: WorkspaceView,
): WorkspaceLayer {
	return (
		WORKSPACE_LAYERS.find(
			(layer) => layer.id === view,
		) ?? WORKSPACE_LAYERS[0]
	);
}

export function parseWorkspaceHash(): WorkspaceView {
	if (
		typeof window ===
		"undefined"
	) {
		return "agent";
	}

	const hash = window.location.hash
		.replace(/^#/, "")
		.trim()
		.toLowerCase();

	return isWorkspaceView(hash)
		? hash
		: "agent";
}

export function workspaceHash(
	view: WorkspaceView,
): string {
	return view === "agent"
		? ""
		: `#${view}`;
}

export function getWorkspaceHref(
	view: WorkspaceView,
): string {
	const hash = workspaceHash(view);

	return `${window.location.pathname}${window.location.search}${hash}`;
}
