import { useMemo, useState, type CSSProperties } from "react";
import { toast } from "sonner";
import Modal from "@beacon-shared/Modal";
import { useCreateQualityProfile, useUpdateQualityProfile } from "@/api/quality";
import type { Quality, QualityProfile } from "@/types";

// ── Quality catalog ───────────────────────────────────────────────────────────

export interface CatalogQuality {
  name: string;
  resolution: string;
  source: string;
}

/**
 * Sonarr's qualities as Pilot stores them (resolution + source), highest
 * first. Mirrors mapSonarrQuality in internal/sonarrimport/service.go so
 * imported and hand-made profiles mean the same thing. SD qualities use
 * resolution "sd", which also matches 480p and 576p releases.
 */
export const QUALITY_CATALOG: CatalogQuality[] = [
  { name: "Bluray-2160p Remux", resolution: "2160p", source: "remux" },
  { name: "Bluray-2160p", resolution: "2160p", source: "bluray" },
  { name: "WEBDL-2160p", resolution: "2160p", source: "webdl" },
  { name: "WEBRip-2160p", resolution: "2160p", source: "webrip" },
  { name: "HDTV-2160p", resolution: "2160p", source: "hdtv" },
  { name: "Bluray-1080p Remux", resolution: "1080p", source: "remux" },
  { name: "Bluray-1080p", resolution: "1080p", source: "bluray" },
  { name: "WEBDL-1080p", resolution: "1080p", source: "webdl" },
  { name: "WEBRip-1080p", resolution: "1080p", source: "webrip" },
  { name: "HDTV-1080p", resolution: "1080p", source: "hdtv" },
  { name: "Bluray-720p Remux", resolution: "720p", source: "remux" },
  { name: "Bluray-720p", resolution: "720p", source: "bluray" },
  { name: "WEBDL-720p", resolution: "720p", source: "webdl" },
  { name: "WEBRip-720p", resolution: "720p", source: "webrip" },
  { name: "HDTV-720p", resolution: "720p", source: "hdtv" },
  { name: "Bluray-480p", resolution: "sd", source: "bluray" },
  { name: "WEBDL-480p", resolution: "sd", source: "webdl" },
  { name: "WEBRip-480p", resolution: "sd", source: "webrip" },
  { name: "DVD", resolution: "sd", source: "dvd" },
  { name: "SDTV", resolution: "sd", source: "hdtv" },
];

const SD = new Set(["sd", "480p", "576p"]);

/** Key a quality by resolution + source, with the SD resolutions as one. */
export function qualityKey(q: { resolution: string; source: string }): string {
  const res = SD.has(q.resolution) ? "sd" : q.resolution;
  return `${res}|${q.source}`;
}

const catalogByKey = new Map(QUALITY_CATALOG.map((c) => [qualityKey(c), c]));

function toQuality(c: CatalogQuality): Quality {
  return { name: c.name, resolution: c.resolution, source: c.source, codec: "", hdr: "" };
}

// ── Form state ────────────────────────────────────────────────────────────────

export interface ProfileForm {
  name: string;
  checked: Set<string>; // catalog keys
  extras: Quality[]; // qualities the catalog doesn't know; kept as-is
  cutoff: string; // catalog key
  upgradeAllowed: boolean;
  upgradeUntil: string; // catalog key, "" = no limit
}

export function formFromProfile(p: QualityProfile | null): ProfileForm {
  if (!p) {
    const checked = new Set(["1080p|webdl", "1080p|webrip", "1080p|bluray"]);
    return { name: "", checked, extras: [], cutoff: "1080p|webdl", upgradeAllowed: true, upgradeUntil: "" };
  }
  const checked = new Set<string>();
  const extras: Quality[] = [];
  for (const q of p.qualities) {
    const k = qualityKey(q);
    if (catalogByKey.has(k)) checked.add(k);
    else extras.push(q);
  }
  return {
    name: p.name,
    checked,
    extras,
    cutoff: qualityKey(p.cutoff),
    upgradeAllowed: p.upgrade_allowed,
    upgradeUntil: p.upgrade_until ? qualityKey(p.upgrade_until) : "",
  };
}

/** Returns an error message, or null when the form can be saved. */
export function validateForm(f: ProfileForm): string | null {
  if (f.name.trim() === "") return "Give the profile a name.";
  if (f.checked.size === 0 && f.extras.length === 0) return "Allow at least one quality.";
  if (!f.checked.has(f.cutoff)) return "Choose a cutoff from the allowed qualities.";
  if (f.upgradeUntil !== "" && !f.checked.has(f.upgradeUntil)) return "Choose an upgrade limit from the allowed qualities.";
  return null;
}

/** Request body for POST/PUT /quality-profiles. Allowed qualities are sent highest first. */
export function buildProfileRequest(f: ProfileForm, existing: QualityProfile | null): Omit<QualityProfile, "id"> {
  const qualities = QUALITY_CATALOG.filter((c) => f.checked.has(qualityKey(c))).map(toQuality);
  const cutoff = catalogByKey.get(f.cutoff);
  const until = f.upgradeUntil ? catalogByKey.get(f.upgradeUntil) : undefined;
  return {
    name: f.name.trim(),
    qualities: [...qualities, ...f.extras],
    cutoff: cutoff ? toQuality(cutoff) : qualities[0],
    upgrade_allowed: f.upgradeAllowed,
    upgrade_until: until ? toQuality(until) : undefined,
    min_custom_format_score: existing?.min_custom_format_score ?? 0,
    upgrade_until_cf_score: existing?.upgrade_until_cf_score ?? 0,
  };
}

// ── Component ─────────────────────────────────────────────────────────────────

const labelStyle: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: 6,
  fontSize: 12,
  fontWeight: 500,
  color: "var(--color-text-secondary)",
};

const inputStyle: CSSProperties = {
  padding: "8px 10px",
  borderRadius: 6,
  border: "1px solid var(--color-border-default)",
  background: "var(--color-bg-elevated)",
  color: "var(--color-text-primary)",
  fontSize: 13,
};

const hintStyle: CSSProperties = { fontSize: 11, fontWeight: 400, color: "var(--color-text-muted)" };

const GROUPS = ["2160p", "1080p", "720p", "sd"] as const;
const GROUP_LABEL: Record<(typeof GROUPS)[number], string> = { "2160p": "2160p", "1080p": "1080p", "720p": "720p", sd: "SD" };

export default function QualityProfileEditor({
  profile,
  onClose,
}: {
  profile: QualityProfile | null;
  onClose: () => void;
}) {
  const [form, setForm] = useState<ProfileForm>(() => formFromProfile(profile));
  const create = useCreateQualityProfile();
  const update = useUpdateQualityProfile(profile?.id ?? "");
  const saving = create.isPending || update.isPending;

  const checkedCatalog = useMemo(
    () => QUALITY_CATALOG.filter((c) => form.checked.has(qualityKey(c))),
    [form.checked],
  );

  function toggle(key: string) {
    setForm((f) => {
      const checked = new Set(f.checked);
      if (checked.has(key)) checked.delete(key);
      else checked.add(key);
      // Keep the cutoff valid: fall back to the highest allowed quality.
      const first = QUALITY_CATALOG.find((c) => checked.has(qualityKey(c)));
      const cutoff = checked.has(f.cutoff) ? f.cutoff : first ? qualityKey(first) : "";
      const upgradeUntil = checked.has(f.upgradeUntil) ? f.upgradeUntil : "";
      return { ...f, checked, cutoff, upgradeUntil };
    });
  }

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    const problem = validateForm(form);
    if (problem) {
      toast.error(problem);
      return;
    }
    const body = buildProfileRequest(form, profile);
    try {
      if (profile) await update.mutateAsync(body);
      else await create.mutateAsync(body);
      toast.success(profile ? "Quality profile saved" : "Quality profile created");
      onClose();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Could not save the quality profile");
    }
  }

  return (
    <Modal onClose={onClose} width={560}>
      <form onSubmit={onSubmit} style={{ padding: 24, display: "flex", flexDirection: "column", gap: 16 }}>
        <h2 style={{ margin: 0, fontSize: 16, fontWeight: 600, color: "var(--color-text-primary)" }}>
          {profile ? "Edit quality profile" : "New quality profile"}
        </h2>

        {profile?.managed_by_pulse && (
          <div style={{ fontSize: 12, padding: "8px 10px", borderRadius: 6, background: "color-mix(in srgb, var(--color-warning) 12%, transparent)", color: "var(--color-text-primary)" }}>
            This profile is managed by Pulse. Saving changes here disconnects it from Pulse.
          </div>
        )}

        <label style={labelStyle}>
          Name
          <input id="qp-name" required value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} style={inputStyle} />
        </label>

        <fieldset style={{ border: "none", padding: 0, margin: 0, display: "flex", flexDirection: "column", gap: 8 }}>
          <legend style={{ ...labelStyle, padding: 0, marginBottom: 6 }}>
            Allowed qualities
            <span style={hintStyle}>A release must match one of these. Codec and HDR don't matter.</span>
          </legend>
          {GROUPS.map((g) => (
            <div key={g} style={{ display: "grid", gridTemplateColumns: "56px minmax(0, 1fr)", gap: 8, alignItems: "start" }}>
              <span style={{ fontSize: 12, color: "var(--color-text-muted)", paddingTop: 2 }}>{GROUP_LABEL[g]}</span>
              <div style={{ display: "flex", flexWrap: "wrap", gap: "6px 14px" }}>
                {QUALITY_CATALOG.filter((c) => c.resolution === g).map((c) => {
                  const key = qualityKey(c);
                  return (
                    <label key={key} style={{ display: "flex", alignItems: "center", gap: 5, fontSize: 13, color: "var(--color-text-primary)", cursor: "pointer" }}>
                      <input type="checkbox" checked={form.checked.has(key)} onChange={() => toggle(key)} />
                      {c.name}
                    </label>
                  );
                })}
              </div>
            </div>
          ))}
          {form.extras.length > 0 && (
            <p style={{ ...hintStyle, margin: 0 }}>
              Also kept: {form.extras.map((q) => q.name || `${q.source} ${q.resolution}`).join(", ")}
            </p>
          )}
        </fieldset>

        <label style={labelStyle}>
          Cutoff
          <span style={hintStyle}>Once an episode has this quality, Pilot stops looking for upgrades (unless upgrades past the cutoff are on).</span>
          <select id="qp-cutoff" value={form.cutoff} onChange={(e) => setForm({ ...form, cutoff: e.target.value })} style={inputStyle}>
            {checkedCatalog.map((c) => (
              <option key={qualityKey(c)} value={qualityKey(c)}>
                {c.name}
              </option>
            ))}
          </select>
        </label>

        <label style={{ ...labelStyle, flexDirection: "row", alignItems: "center", gap: 8 }}>
          <input id="qp-upgrades" type="checkbox" checked={form.upgradeAllowed} onChange={(e) => setForm({ ...form, upgradeAllowed: e.target.checked })} />
          Keep upgrading past the cutoff
        </label>

        {form.upgradeAllowed && (
          <label style={labelStyle}>
            Upgrade until
            <select id="qp-until" value={form.upgradeUntil} onChange={(e) => setForm({ ...form, upgradeUntil: e.target.value })} style={inputStyle}>
              <option value="">No limit</option>
              {checkedCatalog.map((c) => (
                <option key={qualityKey(c)} value={qualityKey(c)}>
                  {c.name}
                </option>
              ))}
            </select>
          </label>
        )}

        <div style={{ display: "flex", justifyContent: "flex-end", gap: 8 }}>
          <button type="button" onClick={onClose} style={{ ...inputStyle, cursor: "pointer" }}>
            Cancel
          </button>
          <button
            type="submit"
            disabled={saving}
            style={{ ...inputStyle, cursor: "pointer", border: "none", background: "var(--color-accent)", color: "var(--color-accent-fg)" }}
          >
            {saving ? "Saving…" : "Save"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
