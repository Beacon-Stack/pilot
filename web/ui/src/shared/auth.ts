// Browser side of the shared login (pulse/pkg/auth), the same in every
// Beacon app. By default the server lets the local network straight in.
// When a login is required it issues an HttpOnly session cookie, which
// fetch and WebSocket send on their own for same-origin requests, so
// there is no token to handle here: only login, logout, status, and
// sending the user to /login on a 401.

export const LOGIN_PATH = "/login";

const AUTH_ENDPOINTS = ["/api/v1/auth/login", "/api/v1/auth/logout", "/api/v1/auth/status"];

export interface AuthStatus {
  /** The app's name, for messages. */
  app: string;
  authenticated: boolean;
  /** How this browser got in: a login session, the local network, or not at all. */
  method: "session" | "local" | "api_key" | "none";
  /** The rule in force. */
  required: "none" | "disabled_for_local" | "enabled";
  credentials_configured: boolean;
  /** Why this browser was not let in. */
  reason?: "login_required" | "not_local" | "untrusted_host" | "bad_api_key";
  /** The same, in words. */
  detail?: string;
  untrusted_host?: string;
}

export async function getAuthStatus(): Promise<AuthStatus> {
  const res = await fetch("/api/v1/auth/status");
  if (!res.ok) throw new Error(`Could not check sign-in status (HTTP ${res.status}).`);
  return (await res.json()) as AuthStatus;
}

export async function login(username: string, password: string): Promise<void> {
  const res = await fetch("/api/v1/auth/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password }),
  });
  if (res.status === 401 || res.status === 429) {
    const body = (await res.json().catch(() => null)) as { detail?: string } | null;
    throw new Error(body?.detail || "Incorrect username or password.");
  }
  if (!res.ok) throw new Error(`Login failed (HTTP ${res.status}).`);
}

export async function logout(): Promise<void> {
  await fetch("/api/v1/auth/logout", { method: "POST" });
  window.location.assign(LOGIN_PATH);
}

/** Only allow same-app relative paths as the post-login destination. */
export function safeNextPath(next: string | null): string {
  if (!next || !next.startsWith("/") || next.startsWith("//") || next.startsWith(LOGIN_PATH)) {
    return "/";
  }
  return next;
}

function requestPath(input: RequestInfo | URL): string | null {
  try {
    const raw = typeof input === "string" ? input : input instanceof URL ? input.href : input.url;
    const url = new URL(raw, window.location.origin);
    return url.origin === window.location.origin ? url.pathname : null;
  } catch {
    return null;
  }
}

/**
 * Wraps window.fetch so any 401 from the app's own /api sends the user to
 * the login page, which says why they weren't let in. Install once at
 * startup; it covers every fetch in the app.
 */
export function installAuthRedirect(): void {
  const original = window.fetch.bind(window);
  window.fetch = async (input, init) => {
    const res = await original(input, init);
    if (res.status === 401) {
      const path = requestPath(input);
      if (
        path?.startsWith("/api/") &&
        !AUTH_ENDPOINTS.includes(path) &&
        window.location.pathname !== LOGIN_PATH
      ) {
        const next = window.location.pathname + window.location.search;
        window.location.assign(`${LOGIN_PATH}?next=${encodeURIComponent(next)}`);
      }
    }
    return res;
  };
}
