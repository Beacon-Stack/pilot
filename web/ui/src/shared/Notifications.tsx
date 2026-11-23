import { useCallback, useEffect, useMemo, useState } from "react";
import type { CSSProperties, ReactNode } from "react";
import { Bell, CheckCircle2, Clock, Plus, Send, XCircle } from "lucide-react";
import { apiFetch } from "./api";
import Modal from "./Modal";
import PageHeader from "./PageHeader";
import { useConfirm } from "./ConfirmDialog";

// The Notifications settings page, the same in Pulse, Pilot, Prism and
// Haul. It talks to /api/v1/notifications/* (pulse/pkg/notify/notifyapi).

interface Field {
  key: string;
  label: string;
  type: "text" | "password" | "url" | "number" | "checkbox" | "list";
  required?: boolean;
  placeholder?: string;
  help?: string;
}
interface Kind {
  id: string;
  name: string;
  help?: string;
  fields: Field[];
}
interface EventType {
  id: string;
  label: string;
  description: string;
  apps: string[];
  default: boolean;
}
interface Catalog {
  app: string;
  hub: boolean;
  kinds: Kind[];
  types: EventType[];
  apps?: { id: string; name: string }[];
}
interface Route {
  mode: "auto" | "local";
  via_pulse: boolean;
  pulse_connected: boolean;
  pulse_channels: number;
}
interface Status {
  app: string;
  hub: boolean;
  route: Route;
  channels: number;
  enabled: number;
  waiting: number;
}
interface Channel {
  id: string;
  name: string;
  kind: string;
  enabled: boolean;
  settings: Record<string, unknown>;
  events: string[];
  apps: string[];
  retired?: boolean;
}
interface Sent {
  time: string;
  app: string;
  title: string;
  level: string;
  via: string;
  results: { channel_name: string; error?: string }[];
  error?: string;
  pending?: boolean;
}

const MASKED = "***";

const box: CSSProperties = {
  background: "var(--color-bg-surface)",
  border: "1px solid var(--color-border-subtle)",
  borderRadius: 8,
  boxShadow: "var(--shadow-card)",
};
const btn: CSSProperties = {
  display: "inline-flex",
  alignItems: "center",
  gap: 6,
  padding: "6px 12px",
  borderRadius: 6,
  border: "1px solid var(--color-border-default)",
  background: "var(--color-bg-elevated)",
  color: "var(--color-text-primary)",
  fontSize: 12,
  cursor: "pointer",
};
const primary: CSSProperties = { ...btn, background: "var(--color-accent)", borderColor: "var(--color-accent)", color: "var(--color-accent-fg, #fff)" };
const input: CSSProperties = {
  width: "100%",
  padding: "8px 10px",
  borderRadius: 6,
  border: "1px solid var(--color-border-default)",
  background: "var(--color-bg-elevated)",
  color: "var(--color-text-primary)",
  fontSize: 13,
};
const muted: CSSProperties = { fontSize: 12, color: "var(--color-text-muted)" };

function errText(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}

function Banner({ tone, children }: { tone: "info" | "warn"; children: ReactNode }) {
  const color = tone === "warn" ? "var(--color-warning)" : "var(--color-accent)";
  return (
    <div
      role="status"
      style={{
        padding: "12px 16px",
        borderRadius: 8,
        border: `1px solid ${color}`,
        background: `color-mix(in srgb, ${color} 8%, transparent)`,
        fontSize: 13,
        color: "var(--color-text-primary)",
        lineHeight: 1.5,
      }}
    >
      {children}
    </div>
  );
}

function RouteBanner({ status, onMode, busy }: { status: Status; onMode: (m: "auto" | "local") => void; busy: boolean }) {
  const { app, route } = status;
  const link: CSSProperties = { background: "none", border: "none", padding: 0, color: "var(--color-accent)", cursor: "pointer", fontSize: 13 };
  if (route.via_pulse) {
    return (
      <Banner tone="info">
        <strong>Notifications go through Pulse</strong> ({route.pulse_channels} {route.pulse_channels === 1 ? "channel" : "channels"}). Manage them in Pulse;
        nothing to set up here. {status.waiting > 0 && <>{status.waiting} waiting for Pulse to answer. </>}
        <button type="button" disabled={busy} onClick={() => onMode("local")} style={link}>
          Send from {app} instead
        </button>
      </Banner>
    );
  }
  if (route.pulse_connected && route.mode === "local") {
    return (
      <Banner tone="info">
        {app} sends from its own channels below.{" "}
        <button type="button" disabled={busy} onClick={() => onMode("auto")} style={link}>
          Send through Pulse instead
        </button>
      </Banner>
    );
  }
  if (route.pulse_connected) {
    return (
      <Banner tone="info">
        Connected to Pulse, which has no channels yet. Add one in Pulse to set up notifications once for every app; until then {app} uses
        its own channels below.
      </Banner>
    );
  }
  return null;
}

function eventsSummary(ch: Channel, types: EventType[]): string {
  if (ch.events.length === 0) return "Every event";
  const labels = ch.events.map((e) => types.find((t) => t.id === e)?.label ?? e);
  return labels.length <= 3 ? labels.join(", ") : `${labels.slice(0, 3).join(", ")} +${labels.length - 3}`;
}

function ChannelRow({
  ch,
  catalog,
  onEdit,
  onDelete,
  onToggle,
  last,
}: {
  ch: Channel;
  catalog: Catalog;
  onEdit: () => void;
  onDelete: () => void;
  onToggle: (on: boolean) => void;
  last: boolean;
}) {
  const [test, setTest] = useState<{ busy?: boolean; ok?: boolean; error?: string }>({});
  const kind = catalog.kinds.find((k) => k.id === ch.kind);
  async function runTest() {
    setTest({ busy: true });
    try {
      const r = await apiFetch<{ ok: boolean; error?: string }>("/notifications/test", {
        method: "POST",
        body: JSON.stringify({ id: ch.id, kind: ch.kind, settings: ch.settings }),
      });
      setTest({ ok: r.ok, error: r.error });
    } catch (e) {
      setTest({ ok: false, error: errText(e) });
    }
  }
  const apps = catalog.hub && ch.apps.length > 0 ? ch.apps.map((a) => catalog.apps?.find((x) => x.id === a)?.name ?? a).join(", ") : null;
  return (
    <div style={{ padding: "12px 16px", borderBottom: last ? "none" : "1px solid var(--color-border-subtle)" }}>
      <div style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
        <label style={{ display: "flex", alignItems: "center" }} title={ch.enabled ? "On" : "Off"}>
          <input type="checkbox" aria-label={`${ch.name} on`} checked={ch.enabled} disabled={ch.retired} onChange={(e) => onToggle(e.target.checked)} />
        </label>
        <div style={{ flex: 1, minWidth: 160 }}>
          <div style={{ fontSize: 14, fontWeight: 500, color: ch.enabled ? "var(--color-text-primary)" : "var(--color-text-muted)" }}>{ch.name}</div>
          <div style={muted}>
            {ch.retired ? "Custom scripts are no longer supported. Remove this channel." : `${kind?.name ?? ch.kind} · ${eventsSummary(ch, catalog.types)}`}
            {apps && ` · ${apps} only`}
          </div>
        </div>
        {!ch.retired && (
          <>
            <button type="button" style={btn} onClick={() => void runTest()} disabled={test.busy}>
              <Send size={12} /> {test.busy ? "Sending…" : "Send test"}
            </button>
            <button type="button" style={btn} onClick={onEdit}>
              Edit
            </button>
          </>
        )}
        <button type="button" style={btn} onClick={onDelete} aria-label={`Remove ${ch.name}`}>
          Remove
        </button>
      </div>
      {test.ok === true && <div style={{ ...muted, color: "var(--color-success)", marginTop: 6 }}>Sent. Check {kind?.name ?? "the channel"}.</div>}
      {test.ok === false && (
        <div role="alert" style={{ fontSize: 12, color: "var(--color-danger)", marginTop: 6 }}>
          Couldn't send: {test.error}
        </div>
      )}
    </div>
  );
}

function ChannelForm({ catalog, editing, onClose, onSaved }: { catalog: Catalog; editing: Channel | null; onClose: () => void; onSaved: () => void }) {
  const isNew = editing === null;
  const [kindId, setKindId] = useState(editing?.kind ?? "");
  const kind = catalog.kinds.find((k) => k.id === kindId);
  const myTypes = catalog.hub ? catalog.types : catalog.types.filter((t) => t.apps.includes(catalog.app.toLowerCase()));
  const [name, setName] = useState(editing?.name ?? "");
  const [enabled, setEnabled] = useState(editing?.enabled ?? true);
  const [settings, setSettings] = useState<Record<string, unknown>>(editing?.settings ?? {});
  const [events, setEvents] = useState<string[]>(
    editing ? (editing.events.length ? editing.events : myTypes.map((t) => t.id)) : myTypes.filter((t) => t.default).map((t) => t.id),
  );
  const [apps, setApps] = useState<string[]>(editing?.apps ?? []);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [test, setTest] = useState<{ busy?: boolean; ok?: boolean; error?: string }>({});

  function pickKind(k: Kind) {
    setKindId(k.id);
    if (!name) setName(k.name);
    setSettings({});
  }

  function setField(f: Field, raw: string | boolean) {
    let v: unknown = raw;
    if (f.type === "number") v = raw === "" ? undefined : Number(raw);
    if (f.type === "list") v = String(raw).split(",").map((s) => s.trim()).filter(Boolean);
    setSettings((s) => ({ ...s, [f.key]: v }));
  }

  function fieldValue(f: Field): string {
    const v = settings[f.key];
    if (v === undefined || v === null) return "";
    if (Array.isArray(v)) return v.join(", ");
    return String(v);
  }

  // Every event ticked means "every event", stored as an empty list.
  const payloadEvents = events.length === myTypes.length && catalog.hub ? [] : events;

  async function save() {
    setSaving(true);
    setError(null);
    try {
      const body = JSON.stringify({ name, kind: kindId, enabled, settings, events: payloadEvents, apps });
      if (isNew) await apiFetch("/notifications/channels", { method: "POST", body });
      else await apiFetch(`/notifications/channels/${editing.id}`, { method: "PUT", body });
      onSaved();
    } catch (e) {
      setError(errText(e));
    } finally {
      setSaving(false);
    }
  }

  async function sendTest() {
    setTest({ busy: true });
    try {
      const r = await apiFetch<{ ok: boolean; error?: string }>("/notifications/test", {
        method: "POST",
        body: JSON.stringify({ id: editing?.id, kind: kindId, settings }),
      });
      setTest({ ok: r.ok, error: r.error });
    } catch (e) {
      setTest({ ok: false, error: errText(e) });
    }
  }

  const label: CSSProperties = { display: "block", fontSize: 12, fontWeight: 500, color: "var(--color-text-secondary)", marginBottom: 4 };

  return (
    <Modal onClose={onClose} width={520}>
      <div style={{ padding: 24, display: "flex", flexDirection: "column", gap: 16 }}>
        <h3 style={{ margin: 0, fontSize: 16, fontWeight: 600, color: "var(--color-text-primary)" }}>{isNew ? "Add a channel" : `Edit ${editing.name}`}</h3>

        {!kind ? (
          <>
            <div style={muted}>Where should notifications go?</div>
            <div role="group" aria-label="Channel type" style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(140px, 1fr))", gap: 8 }}>
              {catalog.kinds.map((k) => (
                <button key={k.id} type="button" style={{ ...btn, justifyContent: "center", padding: "12px 8px", fontSize: 13 }} onClick={() => pickKind(k)}>
                  {k.name}
                </button>
              ))}
            </div>
          </>
        ) : (
          <>
            {kind.help && <div style={{ ...muted, lineHeight: 1.5 }}>{kind.help}</div>}
            <div>
              <label style={label} htmlFor="nf-name">
                Name
              </label>
              <input id="nf-name" style={input} value={name} onChange={(e) => setName(e.target.value)} />
            </div>
            {kind.fields.map((f) =>
              f.type === "checkbox" ? (
                <label key={f.key} style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 13, color: "var(--color-text-primary)" }}>
                  <input type="checkbox" checked={settings[f.key] === true} onChange={(e) => setField(f, e.target.checked)} />
                  {f.label}
                </label>
              ) : (
                <div key={f.key}>
                  <label style={label} htmlFor={`nf-${f.key}`}>
                    {f.label}
                    {!f.required && <span style={{ fontWeight: 400, color: "var(--color-text-muted)" }}> (optional)</span>}
                  </label>
                  <input
                    id={`nf-${f.key}`}
                    style={input}
                    type={f.type === "password" ? "password" : f.type === "number" ? "number" : "text"}
                    placeholder={settings[f.key] === MASKED ? "Saved; type to replace" : f.placeholder}
                    value={settings[f.key] === MASKED ? "" : fieldValue(f)}
                    onChange={(e) => setField(f, e.target.value)}
                    autoComplete="off"
                  />
                  {f.help && <div style={{ ...muted, marginTop: 4 }}>{f.help}</div>}
                </div>
              ),
            )}

            <fieldset style={{ border: "none", padding: 0, margin: 0 }}>
              <legend style={label}>Send me</legend>
              <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
                {myTypes.map((t) => (
                  <label key={t.id} style={{ display: "flex", gap: 8, fontSize: 13, color: "var(--color-text-primary)", alignItems: "flex-start" }}>
                    <input
                      type="checkbox"
                      checked={events.includes(t.id)}
                      onChange={(e) => setEvents((ev) => (e.target.checked ? [...ev, t.id] : ev.filter((x) => x !== t.id)))}
                      style={{ marginTop: 2 }}
                    />
                    <span>
                      {t.label}
                      <span style={{ ...muted, display: "block" }}>{t.description}</span>
                    </span>
                  </label>
                ))}
              </div>
            </fieldset>

            {catalog.hub && catalog.apps && (
              <fieldset style={{ border: "none", padding: 0, margin: 0 }}>
                <legend style={label}>From</legend>
                <div style={{ display: "flex", gap: 12, flexWrap: "wrap", fontSize: 13 }}>
                  <label style={{ display: "flex", gap: 6, color: "var(--color-text-primary)" }}>
                    <input type="checkbox" checked={apps.length === 0} onChange={() => setApps([])} />
                    Every app
                  </label>
                  {catalog.apps.map((a) => (
                    <label key={a.id} style={{ display: "flex", gap: 6, color: "var(--color-text-primary)" }}>
                      <input
                        type="checkbox"
                        checked={apps.includes(a.id)}
                        onChange={(e) => setApps((x) => (e.target.checked ? [...x, a.id] : x.filter((y) => y !== a.id)))}
                      />
                      {a.name}
                    </label>
                  ))}
                </div>
              </fieldset>
            )}

            <label style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 13, color: "var(--color-text-primary)" }}>
              <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
              On
            </label>

            {test.ok === true && <div style={{ fontSize: 12, color: "var(--color-success)" }}>Test sent. Check {kind.name}.</div>}
            {test.ok === false && (
              <div role="alert" style={{ fontSize: 12, color: "var(--color-danger)" }}>
                Couldn't send: {test.error}
              </div>
            )}
            {error && (
              <div role="alert" style={{ fontSize: 13, color: "var(--color-danger)" }}>
                {error}
              </div>
            )}
            {events.length === 0 && <div style={{ fontSize: 12, color: "var(--color-warning)" }}>Pick at least one event, or this channel will get nothing.</div>}

            <div style={{ display: "flex", gap: 8, justifyContent: "space-between", flexWrap: "wrap" }}>
              <div style={{ display: "flex", gap: 8 }}>
                {isNew && (
                  <button type="button" style={btn} onClick={() => setKindId("")}>
                    Back
                  </button>
                )}
                <button type="button" style={btn} onClick={() => void sendTest()} disabled={test.busy}>
                  <Send size={12} /> {test.busy ? "Sending…" : "Send test"}
                </button>
              </div>
              <div style={{ display: "flex", gap: 8 }}>
                <button type="button" style={btn} onClick={onClose}>
                  Cancel
                </button>
                <button type="button" style={primary} onClick={() => void save()} disabled={saving || !name.trim() || events.length === 0}>
                  {saving ? "Saving…" : "Save"}
                </button>
              </div>
            </div>
          </>
        )}
      </div>
    </Modal>
  );
}

function RecentList({ items }: { items: Sent[] }) {
  if (items.length === 0) return <div style={{ padding: 16, ...muted }}>Nothing sent yet.</div>;
  return (
    <>
      {items.slice(0, 20).map((s, i) => {
        const failed = s.error || s.results?.some((r) => r.error);
        const Icon = s.pending ? Clock : failed ? XCircle : CheckCircle2;
        const color = s.pending ? "var(--color-text-muted)" : failed ? "var(--color-danger)" : "var(--color-success)";
        const detail = s.pending
          ? "Waiting for Pulse"
          : s.error ??
            (s.results?.length
              ? s.results.map((r) => (r.error ? `${r.channel_name}: ${r.error}` : r.channel_name)).join(" · ")
              : `via ${s.via}`);
        return (
          <div key={i} style={{ display: "flex", gap: 10, padding: "8px 16px", borderTop: i ? "1px solid var(--color-border-subtle)" : "none", fontSize: 13 }}>
            <Icon size={15} color={color} style={{ flexShrink: 0, marginTop: 2 }} aria-label={s.pending ? "waiting" : failed ? "failed" : "sent"} />
            <div style={{ minWidth: 0, flex: 1 }}>
              <div style={{ color: "var(--color-text-primary)", overflowWrap: "anywhere" }}>
                {s.app && <span style={muted}>{s.app} · </span>}
                {s.title}
              </div>
              <div style={{ ...muted, overflowWrap: "anywhere" }}>{detail}</div>
            </div>
            <time style={{ ...muted, flexShrink: 0 }} dateTime={s.time}>
              {new Date(s.time).toLocaleTimeString([], { hour: "numeric", minute: "2-digit" })}
            </time>
          </div>
        );
      })}
    </>
  );
}

export default function Notifications() {
  const [catalog, setCatalog] = useState<Catalog | null>(null);
  const [status, setStatus] = useState<Status | null>(null);
  const [channels, setChannels] = useState<Channel[] | null>(null);
  const [recent, setRecent] = useState<Sent[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState<Channel | null | undefined>(undefined); // undefined: closed; null: new
  const [busy, setBusy] = useState(false);
  const confirm = useConfirm();

  const load = useCallback(async () => {
    try {
      const [c, s, ch, r] = await Promise.all([
        apiFetch<Catalog>("/notifications/catalog"),
        apiFetch<Status>("/notifications/status"),
        apiFetch<Channel[]>("/notifications/channels"),
        apiFetch<Sent[]>("/notifications/recent"),
      ]);
      setCatalog(c);
      setStatus(s);
      setChannels(ch ?? []);
      setRecent(r ?? []);
      setError(null);
    } catch (e) {
      setError(errText(e));
    }
  }, []);

  useEffect(() => {
    void load();
    const t = setInterval(() => {
      apiFetch<Sent[]>("/notifications/recent")
        .then((r) => setRecent(r ?? []))
        .catch(() => {});
    }, 15_000);
    return () => clearInterval(t);
  }, [load]);

  async function setMode(mode: "auto" | "local") {
    setBusy(true);
    try {
      await apiFetch("/notifications/mode", { method: "PUT", body: JSON.stringify({ mode }) });
      await load();
    } catch (e) {
      setError(errText(e));
    } finally {
      setBusy(false);
    }
  }

  async function toggle(ch: Channel, on: boolean) {
    try {
      await apiFetch(`/notifications/channels/${ch.id}`, { method: "PUT", body: JSON.stringify({ ...ch, enabled: on }) });
      await load();
    } catch (e) {
      setError(errText(e));
    }
  }

  async function remove(ch: Channel) {
    const ok = await confirm({ title: `Remove ${ch.name}?`, message: "It will stop getting notifications.", confirmLabel: "Remove", danger: true });
    if (!ok) return;
    try {
      await apiFetch(`/notifications/channels/${ch.id}`, { method: "DELETE" });
      await load();
    } catch (e) {
      setError(errText(e));
    }
  }

  const app = catalog?.app ?? status?.app ?? "This app";
  const isHub = catalog?.hub ?? false;
  const ownTitle = useMemo(() => {
    if (isHub) return "Channels";
    if (status?.route.via_pulse) return `${app}'s own channels`;
    return "Channels";
  }, [isHub, status, app]);

  return (
    <div style={{ padding: 24, maxWidth: 900 }}>
      <PageHeader
        title="Notifications"
        description={
          isHub
            ? "Set up Discord, ntfy, email and more once; Pilot, Prism and Haul send through here."
            : `Get a message when something happens in ${app}.`
        }
      />
      <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
        {error && (
          <div role="alert" style={{ fontSize: 13, color: "var(--color-danger)" }}>
            {error}
          </div>
        )}
        {status && !isHub && <RouteBanner status={status} onMode={(m) => void setMode(m)} busy={busy} />}

        <section aria-label={ownTitle} style={box}>
          <header style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12, padding: "12px 16px", borderBottom: "1px solid var(--color-border-subtle)" }}>
            <div>
              <h2 style={{ margin: 0, fontSize: 14, fontWeight: 600, color: "var(--color-text-primary)" }}>{ownTitle}</h2>
              {status?.route.via_pulse && <div style={muted}>Used only if Pulse can't be reached.</div>}
            </div>
            <button type="button" style={btn} onClick={() => setEditing(null)} disabled={!catalog}>
              <Plus size={13} /> Add channel
            </button>
          </header>
          {!channels && !error && <div className="skeleton" style={{ height: 40, margin: 16, borderRadius: 4 }} />}
          {channels && channels.length === 0 && (
            <div style={{ padding: "28px 16px", textAlign: "center" }}>
              <Bell size={22} color="var(--color-text-muted)" />
              <div style={{ fontSize: 14, color: "var(--color-text-secondary)", marginTop: 8 }}>No channels yet</div>
              <div style={{ ...muted, marginTop: 4 }}>Add Discord, ntfy, Telegram, email and more.</div>
            </div>
          )}
          {catalog &&
            channels?.map((ch, i) => (
              <ChannelRow
                key={ch.id}
                ch={ch}
                catalog={catalog}
                last={i === channels.length - 1}
                onEdit={() => setEditing(ch)}
                onDelete={() => void remove(ch)}
                onToggle={(on) => void toggle(ch, on)}
              />
            ))}
        </section>

        <section aria-label="Recently sent" style={box}>
          <header style={{ padding: "12px 16px", borderBottom: "1px solid var(--color-border-subtle)" }}>
            <h2 style={{ margin: 0, fontSize: 14, fontWeight: 600, color: "var(--color-text-primary)" }}>Recently sent</h2>
          </header>
          <RecentList items={recent} />
        </section>
      </div>

      {editing !== undefined && catalog && (
        <ChannelForm
          catalog={catalog}
          editing={editing}
          onClose={() => setEditing(undefined)}
          onSaved={() => {
            setEditing(undefined);
            void load();
          }}
        />
      )}
    </div>
  );
}
