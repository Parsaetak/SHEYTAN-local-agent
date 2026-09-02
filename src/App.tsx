import { useEffect, useState } from "react";

function App() {
	const [ready, setReady] = useState(false);

	useEffect(() => {
		setReady(true);
	}, []);

	return (
		<div className="app-shell">
			<header className="app-header">
				<div className="brand">
					<span className="brand-mark">S</span>
					<div>
						<strong>SHEYTAN</strong>
						<span>Local Agent</span>
					</div>
				</div>

				<div className="runtime-status" aria-live="polite">
					<span className={`status-dot${ready ? " ready" : ""}`} />
					<span>{ready ? "Ready" : "Starting"}</span>
				</div>
			</header>

			<main className="app-main">
				<section className="workspace">
					<div className="workspace-badge">VERSION ZETA</div>

					<h1>What shall we forge today?</h1>

					<p>
						A local AI coding and research workspace powered by the
						SHEYTAN runtime.
					</p>

					<div className="workspace-grid">
						<div className="workspace-card">
							<span>01</span>
							<h2>Agent</h2>
							<p>
								Run local reasoning, tools, coding tasks, and autonomous
								actions.
							</p>
						</div>

						<div className="workspace-card">
							<span>02</span>
							<h2>Research</h2>
							<p>
								Search connected sources such as GitHub, Reddit, and web
								backends.
							</p>
						</div>

						<div className="workspace-card">
							<span>03</span>
							<h2>Workspace</h2>
							<p>
								Build, inspect, repair, and iterate on projects from one
								desktop environment.
							</p>
						</div>
					</div>
				</section>
			</main>

			<footer className="app-footer">
				<span>Native Desktop</span>
				<span>•</span>
				<span>Go Runtime</span>
				<span>•</span>
				<span>React Interface</span>
			</footer>
		</div>
	);
}

export default App;
