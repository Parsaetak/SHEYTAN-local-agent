import {
	memo,
	useEffect,
	useMemo,
	useRef,
} from "react";

import type { ActivityEvent } from "./store";
import { useRuntimeStore } from "./store";

const MAX_VISIBLE_EVENTS = 50;

function formatActivity(
	activity: ActivityEvent,
): string {
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

const ActivityItem = memo(
	function ActivityItem({
		item,
	}: {
		item: ActivityEvent;
	}) {
		return (
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
		);
	},
);

function ActivityStream() {
	const activity = useRuntimeStore(
		(state) => state.activity,
	);

	const clearActivity =
		useRuntimeStore(
			(state) =>
				state.clearActivity,
		);

	const activityEndRef =
		useRef<HTMLDivElement | null>(
			null,
		);

	const scrollFrameRef =
		useRef<number | null>(null);

	const visibleActivity = useMemo(
		() =>
			activity.length >
			MAX_VISIBLE_EVENTS
				? activity.slice(
						-MAX_VISIBLE_EVENTS,
					)
				: activity,
		[activity],
	);

	useEffect(() => {
		if (
			scrollFrameRef.current !==
			null
		) {
			cancelAnimationFrame(
				scrollFrameRef.current,
			);
		}

		scrollFrameRef.current =
			requestAnimationFrame(() => {
				scrollFrameRef.current =
					null;

				activityEndRef.current?.scrollIntoView(
					{
						behavior: "auto",
						block: "nearest",
					},
				);
			});

		return () => {
			if (
				scrollFrameRef.current !==
				null
			) {
				cancelAnimationFrame(
					scrollFrameRef.current,
				);

				scrollFrameRef.current =
					null;
			}
		};
	}, [activity]);

	return (
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
					visibleActivity.map(
						(item) => (
							<ActivityItem
								key={item.id}
								item={item}
							/>
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
	);
}

export default ActivityStream;
