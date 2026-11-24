import { useEffect, useState } from "react";
import type { CSSProperties, FormEvent } from "react";
import { apiFetch } from "./api";
import { card, sectionHeader } from "./styles";

// The Security settings card, the same in every Beacon app. It talks to
// /api/v1/system/auth (pulse/pkg/auth/authapi).

type Required = "none" | "disabled_for_local" | "enabled";

interface AuthSettings {
  required: Required;
  username: string;
  password_set: boolean;
  local_networks: string[];
  client_address: string;
  client_is_local: boolean;
}

const MODES: { value: Required; label: string; help: string }[] = [
  {
    value: "none",
    label: "None",
    help: "Devices on your local network open this app without logging in. Connections from anywhere else are refused.",
  },
  {
    value: "disabled_for_local",
    label: "Login, except on my local network",
    help: "Devices on your local network open this app without logging in. Anyone else needs the username and password.",
  },
  {
    value: "enabled",
    label: "Login for everyone",
    help: "Everyone needs the username and password, including devices on your local network.",
  },
];

const label: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: 6,
  fontSize: 12,
  color: "var(--color-text-secondary)",
};

const input: CSSProperties = {
  height: 34,
  padding: "0 10px",
  borderRadius: 6,
  border: "1px solid var(--color-border-default)",
  background: "var(--color-bg-elevated)",
  color: "var(--color-text-primary)",
  fontSize: 13,
  maxWidth: 320,
};

const hint: CSSProperties = { fontSize: 12, color: "var(--color-text-muted)", margin: 0, lineHeight: 1.5 };

function parseNetworks(text: string): string[] {
  return text
    .split(/[\s,]+/)
    .map((s) => s.trim())
    .filter(Boolean);
}

export default function Security() {
  const [saved, setSaved] = useState<AuthSettings | null>(null);
  const [required, setRequired] = useState<Required>("none");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [currentPassword, setCurrentPassword] = useState("");
  const [networks, setNetworks] = useState("");
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState<{ ok: boolean; text: string } | null>(null);

  function show(s: AuthSettings) {
    setSaved(s);
    setRequired(s.required);
    setUsername(s.username);
    setNetworks(s.local_networks.join("\n"));
    setPassword("");
    setCurrentPassword("");
  }

  useEffect(() => {
    apiFetch<AuthSettings>("/system/auth")
      .then(show)
      .catch((err: Error) => setMessage({ ok: false, text: err.message }));
  }, []);

  const networkList = parseNetworks(networks);
  const dirty =
    saved !== null &&
    (required !== saved.required ||
      username.trim() !== saved.username ||
      password !== "" ||
      networkList.join(",") !== saved.local_networks.join(","));
  // Once a login exists, changing anything here takes the current password.
  const needsCurrent = saved?.password_set === true && dirty;
  const needsLogin = required !== "none";
  const canSave = dirty && !saving && (!needsCurrent || currentPassword !== "");
  const mode = MODES.find((m) => m.value === required) ?? MODES[0];

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setSaving(true);
    setMessage(null);
    try {
      const body: Record<string, unknown> = {
        required,
        username: username.trim(),
        local_networks: networkList,
      };
      if (password) body.password = password;
      if (currentPassword) body.current_password = currentPassword;
      const s = await apiFetch<AuthSettings>("/system/auth", { method: "PUT", body: JSON.stringify(body) });
      show(s);
      setMessage({ ok: true, text: "Saved." });
    } catch (err) {
      const e2 = err as Error & { detail?: string };
      setMessage({ ok: false, text: e2.detail || e2.message });
    } finally {
      setSaving(false);
    }
  }

  return (
    <form style={card} onSubmit={onSubmit}>
      <p style={sectionHeader}>Security</p>
      <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
        <label style={label}>
          Login
          <select
            value={required}
            onChange={(e) => setRequired(e.target.value as Required)}
            style={input}
            disabled={saved === null}
          >
            {MODES.map((m) => (
              <option key={m.value} value={m.value}>
                {m.label}
              </option>
            ))}
          </select>
        </label>
        <p style={hint}>{mode.help}</p>
        {required === "none" && (
          <p style={hint}>
            This is only safe while the app can't be reached from the internet. If it can, choose a login.
          </p>
        )}

        {(needsLogin || saved?.password_set) && (
          <>
            <label style={label}>
              Username
              <input
                type="text"
                autoComplete="username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                style={input}
              />
            </label>
            <label style={label}>
              {saved?.password_set ? "New password" : "Password"}
              <input
                type="password"
                autoComplete="new-password"
                value={password}
                placeholder={saved?.password_set ? "Leave blank to keep the current password" : "At least 8 characters"}
                onChange={(e) => setPassword(e.target.value)}
                style={input}
              />
            </label>
          </>
        )}

        <label style={label}>
          Also treat these networks as local
          <textarea
            value={networks}
            onChange={(e) => setNetworks(e.target.value)}
            placeholder={"One per line, for example\n100.64.0.0/10"}
            rows={3}
            style={{ ...input, height: "auto", padding: "8px 10px", fontFamily: "var(--font-family-mono, monospace)" }}
            disabled={saved === null}
          />
        </label>
        <p style={hint}>
          Home networks (192.168.x.x, 10.x.x.x, 172.16–31.x.x) already count as local. Add a range here if yours is
          different, or if the app sees a different address for your devices.
          {saved && (
            <>
              {" "}
              You appear to this app as <strong>{saved.client_address}</strong>, which{" "}
              {saved.client_is_local ? "counts as local" : "does not count as local"}.
            </>
          )}
        </p>

        {needsCurrent && (
          <label style={label}>
            Current password
            <input
              type="password"
              autoComplete="current-password"
              value={currentPassword}
              placeholder="Needed to change these settings"
              onChange={(e) => setCurrentPassword(e.target.value)}
              style={input}
            />
          </label>
        )}

        <p style={hint}>Scripts and other apps use the API key, whatever is chosen here.</p>

        <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
          <button
            type="submit"
            disabled={!canSave}
            style={{
              height: 32,
              padding: "0 14px",
              borderRadius: 6,
              border: "none",
              background: "var(--color-accent)",
              color: "var(--color-accent-fg)",
              fontSize: 13,
              fontWeight: 500,
              cursor: canSave ? "pointer" : "default",
              opacity: canSave ? 1 : 0.5,
            }}
          >
            {saving ? "Saving…" : "Save"}
          </button>
          {message && (
            <span
              role={message.ok ? "status" : "alert"}
              style={{ fontSize: 13, color: message.ok ? "var(--color-success)" : "var(--color-danger)" }}
            >
              {message.text}
            </span>
          )}
        </div>
      </div>
    </form>
  );
}
