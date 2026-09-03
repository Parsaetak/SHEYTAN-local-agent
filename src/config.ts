export const API_BASE = "/api";

function browserOrigin(): string {
  return window.location.origin;
}

export const WS_BASE = `${window.location.protocol === "https:" ? "wss:" : "ws:"}//${window.location.host}`;

export function activityWebSocketURL(sessionId?: string | null): string {
  const query =
    sessionId && sessionId.trim()
      ? `?sessionId=${encodeURIComponent(sessionId)}`
      : "";

  return `${WS_BASE}/ws/activity${query}`;
}

export function apiOrigin(): string {
  return browserOrigin();
}
