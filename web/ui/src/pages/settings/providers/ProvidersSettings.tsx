import { useState } from "react";
import PageHeader from "@/components/PageHeader";
import {
  useProviderStatus,
  useSetProviderOverride,
  useClearProviderOverride,
  type ProviderStatus,
} from "@/api/providers";

// Settings → Providers
//
// Manages overrides for third-party metadata API keys. Pilot ships
// with a baked-in TVDB key (via ldflag in CI); this page lets
// operators replace it with their own if the bundled one gets rate-
// limited or leaked. Changes take effect on the next Pilot restart.
//
// The baked-in key is NEVER shown here. We only ever display:
//   - whether a baked default exists ("Using built-in default")
//   - whether an override is active
//   - a redacted preview of the override (last 3 chars)
//
// This matches the server-side promise in internal/api/v1/providers.go.

export default function ProvidersSettings() {
  return (
    <div style={{ padding: 24, maxWidth: 720 }}>
      <PageHeader
        title="Providers"
        description="Override baked-in third-party API keys. Changes take effect after the next Pilot restart."
      />

      <ProviderRow
        name="tvdb"
        label="TVDB (TheMovieDB) API Key"
        signupUrl="https://www.themoviedb.org/settings/api"
        description="Used for TV series metadata lookup. Required to add new series."
      />
    </div>
  );
}

interface ProviderRowProps {
  name: string;
  label: string;
  signupUrl: string;
  description: string;
}

function ProviderRow({ name, label, signupUrl, description }: ProviderRowProps) {
  const { data: status, isLoading } = useProviderStatus(name);
  const setOverride = useSetProviderOverride(name);
  const clearOverride = useClearProviderOverride(name);

  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState("");
  const [error, setError] = useState<string | null>(null);

  if (isLoading || !status) {
    return (
      <Card>
        <div className="skeleton" style={{ height: 14, borderRadius: 4, width: "40%", marginBottom: 8 }} />
        <div className="skeleton" style={{ height: 12, borderRadius: 4, width: "70%" }} />
      </Card>
    );
  }

  const handleSave = async () => {
    setError(null);
    const value = draft.trim();
    if (!value) {
      setError("Key cannot be empty");
      return;
    }
    try {
      await setOverride.mutateAsync(value);
      setDraft("");
      setEditing(false);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to save override");
    }
  };

  const handleClear = async () => {
    setError(null);
    try {
      await clearOverride.mutateAsync();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to clear override");
    }
  };

  return (
    <Card>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: 16 }}>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ fontSize: 14, fontWeight: 600, color: "var(--color-text-primary)", marginBottom: 4 }}>
            {label}
          </div>
          <div style={{ fontSize: 12, color: "var(--color-text-muted)", marginBottom: 12 }}>
            {description}
          </div>
          <StatusPill status={status} />
        </div>
      </div>

      {editing ? (
        <div style={{ marginTop: 16, display: "flex", gap: 8, flexWrap: "wrap" }}>
          <input
            type="password"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            placeholder="Paste your API key"
            autoFocus
            style={{
              flex: 1,
              minWidth: 240,
              padding: "8px 10px",
              borderRadius: 6,
              border: "1px solid var(--color-border-default)",
              background: "var(--color-bg-input)",
              color: "var(--color-text-primary)",
              fontSize: 13,
              fontFamily: "var(--font-family-mono)",
            }}
          />
          <button
            onClick={handleSave}
            disabled={setOverride.isPending || !draft.trim()}
            style={primaryBtn(setOverride.isPending || !draft.trim())}
          >
            {setOverride.isPending ? "Saving..." : "Save"}
          </button>
          <button
            onClick={() => {
              setEditing(false);
              setDraft("");
              setError(null);
            }}
            style={ghostBtn()}
          >
            Cancel
          </button>
        </div>
      ) : (
        <div style={{ marginTop: 16, display: "flex", gap: 8, flexWrap: "wrap" }}>
          <button onClick={() => setEditing(true)} style={primaryBtn(false)}>
            {status.hasOverride ? "Replace override" : "Set override"}
          </button>
          {status.hasOverride && (
            <button
              onClick={handleClear}
              disabled={clearOverride.isPending}
              style={ghostBtn()}
            >
              {clearOverride.isPending ? "Clearing..." : "Clear override"}
            </button>
          )}
          <a
            href={signupUrl}
            target="_blank"
            rel="noopener noreferrer"
            style={{
              fontSize: 12,
              color: "var(--color-text-muted)",
              textDecoration: "none",
              alignSelf: "center",
              marginLeft: "auto",
            }}
          >
            Get your own key →
          </a>
        </div>
      )}

      {error && (
        <div
          style={{
            marginTop: 12,
            padding: "8px 10px",
            borderRadius: 6,
            background: "var(--color-error-subtle, rgba(220, 38, 38, 0.1))",
            color: "var(--color-error, #dc2626)",
            fontSize: 12,
          }}
        >
          {error}
        </div>
      )}
    </Card>
  );
}

function StatusPill({ status }: { status: ProviderStatus }) {
  if (status.hasOverride) {
    return (
      <div style={pillStyle("var(--color-accent-muted)", "var(--color-accent-hover)")}>
        Override active
        <span style={{ marginLeft: 8, fontFamily: "var(--font-family-mono)", opacity: 0.8 }}>
          {status.preview}
        </span>
      </div>
    );
  }
  if (status.hasDefault) {
    return (
      <div style={pillStyle("var(--color-bg-elevated)", "var(--color-text-secondary)")}>
        Using built-in default
      </div>
    );
  }
  return (
    <div style={pillStyle("var(--color-warning-subtle, rgba(234, 179, 8, 0.15))", "var(--color-warning, #ca8a04)")}>
      No key configured — series lookup will fail
    </div>
  );
}

function pillStyle(bg: string, fg: string): React.CSSProperties {
  return {
    display: "inline-block",
    padding: "4px 10px",
    borderRadius: 999,
    background: bg,
    color: fg,
    fontSize: 11,
    fontWeight: 500,
    letterSpacing: "0.02em",
  };
}

function primaryBtn(disabled: boolean): React.CSSProperties {
  return {
    padding: "8px 14px",
    borderRadius: 6,
    border: "none",
    background: disabled ? "var(--color-bg-elevated)" : "var(--color-accent)",
    color: disabled ? "var(--color-text-muted)" : "white",
    fontSize: 13,
    fontWeight: 500,
    cursor: disabled ? "not-allowed" : "pointer",
  };
}

function ghostBtn(): React.CSSProperties {
  return {
    padding: "8px 14px",
    borderRadius: 6,
    border: "1px solid var(--color-border-default)",
    background: "transparent",
    color: "var(--color-text-secondary)",
    fontSize: 13,
    fontWeight: 500,
    cursor: "pointer",
  };
}

function Card({ children }: { children: React.ReactNode }) {
  return (
    <div
      style={{
        background: "var(--color-bg-surface)",
        border: "1px solid var(--color-border-subtle)",
        borderRadius: 8,
        padding: 16,
        boxShadow: "var(--shadow-card)",
      }}
    >
      {children}
    </div>
  );
}
