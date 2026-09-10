import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import App from "./App";
import { initPerfHUD, recordStreamUpdate } from "./perf-hud";
import { setStreamUpdateRecorder } from "./store";
import "./styles.css";
import "./layers.css";
import "./settings.css";
import "./motion.css";

const rootElement = document.getElementById("root");

if (!rootElement) {
  throw new Error("SHEYTAN Local Agent: root element was not found.");
}

// Phase 4: initialize the frame-budget diagnostic HUD. The HUD is OFF by
// default — toggle with Ctrl+Shift+P or window.__shtnTogglePerfHUD().
initPerfHUD();

// Wire the stream-update recorder so the store's coalesced flushes are
// counted by the HUD (one count per coalesced flush, never per token).
setStreamUpdateRecorder(recordStreamUpdate);

createRoot(rootElement).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
