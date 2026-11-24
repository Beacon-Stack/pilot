import { useState } from "react";
import Modal from "@beacon-shared/Modal";
import { useCreateDownloadClient, useUpdateDownloadClient } from "@/api/download-clients";
import type { DownloadClientConfig, DownloadClientRequest } from "@/types";
import { inputStyle, labelStyle, fieldStyle } from "@/lib/styles";

function strSetting(settings: Record<string, unknown>, key: string): string {
  const v = settings[key];
  return typeof v === "string" ? v : "";
}

interface FormState {
  name: string;
  kind: string;
  enabled: boolean;
  priority: string;
  // qbittorrent
  qb_url: string;
  qb_username: string;
  qb_password: string;
  qb_category: string;
  qb_save_path: string;
  // deluge
  dl_url: string;
  dl_password: string;
  dl_label: string;
  dl_save_path: string;
  // transmission
  trans_url: string;
  trans_username: string;
  trans_password: string;
  // sabnzbd
  sab_url: string;
  sab_api_key: string;
  sab_category: string;
  // nzbget
  nzbget_url: string;
  nzbget_username: string;
  nzbget_password: string;
  nzbget_category: string;
}

function emptyForm(): FormState {
  return {
    name: "", kind: "qbittorrent", enabled: true, priority: "1",
    qb_url: "", qb_username: "", qb_password: "", qb_category: "", qb_save_path: "",
    dl_url: "", dl_password: "", dl_label: "", dl_save_path: "",
    trans_url: "", trans_username: "", trans_password: "",
    sab_url: "", sab_api_key: "", sab_category: "",
    nzbget_url: "", nzbget_username: "", nzbget_password: "", nzbget_category: "",
  };
}

function clientToForm(cfg: DownloadClientConfig): FormState {
  const s = cfg.settings;
  const k = cfg.kind;
  return {
    name: cfg.name,
    kind: k,
    enabled: cfg.enabled,
    priority: String(cfg.priority),
    qb_url: k === "qbittorrent" ? strSetting(s, "url") : "",
    qb_username: k === "qbittorrent" ? strSetting(s, "username") : "",
    qb_password: "",  // never pre-fill
    qb_category: k === "qbittorrent" ? strSetting(s, "category") : "",
    qb_save_path: k === "qbittorrent" ? strSetting(s, "save_path") : "",
    dl_url: k === "deluge" ? strSetting(s, "url") : "",
    dl_password: "",  // never pre-fill
    dl_label: k === "deluge" ? strSetting(s, "label") : "",
    dl_save_path: k === "deluge" ? strSetting(s, "save_path") : "",
    trans_url: k === "transmission" ? strSetting(s, "url") : "",
    trans_username: k === "transmission" ? strSetting(s, "username") : "",
    trans_password: "",  // never pre-fill
    sab_url: k === "sabnzbd" ? strSetting(s, "url") : "",
    sab_api_key: "",  // never pre-fill
    sab_category: k === "sabnzbd" ? strSetting(s, "category") : "",
    nzbget_url: k === "nzbget" ? strSetting(s, "url") : "",
    nzbget_username: k === "nzbget" ? strSetting(s, "username") : "",
    nzbget_password: "",  // never pre-fill
    nzbget_category: k === "nzbget" ? strSetting(s, "category") : "",
  };
}

function formToRequest(f: FormState): DownloadClientRequest {
  let settings: Record<string, unknown>;

  switch (f.kind) {
    case "qbittorrent":
      settings = { url: f.qb_url.trim(), username: f.qb_username.trim() };
      if (f.qb_password.trim()) settings.password = f.qb_password.trim();
      if (f.qb_category.trim()) settings.category = f.qb_category.trim();
      if (f.qb_save_path.trim()) settings.save_path = f.qb_save_path.trim();
      break;
    case "deluge":
      settings = { url: f.dl_url.trim() };
      if (f.dl_password.trim()) settings.password = f.dl_password.trim();
      if (f.dl_label.trim()) settings.label = f.dl_label.trim();
      if (f.dl_save_path.trim()) settings.save_path = f.dl_save_path.trim();
      break;
    case "transmission":
      settings = { url: f.trans_url.trim() };
      if (f.trans_username.trim()) settings.username = f.trans_username.trim();
      if (f.trans_password.trim()) settings.password = f.trans_password.trim();
      break;
    case "sabnzbd":
      settings = { url: f.sab_url.trim() };
      if (f.sab_api_key.trim()) settings.api_key = f.sab_api_key.trim();
      if (f.sab_category.trim()) settings.category = f.sab_category.trim();
      break;
    case "nzbget":
      settings = { url: f.nzbget_url.trim() };
      if (f.nzbget_username.trim()) settings.username = f.nzbget_username.trim();
      if (f.nzbget_password.trim()) settings.password = f.nzbget_password.trim();
      if (f.nzbget_category.trim()) settings.category = f.nzbget_category.trim();
      break;
    default:
      settings = {};
  }

  return {
    name: f.name.trim(),
    kind: f.kind,
    enabled: f.enabled,
    priority: parseInt(f.priority, 10) || 1,
    settings,
  };
}

// Modal

interface ModalProps {
  editing: DownloadClientConfig | null;
  onClose: () => void;
}

export default function DownloadClientModal({ editing, onClose }: ModalProps) {
  const [form, setForm] = useState<FormState>(
    editing ? clientToForm(editing) : emptyForm()
  );
  const [error, setError] = useState<string | null>(null);

  const create = useCreateDownloadClient();
  const update = useUpdateDownloadClient();
  const isPending = create.isPending || update.isPending;

  function set<K extends keyof FormState>(field: K, value: FormState[K]) {
    setForm((f) => ({ ...f, [field]: value }));
    setError(null);
  }

  function handleSubmit() {
    if (!form.name.trim()) { setError("Name is required."); return; }
    const urlMap: Record<string, string> = {
      qbittorrent: form.qb_url, deluge: form.dl_url, transmission: form.trans_url,
      sabnzbd: form.sab_url, nzbget: form.nzbget_url,
    };
    const url = urlMap[form.kind] ?? "";
    if (!url.trim()) { setError("URL is required."); return; }
    if (form.kind === "sabnzbd" && !form.sab_api_key.trim()) { setError("API Key is required."); return; }

    const body = formToRequest(form);

    if (editing) {
      update.mutate(
        { id: editing.id, ...body },
        { onSuccess: onClose, onError: (e) => setError(e.message) }
      );
    } else {
      create.mutate(body, { onSuccess: onClose, onError: (e) => setError(e.message) });
    }
  }

  function focusBorder(e: React.FocusEvent<HTMLInputElement | HTMLSelectElement>) {
    (e.currentTarget as HTMLElement).style.borderColor = "var(--color-accent)";
  }
  function blurBorder(e: React.FocusEvent<HTMLInputElement | HTMLSelectElement>) {
    (e.currentTarget as HTMLElement).style.borderColor = "var(--color-border-default)";
  }

  const sensitiveHint = editing ? (
    <p style={{ margin: "4px 0 0", fontSize: 11, color: "var(--color-text-muted)" }}>
      Masked for security. Enter a new value to update, or leave blank to keep existing.
    </p>
  ) : null;

  return (
    <Modal onClose={onClose} width={540} innerStyle={{ padding: 24, gap: 20, overflowY: "auto" }}>
        {/* Header */}
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
          <h2 style={{ margin: 0, fontSize: 16, fontWeight: 600, color: "var(--color-text-primary)" }}>
            {editing ? "Edit Download Client" : "Add Download Client"}
          </h2>
          <button
            onClick={onClose}
            style={{
              background: "none",
              border: "none",
              cursor: "pointer",
              color: "var(--color-text-muted)",
              fontSize: 18,
              lineHeight: 1,
              padding: "4px 6px",
              borderRadius: 4,
            }}
            onMouseEnter={(e) => { (e.currentTarget as HTMLButtonElement).style.color = "var(--color-text-primary)"; }}
            onMouseLeave={(e) => { (e.currentTarget as HTMLButtonElement).style.color = "var(--color-text-muted)"; }}
          >
            ✕
          </button>
        </div>

        {/* Fields */}
        <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
          <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 16 }}>
            <div style={fieldStyle}>
              <label style={labelStyle}>Name *</label>
              <input
                style={inputStyle}
                value={form.name}
                onChange={(e) => set("name", e.currentTarget.value)}
                onFocus={focusBorder}
                onBlur={blurBorder}
                placeholder="e.g. qBittorrent Local"
                autoFocus
              />
            </div>
            <div style={fieldStyle}>
              <label style={labelStyle}>Client</label>
              <select
                style={{ ...inputStyle, cursor: "pointer" }}
                value={form.kind}
                onChange={(e) => set("kind", e.currentTarget.value)}
                onFocus={focusBorder}
                onBlur={blurBorder}
              >
                <option value="qbittorrent">qBittorrent</option>
                <option value="deluge">Deluge</option>
                <option value="transmission">Transmission</option>
                <option value="sabnzbd">SABnzbd</option>
                <option value="nzbget">NZBGet</option>
              </select>
            </div>
          </div>

          {/* Settings section */}
          <div
            style={{
              background: "var(--color-bg-elevated)",
              border: "1px solid var(--color-border-subtle)",
              borderRadius: 8,
              padding: 16,
              display: "flex",
              flexDirection: "column",
              gap: 14,
            }}
          >
            <p style={{ margin: 0, fontSize: 11, fontWeight: 600, letterSpacing: "0.06em", textTransform: "uppercase", color: "var(--color-text-muted)" }}>
              {{ qbittorrent: "qBittorrent", deluge: "Deluge", transmission: "Transmission", sabnzbd: "SABnzbd", nzbget: "NZBGet" }[form.kind] ?? form.kind} Settings
            </p>

            {form.kind === "qbittorrent" && (
              <>
                <div style={fieldStyle}>
                  <label style={labelStyle}>URL *</label>
                  <input
                    style={inputStyle}
                    value={form.qb_url}
                    onChange={(e) => set("qb_url", e.currentTarget.value)}
                    onFocus={focusBorder}
                    onBlur={blurBorder}
                    placeholder="http://localhost:8080"
                  />
                </div>
                <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 14 }}>
                  <div style={fieldStyle}>
                    <label style={labelStyle}>Username</label>
                    <input
                      style={inputStyle}
                      value={form.qb_username}
                      onChange={(e) => set("qb_username", e.currentTarget.value)}
                      onFocus={focusBorder}
                      onBlur={blurBorder}
                      placeholder="admin"
                      autoComplete="off"
                    />
                  </div>
                  <div style={fieldStyle}>
                    <label style={labelStyle}>Password</label>
                    <input
                      style={inputStyle}
                      type="password"
                      value={form.qb_password}
                      onChange={(e) => set("qb_password", e.currentTarget.value)}
                      onFocus={focusBorder}
                      onBlur={blurBorder}
                      placeholder={editing ? "enter to change" : ""}
                      autoComplete="new-password"
                    />
                    {sensitiveHint}
                  </div>
                </div>
                <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 14 }}>
                  <div style={fieldStyle}>
                    <label style={labelStyle}>Category</label>
                    <input
                      style={inputStyle}
                      value={form.qb_category}
                      onChange={(e) => set("qb_category", e.currentTarget.value)}
                      onFocus={focusBorder}
                      onBlur={blurBorder}
                      placeholder="pilot"
                    />
                  </div>
                  <div style={fieldStyle}>
                    <label style={labelStyle}>Save Path</label>
                    <input
                      style={{ ...inputStyle, fontFamily: "var(--font-family-mono)", fontSize: 12 }}
                      value={form.qb_save_path}
                      onChange={(e) => set("qb_save_path", e.currentTarget.value)}
                      onFocus={focusBorder}
                      onBlur={blurBorder}
                      placeholder="/downloads/tv"
                    />
                  </div>
                </div>
              </>
            )}

            {form.kind === "deluge" && (
              <>
                <div style={fieldStyle}>
                  <label style={labelStyle}>URL *</label>
                  <input
                    style={inputStyle}
                    value={form.dl_url}
                    onChange={(e) => set("dl_url", e.currentTarget.value)}
                    onFocus={focusBorder}
                    onBlur={blurBorder}
                    placeholder="http://localhost:8112"
                  />
                </div>
                <div style={fieldStyle}>
                  <label style={labelStyle}>Password</label>
                  <input
                    style={inputStyle}
                    type="password"
                    value={form.dl_password}
                    onChange={(e) => set("dl_password", e.currentTarget.value)}
                    onFocus={focusBorder}
                    onBlur={blurBorder}
                    placeholder={editing ? "enter to change" : "deluge (default)"}
                    autoComplete="new-password"
                  />
                  {sensitiveHint}
                </div>
                <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 14 }}>
                  <div style={fieldStyle}>
                    <label style={labelStyle}>Label</label>
                    <input
                      style={inputStyle}
                      value={form.dl_label}
                      onChange={(e) => set("dl_label", e.currentTarget.value)}
                      onFocus={focusBorder}
                      onBlur={blurBorder}
                      placeholder="pilot"
                    />
                  </div>
                  <div style={fieldStyle}>
                    <label style={labelStyle}>Save Path</label>
                    <input
                      style={{ ...inputStyle, fontFamily: "var(--font-family-mono)", fontSize: 12 }}
                      value={form.dl_save_path}
                      onChange={(e) => set("dl_save_path", e.currentTarget.value)}
                      onFocus={focusBorder}
                      onBlur={blurBorder}
                      placeholder="/downloads/tv"
                    />
                  </div>
                </div>
              </>
            )}

            {form.kind === "transmission" && (
              <>
                <div style={fieldStyle}>
                  <label style={labelStyle}>URL *</label>
                  <input
                    style={inputStyle}
                    value={form.trans_url}
                    onChange={(e) => set("trans_url", e.currentTarget.value)}
                    onFocus={focusBorder}
                    onBlur={blurBorder}
                    placeholder="http://localhost:9091"
                  />
                </div>
                <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 14 }}>
                  <div style={fieldStyle}>
                    <label style={labelStyle}>Username</label>
                    <input
                      style={inputStyle}
                      value={form.trans_username}
                      onChange={(e) => set("trans_username", e.currentTarget.value)}
                      onFocus={focusBorder}
                      onBlur={blurBorder}
                      autoComplete="off"
                    />
                  </div>
                  <div style={fieldStyle}>
                    <label style={labelStyle}>Password</label>
                    <input
                      style={inputStyle}
                      type="password"
                      value={form.trans_password}
                      onChange={(e) => set("trans_password", e.currentTarget.value)}
                      onFocus={focusBorder}
                      onBlur={blurBorder}
                      placeholder={editing ? "enter to change" : ""}
                      autoComplete="new-password"
                    />
                    {sensitiveHint}
                  </div>
                </div>
              </>
            )}

            {form.kind === "sabnzbd" && (
              <>
                <div style={fieldStyle}>
                  <label style={labelStyle}>URL *</label>
                  <input
                    style={inputStyle}
                    value={form.sab_url}
                    onChange={(e) => set("sab_url", e.currentTarget.value)}
                    onFocus={focusBorder}
                    onBlur={blurBorder}
                    placeholder="http://localhost:8080"
                  />
                </div>
                <div style={fieldStyle}>
                  <label style={labelStyle}>API Key *</label>
                  <input
                    style={inputStyle}
                    type="password"
                    value={form.sab_api_key}
                    onChange={(e) => set("sab_api_key", e.currentTarget.value)}
                    onFocus={focusBorder}
                    onBlur={blurBorder}
                    placeholder={editing ? "enter to change" : ""}
                    autoComplete="new-password"
                  />
                  {sensitiveHint}
                </div>
                <div style={fieldStyle}>
                  <label style={labelStyle}>Category</label>
                  <input
                    style={inputStyle}
                    value={form.sab_category}
                    onChange={(e) => set("sab_category", e.currentTarget.value)}
                    onFocus={focusBorder}
                    onBlur={blurBorder}
                    placeholder="pilot"
                  />
                </div>
              </>
            )}

            {form.kind === "nzbget" && (
              <>
                <div style={fieldStyle}>
                  <label style={labelStyle}>URL *</label>
                  <input
                    style={inputStyle}
                    value={form.nzbget_url}
                    onChange={(e) => set("nzbget_url", e.currentTarget.value)}
                    onFocus={focusBorder}
                    onBlur={blurBorder}
                    placeholder="http://localhost:6789"
                  />
                </div>
                <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 14 }}>
                  <div style={fieldStyle}>
                    <label style={labelStyle}>Username</label>
                    <input
                      style={inputStyle}
                      value={form.nzbget_username}
                      onChange={(e) => set("nzbget_username", e.currentTarget.value)}
                      onFocus={focusBorder}
                      onBlur={blurBorder}
                      placeholder="nzbget"
                      autoComplete="off"
                    />
                  </div>
                  <div style={fieldStyle}>
                    <label style={labelStyle}>Password</label>
                    <input
                      style={inputStyle}
                      type="password"
                      value={form.nzbget_password}
                      onChange={(e) => set("nzbget_password", e.currentTarget.value)}
                      onFocus={focusBorder}
                      onBlur={blurBorder}
                      placeholder={editing ? "enter to change" : ""}
                      autoComplete="new-password"
                    />
                    {sensitiveHint}
                  </div>
                </div>
                <div style={fieldStyle}>
                  <label style={labelStyle}>Category</label>
                  <input
                    style={inputStyle}
                    value={form.nzbget_category}
                    onChange={(e) => set("nzbget_category", e.currentTarget.value)}
                    onFocus={focusBorder}
                    onBlur={blurBorder}
                    placeholder="pilot"
                  />
                </div>
              </>
            )}
          </div>

          {/* Priority + enabled */}
          <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 16 }}>
            <div style={fieldStyle}>
              <label style={labelStyle}>Priority</label>
              <input
                style={inputStyle}
                type="number"
                min="1"
                value={form.priority}
                onChange={(e) => set("priority", e.currentTarget.value)}
                onFocus={focusBorder}
                onBlur={blurBorder}
              />
            </div>
            <div style={{ display: "flex", flexDirection: "column", justifyContent: "flex-end", paddingBottom: 2 }}>
              <label style={{ display: "flex", alignItems: "center", gap: 8, cursor: "pointer", userSelect: "none" }}>
                <input
                  type="checkbox"
                  checked={form.enabled}
                  onChange={(e) => set("enabled", e.currentTarget.checked)}
                  style={{ width: 16, height: 16, cursor: "pointer", accentColor: "var(--color-accent)" }}
                />
                <span style={{ fontSize: 13, color: "var(--color-text-primary)" }}>Enabled</span>
              </label>
            </div>
          </div>
        </div>

        {/* Error */}
        {error && (
          <p style={{ margin: 0, fontSize: 12, color: "var(--color-danger)" }}>{error}</p>
        )}

        {/* Footer */}
        <div style={{ display: "flex", justifyContent: "flex-end", gap: 8 }}>
          <button
            onClick={onClose}
            style={{
              background: "none",
              border: "1px solid var(--color-border-default)",
              borderRadius: 6,
              padding: "8px 16px",
              fontSize: 13,
              color: "var(--color-text-secondary)",
              cursor: "pointer",
            }}
            onMouseEnter={(e) => { (e.currentTarget as HTMLButtonElement).style.background = "var(--color-bg-elevated)"; }}
            onMouseLeave={(e) => { (e.currentTarget as HTMLButtonElement).style.background = "none"; }}
          >
            Cancel
          </button>
          <button
            onClick={handleSubmit}
            disabled={isPending}
            style={{
              background: isPending ? "var(--color-bg-subtle)" : "var(--color-accent)",
              color: isPending ? "var(--color-text-muted)" : "var(--color-accent-fg)",
              border: "none",
              borderRadius: 6,
              padding: "8px 20px",
              fontSize: 13,
              fontWeight: 500,
              cursor: isPending ? "not-allowed" : "pointer",
            }}
            onMouseEnter={(e) => {
              if (!isPending) (e.currentTarget as HTMLButtonElement).style.background = "var(--color-accent-hover)";
            }}
            onMouseLeave={(e) => {
              if (!isPending) (e.currentTarget as HTMLButtonElement).style.background = "var(--color-accent)";
            }}
          >
            {isPending ? "Saving…" : editing ? "Save Changes" : "Add Client"}
          </button>
        </div>
    </Modal>
  );
}

// Row actions
