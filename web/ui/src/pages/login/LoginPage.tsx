import { useEffect, useState, type FormEvent } from "react";
import { Lock } from "lucide-react";
import { getAuthStatus, login, safeNextPath, type AuthStatus } from "@/lib/auth";

const inputStyle: React.CSSProperties = {
  height: 36,
  padding: "0 10px",
  borderRadius: 6,
  border: "1px solid var(--color-border-default)",
  background: "var(--color-bg-elevated)",
  color: "var(--color-text-primary)",
  fontSize: 13,
};

const labelStyle: React.CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: 6,
  fontSize: 12,
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
      .catch((err: unknown) => setError(err instanceof Error ? err.message : "Could not reach Pilot."));
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

  const noLogin = status !== null && !status.credentials_configured;
  const canSubmit = !submitting && username.trim() !== "" && password !== "";

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
            Sign in
          </h1>
        </div>

        {status === null && !error && (
          <p style={{ margin: 0, fontSize: 13, color: "var(--color-text-secondary)" }}>Checking…</p>
        )}

        {status?.untrusted_host && (
          <p style={{ margin: 0, fontSize: 13, lineHeight: 1.5, color: "var(--color-text-secondary)" }}>
            Pilot doesn't recognise <strong>{status.untrusted_host}</strong> as its own address, so it
            asks for a login here. Open it by IP address or local name
            {noLogin ? "" : ", sign in below,"} or add this name to <code>server.allowed_hosts</code>.
          </p>
        )}

        {noLogin && !status?.untrusted_host && (
          <p style={{ margin: 0, fontSize: 13, lineHeight: 1.5, color: "var(--color-text-secondary)" }}>
            Pilot only accepts connections from your local network until you set a username and
            password. Open Pilot from a device on your network and go to{" "}
            <strong>Settings → App Settings → Security</strong>.
          </p>
        )}

        {status !== null && !noLogin && (
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

        {status !== null && !noLogin && (
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
