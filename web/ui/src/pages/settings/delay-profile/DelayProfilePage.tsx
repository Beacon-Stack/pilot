import { useEffect, useState, type CSSProperties } from "react";
import { toast } from "sonner";
import PageHeader from "@/components/PageHeader";
import { useDelayProfile, useSaveDelayProfile } from "@/api/delay";
import type { DelayProfile } from "@/types";

const card: CSSProperties = {
  background: "var(--color-bg-surface)",
  border: "1px solid var(--color-border-subtle)",
  borderRadius: 8,
  padding: 20,
  display: "flex",
  flexDirection: "column",
  gap: 18,
};

const labelStyle: CSSProperties = { display: "flex", flexDirection: "column", gap: 6, fontSize: 12, fontWeight: 500, color: "var(--color-text-secondary)" };
const rowLabel: CSSProperties = { display: "flex", alignItems: "center", gap: 8, fontSize: 13, color: "var(--color-text-primary)" };
const hint: CSSProperties = { fontSize: 11, fontWeight: 400, color: "var(--color-text-muted)" };
const input: CSSProperties = {
  width: 120,
  padding: "7px 10px",
  borderRadius: 6,
  border: "1px solid var(--color-border-default)",
  background: "var(--color-bg-elevated)",
  color: "var(--color-text-primary)",
  fontSize: 13,
};

/** Returns an error message, or null when the profile can be saved. */
export function validateDelayProfile(p: DelayProfile): string | null {
  if (!p.usenet_enabled && !p.torrent_enabled) return "Enable at least one protocol.";
  for (const m of [p.usenet_delay_minutes, p.torrent_delay_minutes]) {
    if (!Number.isInteger(m) || m < 0 || m > 10080) return "Delays must be whole minutes between 0 and 10080 (one week).";
  }
  return null;
}

export default function DelayProfilePage() {
  const { data, isLoading } = useDelayProfile();
  const save = useSaveDelayProfile();
  const [form, setForm] = useState<DelayProfile | null>(null);

  useEffect(() => {
    if (data) setForm(data);
  }, [data]);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!form) return;
    const problem = validateDelayProfile(form);
    if (problem) {
      toast.error(problem);
      return;
    }
    try {
      await save.mutateAsync(form);
      toast.success("Delay profile saved");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Could not save the delay profile");
    }
  }

  return (
    <div style={{ padding: 32, maxWidth: 720 }}>
      <PageHeader
        title="Delay Profile"
        description="Which download protocols to use, and how long to wait after a release appears before grabbing it automatically, so a better one has time to show up."
      />
      {(isLoading || !form) && <div className="skeleton" style={{ height: 260, borderRadius: 8 }} />}
      {form && (
        <form onSubmit={onSubmit} style={card}>
          <fieldset style={{ border: "none", margin: 0, padding: 0, display: "flex", flexDirection: "column", gap: 8 }}>
            <legend style={{ ...labelStyle, padding: 0, marginBottom: 6 }}>Protocols</legend>
            <label style={rowLabel}>
              <input id="dp-usenet" type="checkbox" checked={form.usenet_enabled} onChange={(e) => setForm({ ...form, usenet_enabled: e.target.checked })} />
              Usenet
            </label>
            <label style={rowLabel}>
              <input id="dp-torrent" type="checkbox" checked={form.torrent_enabled} onChange={(e) => setForm({ ...form, torrent_enabled: e.target.checked })} />
              Torrent
            </label>
          </fieldset>

          <label style={labelStyle}>
            Preferred protocol
            <span style={hint}>Between two releases of the same quality, pick this protocol.</span>
            <select
              id="dp-preferred"
              value={form.preferred_protocol}
              onChange={(e) => setForm({ ...form, preferred_protocol: e.target.value as DelayProfile["preferred_protocol"] })}
              style={{ ...input, width: 160 }}
            >
              <option value="usenet">Usenet</option>
              <option value="torrent">Torrent</option>
            </select>
          </label>

          <div style={{ display: "flex", flexWrap: "wrap", gap: 24 }}>
            <label style={labelStyle}>
              Usenet delay (minutes)
              <input
                id="dp-usenet-delay"
                type="number"
                min={0}
                max={10080}
                value={form.usenet_delay_minutes}
                onChange={(e) => setForm({ ...form, usenet_delay_minutes: Number(e.target.value) })}
                style={input}
              />
            </label>
            <label style={labelStyle}>
              Torrent delay (minutes)
              <input
                id="dp-torrent-delay"
                type="number"
                min={0}
                max={10080}
                value={form.torrent_delay_minutes}
                onChange={(e) => setForm({ ...form, torrent_delay_minutes: Number(e.target.value) })}
                style={input}
              />
            </label>
          </div>
          <p style={{ ...hint, margin: 0 }}>
            The wait starts when the release was published. Releases still waiting show on the Queue page. Searches you start yourself are never delayed.
          </p>

          <label style={rowLabel}>
            <input
              id="dp-bypass"
              type="checkbox"
              checked={form.bypass_if_highest_quality}
              onChange={(e) => setForm({ ...form, bypass_if_highest_quality: e.target.checked })}
            />
            Skip the wait when the release is already the best quality the series' profile allows
          </label>

          <div>
            <button
              type="submit"
              disabled={save.isPending}
              style={{ padding: "8px 16px", borderRadius: 6, border: "none", background: "var(--color-accent)", color: "var(--color-accent-fg)", fontSize: 13, fontWeight: 500, cursor: "pointer" }}
            >
              {save.isPending ? "Saving…" : "Save"}
            </button>
          </div>
        </form>
      )}
    </div>
  );
}
