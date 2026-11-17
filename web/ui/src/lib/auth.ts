// Browser session auth. The server issues an HttpOnly `pilot_session`
// cookie from POST /api/v1/auth/login; fetch/WebSocket send it
// automatically for same-origin requests, so no token handling is needed
// here — only login, logout, and bouncing to /login on a 401.

export const LOGIN_PATH = "/login";

const AUTH_ENDPOINTS = ["/api/v1/auth/login", "/api/v1/auth/logout"];

export async function login(apiKey: string): Promise<void> {
  const res = await fetch("/api/v1/auth/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ api_key: apiKey }),
  });
  if (res.status === 401) throw new Error("Invalid API key.");
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
 * Wraps window.fetch so any 401 from our own /api sends the user to the
 * login page. Installed once at startup; covers apiFetch, the vendored
 * @beacon-shared code, and raw fetch calls alike.
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
