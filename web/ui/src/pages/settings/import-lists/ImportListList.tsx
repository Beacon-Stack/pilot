import { useState } from "react";
import Modal from "@beacon-shared/Modal";
import TableScroll from "@beacon-shared/TableScroll";
import PageHeader from "@/components/PageHeader";
import {
  useImportLists,
  useCreateImportList,
  useUpdateImportList,
  useDeleteImportList,
  useTestImportList,
  useSyncAllImportLists,
} from "@/api/importlists";
import { useQualityProfiles } from "@/api/quality";
import { useLibraries } from "@/api/libraries";
import type { ImportListConfig, ImportListRequest } from "@/types";

// ── Constants ──────────────────────────────────────────────────────────────────

interface KindDef {
  value: string;
  label: string;
  group: string;
  desc: string;
  zeroConfig: boolean;
}

const KINDS: KindDef[] = [
  { value: "tmdb_popular_tv",  label: "Popular",     group: "TMDb",  desc: "Most popular TV series right now",                              zeroConfig: true  },
  { value: "tmdb_trending_tv", label: "Trending",    group: "TMDb",  desc: "Trending TV series today or this week",                        zeroConfig: false },
  { value: "trakt_popular_tv", label: "Popular",     group: "Trakt", desc: "Most popular TV series on Trakt",                              zeroConfig: true  },
  { value: "trakt_trending_tv",label: "Trending",    group: "Trakt", desc: "Most watched TV right now",                                    zeroConfig: true  },
  { value: "trakt_list_tv",    label: "User List",   group: "Trakt", desc: "A Trakt user's watchlist or custom list (TV)",                 zeroConfig: false },
  { value: "plex_watchlist_tv",label: "Watchlist",   group: "Plex",  desc: "Your Plex account watchlist (TV shows)",                      zeroConfig: false },
  { value: "custom_list",      label: "Custom JSON", group: "Other", desc: "Any URL returning JSON with TMDb TV IDs",                     zeroConfig: false },
];

const KIND_GROUPS = ["TMDb", "Trakt", "Plex", "Other"] as const;

const MONITOR_TYPE_OPTIONS = [
  { value: "all",          label: "All Episodes"    },
  { value: "future",       label: "Future Episodes" },
  { value: "missing",      label: "Missing Episodes"},
  { value: "existing",     label: "Existing Episodes"},
  { value: "pilot",        label: "Pilot Only"      },
  { value: "first_season", label: "First Season"    },
  { value: "last_season",  label: "Latest Season"   },
  { value: "none",         label: "None"            },
];

// ── Helpers ────────────────────────────────────────────────────────────────────

function strSetting(settings: Record<string, unknown>, key: string): string {
  const v = settings[key];
  return typeof v === "string" ? v : "";
}

function kindDef(value: string): KindDef | undefined {
  return KINDS.find((k) => k.value === value);
}

// ── Shared styles ──────────────────────────────────────────────────────────────

const inputStyle: React.CSSProperties = {
  width: "100%",
  background: "var(--color-bg-elevated)",
  border: "1px solid var(--color-border-default)",
  borderRadius: 6,
  padding: "8px 12px",
  fontSize: 13,
  color: "var(--color-text-primary)",
  outline: "none",
  boxSizing: "border-box",
};

const labelStyle: React.CSSProperties = {
  display: "block",
  fontSize: 12,
  fontWeight: 500,
  color: "var(--color-text-secondary)",
  marginBottom: 6,
};

const fieldStyle: React.CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: 0,
};

function actionBtn(color: string, bg: string): React.CSSProperties {
  return {
    background: bg,
    border: "1px solid var(--color-border-default)",
    borderRadius: 5,
    padding: "3px 10px",
    fontSize: 12,
    color,
    cursor: "pointer",
    whiteSpace: "nowrap",
  };
}

// ── Form state ─────────────────────────────────────────────────────────────────

interface FormState {
  name: string;
  kind: string;
  enabled: boolean;
  monitor: boolean;
  monitor_type: string;
  search_on_add: boolean;
  quality_profile_id: string;
  library_id: string;
  // tmdb_trending_tv
  window: string;
  // trakt_list_tv
  trakt_username: string;
  list_type: string;
  list_slug: string;
  // plex_watchlist_tv
  plex_token: string;
  // custom_list
  url: string;
}

function emptyForm(): FormState {
  return {
    name: "",
    kind: "tmdb_popular_tv",
    enabled: true,
    monitor: true,
    monitor_type: "all",
    search_on_add: true,
    quality_profile_id: "",
    library_id: "",
    window: "week",
    trakt_username: "",
    list_type: "watchlist",
    list_slug: "",
    plex_token: "",
    url: "",
  };
}

function configToForm(cfg: ImportListConfig): FormState {
  return {
    name: cfg.name,
    kind: cfg.kind,
    enabled: cfg.enabled,
    monitor: cfg.monitor,
    monitor_type: cfg.monitor_type,
    search_on_add: cfg.search_on_add,
    quality_profile_id: cfg.quality_profile_id,
    library_id: cfg.library_id,
    window: strSetting(cfg.settings, "window") || "week",
    trakt_username: strSetting(cfg.settings, "username"),
    list_type: strSetting(cfg.settings, "list_type") || "watchlist",
    list_slug: strSetting(cfg.settings, "list_slug"),
    plex_token: "",
    url: strSetting(cfg.settings, "url"),
  };
}

function formToRequest(f: FormState): ImportListRequest {
  const settings: Record<string, unknown> = {};

  switch (f.kind) {
    case "tmdb_trending_tv":
      settings.window = f.window;
      break;
    case "trakt_list_tv":
      settings.username = f.trakt_username.trim();
      settings.list_type = f.list_type;
      if (f.list_type === "custom") settings.list_slug = f.list_slug.trim();
      break;
    case "plex_watchlist_tv":
      if (f.plex_token.trim()) settings.token = f.plex_token.trim();
      break;
    case "custom_list":
      settings.url = f.url.trim();
      break;
    default:
      break;
  }

  return {
    name: f.name.trim(),
    kind: f.kind,
    enabled: f.enabled,
    monitor: f.monitor,
    monitor_type: f.monitor_type,
    search_on_add: f.search_on_add,
    quality_profile_id: f.quality_profile_id,
    library_id: f.library_id,
    settings,
  };
}

// ── Modal ──────────────────────────────────────────────────────────────────────

interface ImportListModalProps {
  editing: ImportListConfig | null;
  onClose: () => void;
}

function ImportListModal({ editing, onClose }: ImportListModalProps) {
  const [form, setForm] = useState<FormState>(
    editing ? configToForm(editing) : emptyForm()
  );
  const [error, setError] = useState<string | null>(null);

  const create = useCreateImportList();
  const update = useUpdateImportList();
  const isPending = create.isPending || update.isPending;

  const { data: qualityProfiles } = useQualityProfiles();
  const { data: libraries } = useLibraries();

  function set<K extends keyof FormState>(field: K, value: FormState[K]) {
    setForm((f) => ({ ...f, [field]: value }));
    setError(null);
  }

  function handleSubmit() {
    if (!form.name.trim()) { setError("Name is required."); return; }
    if (!form.quality_profile_id) { setError("Quality profile is required."); return; }
    if (!form.library_id) { setError("Library is required."); return; }

    if (form.kind === "trakt_list_tv" && !form.trakt_username.trim()) {
      setError("Trakt username is required."); return;
    }
    if (form.kind === "trakt_list_tv" && form.list_type === "custom" && !form.list_slug.trim()) {
      setError("List slug is required for custom lists."); return;
    }
    if (form.kind === "custom_list" && !form.url.trim()) {
      setError("URL is required."); return;
    }

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

  const currentKind = kindDef(form.kind);
  const showPluginSettings = currentKind && !currentKind.zeroConfig;

  return (
    <Modal onClose={onClose} width={560} maxHeight="calc(100vh - 64px)" innerStyle={{ padding: 0 }}>
      {/* Header */}
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "20px 24px 0" }}>
        <h2 style={{ margin: 0, fontSize: 16, fontWeight: 600, color: "var(--color-text-primary)" }}>
          {editing ? "Edit Import List" : "Add Import List"}
        </h2>
        <button
          onClick={onClose}
          style={{ background: "none", border: "none", cursor: "pointer", color: "var(--color-text-muted)", fontSize: 18, lineHeight: 1, padding: "4px 6px", borderRadius: 4 }}
          onMouseEnter={(e) => { (e.currentTarget as HTMLButtonElement).style.color = "var(--color-text-primary)"; }}
          onMouseLeave={(e) => { (e.currentTarget as HTMLButtonElement).style.color = "var(--color-text-muted)"; }}
        >
          &#x2715;
        </button>
      </div>

      {/* Scrollable body */}
      <div style={{ overflowY: "auto", flex: 1, padding: "20px 24px", display: "flex", flexDirection: "column", gap: 16 }}>

        {/* Name + Kind */}
        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 16 }}>
          <div style={fieldStyle}>
            <label style={labelStyle}>Name *</label>
            <input
              style={inputStyle}
              value={form.name}
              onChange={(e) => set("name", e.currentTarget.value)}
              onFocus={focusBorder}
              onBlur={blurBorder}
              placeholder="e.g. TMDb Popular"
              autoFocus
            />
          </div>
          <div style={fieldStyle}>
            <label style={labelStyle}>Source</label>
            <select
              style={{ ...inputStyle, cursor: "pointer" }}
              value={form.kind}
              onChange={(e) => set("kind", e.currentTarget.value)}
              onFocus={focusBorder}
              onBlur={blurBorder}
            >
              {KIND_GROUPS.map((group) => (
                <optgroup key={group} label={group}>
                  {KINDS.filter((k) => k.group === group).map((k) => (
                    <option key={k.value} value={k.value}>{k.label}</option>
                  ))}
                </optgroup>
              ))}
            </select>
          </div>
        </div>

        {/* Kind description hint */}
        {currentKind && (
          <p style={{ margin: 0, fontSize: 12, color: "var(--color-text-muted)", lineHeight: 1.5 }}>
            {currentKind.desc}
          </p>
        )}

        {/* Plugin-specific settings */}
        {showPluginSettings && (
          <div style={{ background: "var(--color-bg-elevated)", border: "1px solid var(--color-border-subtle)", borderRadius: 8, padding: 16, display: "flex", flexDirection: "column", gap: 14 }}>
            <p style={{ margin: 0, fontSize: 11, fontWeight: 600, letterSpacing: "0.06em", textTransform: "uppercase", color: "var(--color-text-muted)" }}>
              {currentKind.group} Settings
            </p>

            {form.kind === "tmdb_trending_tv" && (
              <div style={fieldStyle}>
                <label style={labelStyle}>Window</label>
                <select
                  style={{ ...inputStyle, cursor: "pointer" }}
                  value={form.window}
                  onChange={(e) => set("window", e.currentTarget.value)}
                  onFocus={focusBorder}
                  onBlur={blurBorder}
                >
                  <option value="day">Today</option>
                  <option value="week">This Week</option>
                </select>
              </div>
            )}

            {form.kind === "trakt_list_tv" && (
              <>
                <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 14 }}>
                  <div style={fieldStyle}>
                    <label style={labelStyle}>Username *</label>
                    <input
                      style={inputStyle}
                      value={form.trakt_username}
                      onChange={(e) => set("trakt_username", e.currentTarget.value)}
                      onFocus={focusBorder}
                      onBlur={blurBorder}
                      placeholder="trakt username"
                    />
                  </div>
                  <div style={fieldStyle}>
                    <label style={labelStyle}>List Type</label>
                    <select
                      style={{ ...inputStyle, cursor: "pointer" }}
                      value={form.list_type}
                      onChange={(e) => set("list_type", e.currentTarget.value)}
                      onFocus={focusBorder}
                      onBlur={blurBorder}
                    >
                      <option value="watchlist">Watchlist</option>
                      <option value="custom">Custom List</option>
                    </select>
                  </div>
                </div>
                {form.list_type === "custom" && (
                  <div style={fieldStyle}>
                    <label style={labelStyle}>List Slug *</label>
                    <input
                      style={inputStyle}
                      value={form.list_slug}
                      onChange={(e) => set("list_slug", e.currentTarget.value)}
                      onFocus={focusBorder}
                      onBlur={blurBorder}
                      placeholder="e.g. my-watchlist"
                    />
                  </div>
                )}
              </>
            )}

            {form.kind === "plex_watchlist_tv" && (
              <div style={fieldStyle}>
                <label style={labelStyle}>Access Token</label>
                <input
                  style={inputStyle}
                  type="password"
                  value={form.plex_token}
                  onChange={(e) => set("plex_token", e.currentTarget.value)}
                  onFocus={focusBorder}
                  onBlur={blurBorder}
                  placeholder={editing ? "leave blank to keep existing token" : "your Plex token"}
                  autoComplete="new-password"
                />
                {editing && (
                  <p style={{ margin: "4px 0 0", fontSize: 11, color: "var(--color-text-muted)" }}>
                    Tokens are masked in the UI. Enter a new value to update.
                  </p>
                )}
              </div>
            )}

            {form.kind === "custom_list" && (
              <div style={fieldStyle}>
                <label style={labelStyle}>URL *</label>
                <input
                  style={inputStyle}
                  value={form.url}
                  onChange={(e) => set("url", e.currentTarget.value)}
                  onFocus={focusBorder}
                  onBlur={blurBorder}
                  placeholder="https://example.com/my-list.json"
                />
              </div>
            )}
          </div>
        )}

        {/* Quality Profile + Library */}
        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 16 }}>
          <div style={fieldStyle}>
            <label style={labelStyle}>Quality Profile *</label>
            <select
              style={{ ...inputStyle, cursor: "pointer" }}
              value={form.quality_profile_id}
              onChange={(e) => set("quality_profile_id", e.currentTarget.value)}
              onFocus={focusBorder}
              onBlur={blurBorder}
            >
              <option value="">Select profile…</option>
              {qualityProfiles?.map((p) => (
                <option key={p.id} value={p.id}>{p.name}</option>
              ))}
            </select>
          </div>
          <div style={fieldStyle}>
            <label style={labelStyle}>Library *</label>
            <select
              style={{ ...inputStyle, cursor: "pointer" }}
              value={form.library_id}
              onChange={(e) => set("library_id", e.currentTarget.value)}
              onFocus={focusBorder}
              onBlur={blurBorder}
            >
              <option value="">Select library…</option>
              {libraries?.map((l) => (
                <option key={l.id} value={l.id}>{l.name}</option>
              ))}
            </select>
          </div>
        </div>

        {/* Monitor Type */}
        <div style={fieldStyle}>
          <label style={labelStyle}>Monitor</label>
          <select
            style={{ ...inputStyle, cursor: "pointer" }}
            value={form.monitor_type}
            onChange={(e) => set("monitor_type", e.currentTarget.value)}
            onFocus={focusBorder}
            onBlur={blurBorder}
          >
            {MONITOR_TYPE_OPTIONS.map((o) => (
              <option key={o.value} value={o.value}>{o.label}</option>
            ))}
          </select>
        </div>

        {/* Toggles */}
        <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 12 }}>
          {(
            [
              { field: "monitor",       label: "Monitor new series" },
              { field: "search_on_add", label: "Search on add"      },
              { field: "enabled",       label: "Enabled"            },
            ] as { field: keyof FormState; label: string }[]
          ).map(({ field, label }) => (
            <label key={field} style={{ display: "flex", alignItems: "center", gap: 8, cursor: "pointer", userSelect: "none" }}>
              <input
                type="checkbox"
                checked={form[field] as boolean}
                onChange={(e) => set(field, e.currentTarget.checked)}
                style={{ width: 16, height: 16, cursor: "pointer", accentColor: "var(--color-accent)" }}
              />
              <span style={{ fontSize: 13, color: "var(--color-text-primary)" }}>{label}</span>
            </label>
          ))}
        </div>

        {error && (
          <p style={{ margin: 0, fontSize: 12, color: "var(--color-danger)" }}>{error}</p>
        )}
      </div>

      {/* Footer */}
      <div style={{ display: "flex", justifyContent: "flex-end", gap: 8, padding: "16px 24px", borderTop: "1px solid var(--color-border-subtle)" }}>
        <button
          onClick={onClose}
          style={{ background: "none", border: "1px solid var(--color-border-default)", borderRadius: 6, padding: "8px 16px", fontSize: 13, color: "var(--color-text-secondary)", cursor: "pointer" }}
          onMouseEnter={(e) => { (e.currentTarget as HTMLButtonElement).style.background = "var(--color-bg-elevated)"; }}
          onMouseLeave={(e) => { (e.currentTarget as HTMLButtonElement).style.background = "none"; }}
        >
          Cancel
        </button>
        <button
          onClick={handleSubmit}
          disabled={isPending}
          style={{ background: isPending ? "var(--color-bg-subtle)" : "var(--color-accent)", color: isPending ? "var(--color-text-muted)" : "var(--color-accent-fg)", border: "none", borderRadius: 6, padding: "8px 20px", fontSize: 13, fontWeight: 500, cursor: isPending ? "not-allowed" : "pointer" }}
          onMouseEnter={(e) => { if (!isPending) (e.currentTarget as HTMLButtonElement).style.background = "var(--color-accent-hover)"; }}
          onMouseLeave={(e) => { if (!isPending) (e.currentTarget as HTMLButtonElement).style.background = "var(--color-accent)"; }}
        >
          {isPending ? "Saving…" : editing ? "Save Changes" : "Add Import List"}
        </button>
      </div>
    </Modal>
  );
}

// ── Row actions ────────────────────────────────────────────────────────────────

function RowActions({ cfg, onEdit }: { cfg: ImportListConfig; onEdit: () => void }) {
  const [confirming, setConfirming] = useState(false);
  const [testResult, setTestResult] = useState<{ ok: boolean; message?: string } | null>(null);
  const del = useDeleteImportList();
  const test = useTestImportList();

  function handleTest() {
    setTestResult(null);
    test.mutate(cfg.id, {
      onSuccess: () => { setTestResult({ ok: true }); setTimeout(() => setTestResult(null), 4000); },
      onError: (e) => { setTestResult({ ok: false, message: e.message }); setTimeout(() => setTestResult(null), 4000); },
    });
  }

  if (confirming) {
    return (
      <div style={{ display: "flex", alignItems: "center", gap: 6, justifyContent: "flex-end" }}>
        <span style={{ fontSize: 12, color: "var(--color-text-secondary)" }}>Delete?</span>
        <button
          onClick={() => del.mutate(cfg.id, { onSuccess: () => setConfirming(false) })}
          disabled={del.isPending}
          style={actionBtn("var(--color-danger)", "color-mix(in srgb, var(--color-danger) 15%, transparent)")}
        >
          {del.isPending ? "…" : "Yes"}
        </button>
        <button onClick={() => setConfirming(false)} style={actionBtn("var(--color-text-secondary)", "var(--color-bg-elevated)")}>No</button>
      </div>
    );
  }

  if (testResult !== null) {
    return (
      <div style={{ display: "flex", alignItems: "center", gap: 6, justifyContent: "flex-end" }}>
        <span
          style={{ fontSize: 12, color: testResult.ok ? "var(--color-success)" : "var(--color-danger)", maxWidth: 200, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}
          title={testResult.message}
        >
          {testResult.ok ? "Connected \u2713" : `Failed: ${testResult.message ?? "unknown error"}`}
        </span>
      </div>
    );
  }

  return (
    <div style={{ display: "flex", alignItems: "center", gap: 6, justifyContent: "flex-end" }}>
      <button onClick={handleTest} disabled={test.isPending} style={actionBtn("var(--color-text-secondary)", "var(--color-bg-elevated)")}>
        {test.isPending ? "Testing…" : "Test"}
      </button>
      <button onClick={onEdit} style={actionBtn("var(--color-text-secondary)", "var(--color-bg-elevated)")}>Edit</button>
      <button onClick={() => setConfirming(true)} style={actionBtn("var(--color-danger)", "color-mix(in srgb, var(--color-danger) 12%, transparent)")}>Delete</button>
    </div>
  );
}

// ── Kind badge ─────────────────────────────────────────────────────────────────

const GROUP_COLORS: Record<string, { bg: string; fg: string }> = {
  TMDb:  { bg: "color-mix(in srgb, var(--color-accent) 12%, transparent)",  fg: "var(--color-accent)"  },
  Trakt: { bg: "color-mix(in srgb, var(--color-success) 12%, transparent)", fg: "var(--color-success)" },
  Plex:  { bg: "color-mix(in srgb, #e5a00d 18%, transparent)",              fg: "#e5a00d"              },
  Other: { bg: "color-mix(in srgb, var(--color-text-muted) 14%, transparent)", fg: "var(--color-text-muted)" },
};

function SourceBadge({ kind }: { kind: string }) {
  const def = kindDef(kind);
  const group = def?.group ?? "Other";
  const colors = GROUP_COLORS[group] ?? GROUP_COLORS.Other;
  return (
    <span style={{ display: "inline-block", padding: "2px 8px", borderRadius: 4, fontSize: 11, fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.05em", background: colors.bg, color: colors.fg, whiteSpace: "nowrap" }}>
      {def ? `${def.group} ${def.label}` : kind}
    </span>
  );
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function ImportListList() {
  const { data, isLoading, error } = useImportLists();
  const [modal, setModal] = useState<{ open: boolean; editing: ImportListConfig | null }>({ open: false, editing: null });

  const syncAll = useSyncAllImportLists();

  return (
    <div style={{ padding: 24, maxWidth: 900 }}>
      <PageHeader
        title="Import Lists"
        description="External sources that automatically add new series to your library."
        action={
          <div style={{ display: "flex", gap: 8, flexShrink: 0 }}>
            <button
              onClick={() => syncAll.mutate()}
              disabled={syncAll.isPending}
              style={{ background: "none", border: "1px solid var(--color-border-default)", borderRadius: 6, padding: "8px 14px", fontSize: 13, color: syncAll.isPending ? "var(--color-text-muted)" : "var(--color-text-secondary)", cursor: syncAll.isPending ? "not-allowed" : "pointer" }}
              onMouseEnter={(e) => { if (!syncAll.isPending) (e.currentTarget as HTMLButtonElement).style.background = "var(--color-bg-elevated)"; }}
              onMouseLeave={(e) => { (e.currentTarget as HTMLButtonElement).style.background = "none"; }}
            >
              {syncAll.isPending ? "Syncing…" : "Sync All"}
            </button>
            <button
              onClick={() => setModal({ open: true, editing: null })}
              style={{ background: "var(--color-accent)", color: "var(--color-accent-fg)", border: "none", borderRadius: 6, padding: "8px 16px", fontSize: 13, fontWeight: 500, cursor: "pointer" }}
              onMouseEnter={(e) => { (e.currentTarget as HTMLButtonElement).style.background = "var(--color-accent-hover)"; }}
              onMouseLeave={(e) => { (e.currentTarget as HTMLButtonElement).style.background = "var(--color-accent)"; }}
            >
              + Add Import List
            </button>
          </div>
        }
      />

      <div style={{ background: "var(--color-bg-surface)", border: "1px solid var(--color-border-subtle)", borderRadius: 8, boxShadow: "var(--shadow-card)", overflow: "hidden" }}>
        {isLoading ? (
          <div style={{ padding: 20, display: "flex", flexDirection: "column", gap: 12 }}>
            {[1, 2, 3].map((i) => (
              <div key={i} className="skeleton" style={{ height: 44, borderRadius: 4 }} />
            ))}
          </div>
        ) : error ? (
          <div style={{ padding: 24, fontSize: 13, color: "var(--color-text-muted)" }}>
            Failed to load import lists.
          </div>
        ) : !data?.length ? (
          <div style={{ padding: 48, textAlign: "center" }}>
            <p style={{ margin: 0, fontSize: 14, color: "var(--color-text-secondary)", fontWeight: 500 }}>
              No import lists configured
            </p>
            <p style={{ margin: "6px 0 0", fontSize: 13, color: "var(--color-text-muted)" }}>
              Add a source like TMDb Popular or a Trakt list to auto-populate your library.
            </p>
          </div>
        ) : (
          <TableScroll minWidth={700}>
            <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}>
              <thead>
                <tr style={{ borderBottom: "1px solid var(--color-border-subtle)" }}>
                  {["Name", "Source", "Monitor", "Status", ""].map((h) => (
                  <th key={h} style={{ textAlign: "left", padding: "10px 16px", fontSize: 11, fontWeight: 600, letterSpacing: "0.08em", textTransform: "uppercase", color: "var(--color-text-muted)", whiteSpace: "nowrap" }}>
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {data.map((cfg, i) => {
                const monitorLabel = MONITOR_TYPE_OPTIONS.find((o) => o.value === cfg.monitor_type)?.label ?? cfg.monitor_type;
                return (
                  <tr key={cfg.id} style={{ borderBottom: i < data.length - 1 ? "1px solid var(--color-border-subtle)" : "none" }}>
                    <td style={{ padding: "0 16px", height: 52, color: "var(--color-text-primary)", fontWeight: 500 }}>
                      {cfg.name}
                    </td>
                    <td style={{ padding: "0 16px", height: 52 }}>
                      <SourceBadge kind={cfg.kind} />
                    </td>
                    <td style={{ padding: "0 16px", height: 52, color: "var(--color-text-secondary)", whiteSpace: "nowrap" }}>
                      {monitorLabel}
                    </td>
                    <td style={{ padding: "0 16px", height: 52 }}>
                      <span style={{ display: "inline-flex", alignItems: "center", gap: 6, fontSize: 12, color: cfg.enabled ? "var(--color-success)" : "var(--color-text-muted)" }}>
                        <span style={{ width: 6, height: 6, borderRadius: "50%", background: cfg.enabled ? "var(--color-success)" : "var(--color-text-muted)", flexShrink: 0 }} />
                        {cfg.enabled ? "Enabled" : "Disabled"}
                      </span>
                    </td>
                    <td style={{ padding: "0 16px", height: 52, width: 1 }}>
                      <RowActions cfg={cfg} onEdit={() => setModal({ open: true, editing: cfg })} />
                    </td>
                  </tr>
                );
              })}
            </tbody>
            </table>
          </TableScroll>
        )}
      </div>

      {modal.open && (
        <ImportListModal
          editing={modal.editing}
          onClose={() => setModal({ open: false, editing: null })}
        />
      )}
    </div>
  );
}
