import { useState, type FormEvent } from "react";
import { KeyRound } from "lucide-react";
import { login, safeNextPath } from "@/lib/auth";

export default function LoginPage() {
  const [apiKey, setApiKey] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await login(apiKey.trim());
      const next = safeNextPath(new URLSearchParams(window.location.search).get("next"));
      // Full navigation so every query refetches with the new cookie.
      window.location.assign(next);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Login failed.");
      setSubmitting(false);
    }
  }

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
          <KeyRound size={20} strokeWidth={1.75} style={{ color: "var(--color-accent)" }} />
          <h1 style={{ margin: 0, fontSize: 18, fontWeight: 600, color: "var(--color-text-primary)" }}>
            Sign in
          </h1>
        </div>
        <p style={{ margin: 0, fontSize: 13, lineHeight: 1.5, color: "var(--color-text-secondary)" }}>
          Enter the API key from <code>auth.api_key</code> in your config.yaml.
        </p>
        <label style={{ display: "flex", flexDirection: "column", gap: 6, fontSize: 12, color: "var(--color-text-secondary)" }}>
          API key
          <input
            type="password"
            autoComplete="current-password"
            autoFocus
            required
            value={apiKey}
            onChange={(e) => setApiKey(e.target.value)}
            style={{
              height: 36,
              padding: "0 10px",
              borderRadius: 6,
              border: "1px solid var(--color-border-default)",
              background: "var(--color-bg-elevated)",
              color: "var(--color-text-primary)",
              fontSize: 13,
              fontFamily: "var(--font-family-mono, monospace)",
            }}
          />
        </label>
        {error && (
          <div role="alert" style={{ fontSize: 13, color: "var(--color-danger)" }}>
            {error}
          </div>
        )}
        <button
          type="submit"
          disabled={submitting || apiKey.trim() === ""}
          style={{
            height: 36,
            borderRadius: 6,
            border: "none",
            background: "var(--color-accent)",
            color: "var(--color-accent-fg)",
            fontSize: 13,
            fontWeight: 500,
            cursor: submitting ? "default" : "pointer",
            opacity: submitting || apiKey.trim() === "" ? 0.6 : 1,
          }}
        >
          {submitting ? "Signing in…" : "Sign in"}
        </button>
      </form>
    </div>
  );
}
