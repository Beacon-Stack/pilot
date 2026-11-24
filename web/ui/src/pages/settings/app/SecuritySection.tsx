import { useEffect, useState, type FormEvent } from "react";
import { toast } from "sonner";
import { apiFetch } from "@/api/client";
import { cardStyle, sectionHeaderStyle } from "@/lib/styles";

type Required = "disabled_for_local" | "enabled";

interface AuthSettings {
  required: Required;
  username: string;
  password_set: boolean;
}

const label: React.CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: 6,
  fontSize: 12,
  color: "var(--color-text-secondary)",
};

const input: React.CSSProperties = {
  height: 34,
  padding: "0 10px",
  borderRadius: 6,
  border: "1px solid var(--color-border-default)",
  background: "var(--color-bg-elevated)",
  color: "var(--color-text-primary)",
  fontSize: 13,
  maxWidth: 320,
};

const hint: React.CSSProperties = { fontSize: 12, color: "var(--color-text-muted)", margin: 0, lineHeight: 1.5 };

/**
 * Browser login settings, modelled on Sonarr's Security section: by
 * default nobody on the local network has to log in, and a username and
 * password protect access from anywhere else.
 */
export default function SecuritySection() {
  const [saved, setSaved] = useState<AuthSettings | null>(null);
  const [required, setRequired] = useState<Required>("disabled_for_local");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [currentPassword, setCurrentPassword] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    apiFetch<AuthSettings>("/system/auth")
      .then((s) => {
        setSaved(s);
        setRequired(s.required);
        setUsername(s.username);
      })
      .catch((err: Error) => toast.error(err.message));
  }, []);

  const dirty =
    saved !== null && (required !== saved.required || username.trim() !== saved.username || password !== "");
  // Once a login exists, changing it takes the current password.
  const needsCurrent = saved?.password_set === true && dirty;
  const canSave = dirty && !saving && (!needsCurrent || currentPassword !== "");

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setSaving(true);
    try {
      const body: Record<string, string> = { required, username: username.trim() };
      if (password) body.password = password;
      if (currentPassword) body.current_password = currentPassword;
      const s = await apiFetch<AuthSettings>("/system/auth", { method: "PUT", body: JSON.stringify(body) });
      setSaved(s);
      setRequired(s.required);
      setUsername(s.username);
      setPassword("");
      setCurrentPassword("");
      toast.success(s.username ? "Login saved" : "Login removed");
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setSaving(false);
    }
  }

  return (
    <form style={cardStyle} onSubmit={onSubmit}>
      <p style={sectionHeaderStyle}>Security</p>
      <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
        <label style={label}>
          Authentication required
          <select
            value={required}
            onChange={(e) => setRequired(e.target.value as Required)}
            style={input}
            disabled={saved === null}
          >
            <option value="disabled_for_local">Disabled for local addresses</option>
            <option value="enabled">Enabled</option>
          </select>
        </label>
        <p style={hint}>
          {required === "disabled_for_local"
            ? "Devices on your own network open Pilot without logging in. Anyone connecting from outside it (for example through a reverse proxy) needs the username and password below."
            : "Everyone has to log in, including devices on your own network."}
        </p>

        <label style={label}>
          Username
          <input
            type="text"
            autoComplete="username"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            style={input}
            disabled={saved === null}
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
            disabled={saved === null}
          />
        </label>
        {needsCurrent && (
          <label style={label}>
            Current password
            <input
              type="password"
              autoComplete="current-password"
              value={currentPassword}
              placeholder="Needed to change the login"
              onChange={(e) => setCurrentPassword(e.target.value)}
              style={input}
            />
          </label>
        )}
        {saved !== null && !saved.username && (
          <p style={hint}>No login is set up, so Pilot can only be reached from your local network.</p>
        )}
        <p style={hint}>Scripts and other apps keep using the API key below.</p>

        <div>
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
        </div>
      </div>
    </form>
  );
}
