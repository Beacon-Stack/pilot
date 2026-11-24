import { useEffect, useState } from "react";
import type { CSSProperties, FormEvent } from "react";
import { Lock } from "lucide-react";
import { getAuthStatus, login, safeNextPath } from "./auth";
import type { AuthStatus } from "./auth";

// The sign-in page, the same in every Beacon app. It is also where
// someone lands when the app won't let them in for another reason (not
// on the local network, or opened under a name the app doesn't know), so
// it says which.

const inputStyle: CSSProperties = {
  height: 36,
  padding: "0 10px",
  borderRadius: 6,
  border: "1px solid var(--color-border-default)",
  background: "var(--color-bg-elevated)",
  color: "var(--color-text-primary)",
  fontSize: 13,
};

const labelStyle: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: 6,
  fontSize: 12,
  color: "var(--color-text-secondary)",
};

const noteStyle: CSSProperties = {
  margin: 0,
  fontSize: 13,
  lineHeight: 1.5,
  color: "var(--color-text-secondary)",
};

function nextPath(): string {
  return safeNextPath(new URLSearchParams(window.location.search).get("next"));
}

export default function LoginPage() {
  const [status, setStatus] = useState<AuthStatus | null>(null);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    getAuthStatus()
      .then((s) => {
        // Already allowed in (e.g. on the local network): skip the form.
        if (s.authenticated) window.location.replace(nextPath());
        else setStatus(s);
      })
      .catch((err: unknown) => setError(err instanceof Error ? err.message : "Could not reach the app."));
  }, []);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await login(username.trim(), password);
      // Full navigation so every query refetches with the new cookie.
      window.location.assign(nextPath());
    } catch (err) {
      setError(err instanceof Error ? err.message : "Login failed.");
      setSubmitting(false);
    }
  }

  // A login form only helps when a login is what's missing.
  const canLogIn = status !== null && status.required !== "none";
  const canSubmit = !submitting && username.trim() !== "" && password !== "";
  const blocked = status !== null && status.reason !== undefined && status.reason !== "login_required";

  return (
    <div
      style={{
        minHeight: "100vh",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        padding: "16px",
        background: "var(--color-bg-base)",
      }}
    >
      <form
        onSubmit={onSubmit}
        style={{
          width: "100%",
          maxWidth: 380,
          background: "var(--color-bg-surface)",
          border: "1px solid var(--color-border-subtle)",
          borderRadius: 10,
          padding: 28,
          display: "flex",
          flexDirection: "column",
          gap: 16,
        }}
      >
        <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
          <Lock size={20} strokeWidth={1.75} style={{ color: "var(--color-accent)" }} />
          <h1 style={{ margin: 0, fontSize: 18, fontWeight: 600, color: "var(--color-text-primary)" }}>
            {canLogIn || status === null ? "Sign in" : `${status.app} isn't available here`}
          </h1>
        </div>

        {status === null && !error && <p style={noteStyle}>Checking…</p>}

        {blocked && status.detail && <p style={noteStyle}>{status.detail}</p>}

        {canLogIn && (
          <>
            <label style={labelStyle}>
              Username
              <input
                type="text"
                autoComplete="username"
                autoFocus
                required
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                style={inputStyle}
              />
            </label>
            <label style={labelStyle}>
              Password
              <input
                type="password"
                autoComplete="current-password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                style={inputStyle}
              />
            </label>
          </>
        )}

        {error && (
          <div role="alert" style={{ fontSize: 13, color: "var(--color-danger)" }}>
            {error}
          </div>
        )}

        {canLogIn && (
          <button
            type="submit"
            disabled={!canSubmit}
            style={{
              height: 36,
              borderRadius: 6,
              border: "none",
              background: "var(--color-accent)",
              color: "var(--color-accent-fg)",
              fontSize: 13,
              fontWeight: 500,
              cursor: canSubmit ? "pointer" : "default",
              opacity: canSubmit ? 1 : 0.6,
            }}
          >
            {submitting ? "Signing in…" : "Sign in"}
          </button>
        )}
      </form>
    </div>
  );
}
