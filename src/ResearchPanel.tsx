import {
	type FormEvent,
	useEffect,
	useMemo,
	useState,
} from "react";

import type { ResearchResult } from "./api";
import { useRuntimeStore } from "./store";

function formatDuration(value: number): string {
	if (!Number.isFinite(value) || value < 0) {
		return "—";
	}

	// Go time.Duration is JSON encoded as nanoseconds.
	const milliseconds = value / 1_000_000;

	if (milliseconds < 1000) {
		return `${Math.round(milliseconds)} ms`;
	}

	return `${(milliseconds / 1000).toFixed(2)} s`;
}

function formatPublishedAt(value?: string): string {
	if (!value) {
		return "Unknown date";
	}

	const date = new Date(value);

	if (Number.isNaN(date.getTime())) {
		return value;
	}

	return date.toLocaleDateString([], {
		year: "numeric",
		month: "short",
		day: "numeric",
	});
}

function formatScore(value: number): string {
	if (!Number.isFinite(value)) {
		return "0.00";
	}

	return value.toFixed(2);
}

function resultDomain(url: string): string {
	try {
		return new URL(url).hostname;
	} catch {
		return url;
	}
}

function ResearchResultCard({
	result,
	index,
}: {
	result: ResearchResult;
	index: number;
}) {
	const domain = useMemo(
		() => resultDomain(result.url),
		[result.url],
	);

	return (
		<article className="research-result">
			<div className="research-result-index">
				{String(index + 1).padStart(2, "0")}
			</div>

			<div className="research-result-body">
				<div className="research-result-topline">
					<div className="research-result-source">
						<span className="research-provider">
							{result.provider}
						</span>

						<span className="research-source">
							{result.source}
						</span>

						<span className="research-domain">
							{domain}
						</span>
					</div>

					<div className="research-scores">
						<span>
							AUTHORITY{" "}
							<strong>
								{result.authority}
							</strong>
						</span>

						<span>
							MATCH{" "}
							<strong>
								{formatScore(
									result.matchScore,
								)}
							</strong>
						</span>
					</div>
				</div>

				<h3>{result.title || "Untitled result"}</h3>

				{result.snippet ? (
					<p>{result.snippet}</p>
				) : (
					<p className="muted">
						No summary was provided by the source.
					</p>
				)}

				<div className="research-result-footer">
					<span>
						{formatPublishedAt(
							result.publishedAt,
						)}
					</span>

					{result.contentHash ? (
						<span>
							HASH{" "}
							<code>
								{result.contentHash.slice(
									0,
									16,
								)}
							</code>
						</span>
					) : null}

					<a
						href={result.url}
						target="_blank"
						rel="noreferrer noopener"
					>
						Open source ↗
					</a>
				</div>
			</div>
		</article>
	);
}

export default function ResearchPanel() {
	const {
		researchConfig,
		research,
		researchLoading,
		researchError,
		loadResearchConfig,
		searchResearch,
	} = useRuntimeStore();

	const [query, setQuery] = useState("");
	const [backend, setBackend] = useState("");

	useEffect(() => {
		void loadResearchConfig();
	}, [loadResearchConfig]);

	useEffect(() => {
		if (!researchConfig) {
			return;
		}

		setBackend((current) => {
			if (
				current &&
				researchConfig.providers.includes(current)
			) {
				return current;
			}

			return researchConfig.backend || "";
		});
	}, [researchConfig]);

	async function handleSubmit(
		event: FormEvent<HTMLFormElement>,
	) {
		event.preventDefault();

		const value = query.trim();

		if (!value || researchLoading) {
			return;
		}

		await searchResearch({
			query: value,
			backend: backend || undefined,
		});
	}

	const providerOptions =
		researchConfig?.providers ?? [];

	const resultCount =
		research?.results.length ?? 0;

	return (
		<section className="research-panel">
			<div className="research-panel-header">
				<div>
					<span className="eyebrow">
						RESEARCH ENGINE
					</span>

					<h2>
						External knowledge, locally orchestrated
					</h2>

					<p>
						Search evidence through the configured
						providers. Results remain external
						provenance rather than trusted memory.
					</p>
				</div>

				<div className="research-engine-state">
					<span
						className={`status-dot ${
							researchConfig ? "ready" : ""
						}`}
					/>

					<span>
						{researchConfig
							? "ENGINE READY"
							: "LOADING ENGINE"}
					</span>
				</div>
			</div>

			<form
				className="research-search"
				onSubmit={(event) =>
					void handleSubmit(event)
				}
			>
				<div className="research-query-wrap">
					<label htmlFor="research-query">
						<span className="eyebrow">QUERY</span>

						<input
							id="research-query"
							type="text"
							value={query}
							onChange={(event) =>
								setQuery(event.target.value)
							}
							placeholder="Search GitHub, Reddit, and the web..."
							disabled={researchLoading}
							autoComplete="off"
						/>
					</label>

					<label htmlFor="research-backend">
						<span className="eyebrow">
							BACKEND
						</span>

						<select
							id="research-backend"
							value={backend}
							onChange={(event) =>
								setBackend(event.target.value)
							}
							disabled={
								researchLoading ||
								providerOptions.length === 0
							}
						>
							{backend &&
							!providerOptions.includes(
								backend,
							) ? (
								<option value={backend}>
									{backend}
								</option>
							) : null}

							{providerOptions.length ===
							0 ? (
								<option value="">
									Auto
								</option>
							) : (
								<>
									<option value="">
										Auto
									</option>

									{providerOptions.map(
										(provider) => (
											<option
												key={
													provider
												}
												value={
													provider
												}
											>
												{provider}
											</option>
										),
									)}
								</>
							)}
						</select>
					</label>

					<button
						type="submit"
						className="send-button"
						disabled={
							researchLoading ||
							!query.trim()
						}
					>
						{researchLoading
							? "Searching…"
							: "Search →"}
					</button>
				</div>

				<div className="research-search-meta">
					<span>
						{researchConfig
							? `${providerOptions.length} provider${
									providerOptions.length === 1
										? ""
										: "s"
								} available`
							: "Discovering providers…"}
					</span>

					<span>
						Backend:{" "}
						<strong>
							{backend || "auto"}
						</strong>
					</span>
				</div>
			</form>

			{researchError ? (
				<div
					className="error-banner"
					role="alert"
				>
					<span>{researchError}</span>
				</div>
			) : null}

			{research ? (
				<section className="research-results-section">
					<div className="research-results-heading">
						<div>
							<span className="eyebrow">
								EVIDENCE
							</span>

							<strong>
								{resultCount} result
								{resultCount === 1
									? ""
									: "s"}
							</strong>

							<span className="research-results-query">
								“{research.query}”
							</span>
						</div>

						<div className="research-results-meta">
							<span>
								Provider:{" "}
								<strong>
									{research.provider ||
										"unknown"}
								</strong>
							</span>

							<span>
								Duration:{" "}
								<strong>
									{formatDuration(
										research.duration,
									)}
								</strong>
							</span>
						</div>
					</div>

					{research.results.length > 0 ? (
						<div className="research-result-list">
							{research.results.map(
								(result, index) => (
									<ResearchResultCard
										key={`${result.url}-${index}`}
										result={result}
										index={index}
									/>
								),
							)}
						</div>
					) : (
						<div className="research-empty">
							<div className="activity-empty-mark">
								⌕
							</div>

							<strong>
								No evidence returned
							</strong>

							<span>
								The configured providers did not
								return usable results for this
								query.
							</span>
						</div>
					)}
				</section>
			) : (
				<div className="research-empty">
					<div className="activity-empty-mark">
						⌕
					</div>

					<strong>
						Research engine standing by
					</strong>

					<span>
						Submit a query to retrieve external
						evidence.
					</span>
				</div>
			)}
		</section>
	);
}
