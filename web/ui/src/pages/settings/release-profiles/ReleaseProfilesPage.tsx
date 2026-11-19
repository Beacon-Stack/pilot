import { useState, type CSSProperties } from "react";
import { toast } from "sonner";
import { Pencil, Plus, Trash2 } from "lucide-react";
import PageHeader from "@/components/PageHeader";
import Modal from "@beacon-shared/Modal";
import { useConfirm } from "@beacon-shared/ConfirmDialog";
import { useIndexers } from "@/api/indexers";
import {
  useDeleteReleaseProfile,
  useReleaseProfiles,
  useSaveReleaseProfile,
} from "@/api/release-profiles";
import type { ReleaseProfile, ReleaseProfileRequest } from "@/types";

/** One term per line in the editor; blank lines are dropped. */
export function parseTerms(text: string): string[] {
  return text
    .split("\n")
    .map((t) => t.trim())
    .filter((t) => t !== "");
}

const inputStyle: CSSProperties = {
  width: "100%",
  padding: "8px 10px",
  borderRadius: 6,
  border: "1px solid var(--color-border-default)",
  background: "var(--color-bg-elevated)",
  color: "var(--color-text-primary)",
  fontSize: 13,
  boxSizing: "border-box",
};

const labelStyle: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: 6,
  fontSize: 12,
  fontWeight: 500,
  color: "var(--color-text-secondary)",
};

const hintStyle: CSSProperties = { fontSize: 11, fontWeight: 400, color: "var(--color-text-muted)" };

function TermChips({ terms, tone }: { terms: string[]; tone: "good" | "bad" }) {
  if (terms.length === 0) return <span style={{ color: "var(--color-text-muted)" }}>—</span>;
  return (
    <span style={{ display: "flex", flexWrap: "wrap", gap: 4 }}>
      {terms.map((t) => (
        <span
          key={t}
          style={{
            fontSize: 11,
            fontFamily: "var(--font-family-mono)",
            padding: "1px 6px",
            borderRadius: 4,
            background: tone === "good" ? "var(--color-success-muted, rgba(52,211,153,0.12))" : "var(--color-danger-muted, rgba(248,113,113,0.12))",
            color: tone === "good" ? "var(--color-success)" : "var(--color-danger)",
          }}
        >
          {t}
        </span>
      ))}
    </span>
  );
}

function ProfileEditor({ profile, onClose }: { profile: ReleaseProfile | null; onClose: () => void }) {
  const { data: indexers } = useIndexers();
  const save = useSaveReleaseProfile();
  const [name, setName] = useState(profile?.name ?? "");
  const [enabled, setEnabled] = useState(profile?.enabled ?? true);
  const [required, setRequired] = useState((profile?.required ?? []).join("\n"));
  const [ignored, setIgnored] = useState((profile?.ignored ?? []).join("\n"));
  const [indexerId, setIndexerId] = useState(profile?.indexer_id ?? "");

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    const body: ReleaseProfileRequest = {
      name: name.trim(),
      enabled,
      required: parseTerms(required),
      ignored: parseTerms(ignored),
      indexer_id: indexerId,
    };
    try {
      await save.mutateAsync({ id: profile?.id, body });
      toast.success(profile ? "Release profile saved" : "Release profile created");
      onClose();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Could not save the release profile");
    }
  }

  return (
    <Modal onClose={onClose} width={520}>
      <form onSubmit={onSubmit} style={{ padding: 24, display: "flex", flexDirection: "column", gap: 16 }}>
        <h2 style={{ margin: 0, fontSize: 16, fontWeight: 600, color: "var(--color-text-primary)" }}>
          {profile ? "Edit release profile" : "New release profile"}
        </h2>

        <label style={labelStyle}>
          Name
          <input id="rp-name" required value={name} onChange={(e) => setName(e.target.value)} style={inputStyle} />
        </label>

        <label style={labelStyle}>
          Must contain
          <span style={hintStyle}>One term per line. A release must contain at least one. Leave empty for no requirement.</span>
          <textarea id="rp-required" rows={3} value={required} onChange={(e) => setRequired(e.target.value)} style={{ ...inputStyle, fontFamily: "var(--font-family-mono)" }} />
        </label>

        <label style={labelStyle}>
          Must not contain
          <span style={hintStyle}>One term per line. A release containing any of these is skipped.</span>
          <textarea id="rp-ignored" rows={3} value={ignored} onChange={(e) => setIgnored(e.target.value)} style={{ ...inputStyle, fontFamily: "var(--font-family-mono)" }} />
        </label>
        <p style={{ ...hintStyle, margin: 0 }}>
          Terms are case-insensitive. Write a term as <code>/pattern/</code> to use a regular expression.
        </p>

        <label style={labelStyle}>
          Indexer
          <select id="rp-indexer" value={indexerId} onChange={(e) => setIndexerId(e.target.value)} style={inputStyle}>
            <option value="">All indexers</option>
            {(indexers ?? []).map((ix) => (
              <option key={ix.id} value={ix.id}>
                {ix.name}
              </option>
            ))}
          </select>
        </label>

        <label style={{ ...labelStyle, flexDirection: "row", alignItems: "center", gap: 8 }}>
          <input id="rp-enabled" type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
          Enabled
        </label>

        <div style={{ display: "flex", justifyContent: "flex-end", gap: 8 }}>
          <button type="button" onClick={onClose} style={{ ...inputStyle, width: "auto", cursor: "pointer" }}>
            Cancel
          </button>
          <button
            type="submit"
            disabled={save.isPending}
            style={{
              ...inputStyle,
              width: "auto",
              cursor: "pointer",
              background: "var(--color-accent)",
              color: "var(--color-accent-fg)",
              border: "none",
            }}
          >
            {save.isPending ? "Saving…" : "Save"}
          </button>
        </div>
      </form>
    </Modal>
  );
}

export default function ReleaseProfilesPage() {
  const { data, isLoading, error } = useReleaseProfiles();
  const { data: indexers } = useIndexers();
  const remove = useDeleteReleaseProfile();
  const confirm = useConfirm();
  const [editing, setEditing] = useState<ReleaseProfile | null | undefined>(undefined);

  const indexerName = (id: string) => (id === "" ? "All indexers" : indexers?.find((i) => i.id === id)?.name ?? "Unknown indexer");

  async function onDelete(p: ReleaseProfile) {
    const ok = await confirm({ title: "Delete release profile", message: `Delete "${p.name}"?`, confirmLabel: "Delete", danger: true });
    if (!ok) return;
    try {
      await remove.mutateAsync(p.id);
      toast.success(`Deleted "${p.name}"`);
    } catch {
      toast.error("Could not delete the release profile");
    }
  }

  return (
    <div style={{ padding: 32, maxWidth: 900 }}>
      <PageHeader
        title="Release Profiles"
        description="Terms a release title must contain or must not contain. Applied to every search and to RSS sync."
      />

      <div style={{ display: "flex", justifyContent: "flex-end", marginBottom: 12 }}>
        <button
          type="button"
          onClick={() => setEditing(null)}
          style={{
            display: "flex",
            alignItems: "center",
            gap: 6,
            padding: "7px 12px",
            borderRadius: 6,
            border: "none",
            background: "var(--color-accent)",
            color: "var(--color-accent-fg)",
            fontSize: 13,
            cursor: "pointer",
          }}
        >
          <Plus size={14} /> Add release profile
        </button>
      </div>

      {isLoading && <div className="skeleton" style={{ height: 96, borderRadius: 8 }} />}
      {error && <div style={{ color: "var(--color-danger)", fontSize: 13 }}>Could not load release profiles: {(error as Error).message}</div>}

      {data && data.length === 0 && (
        <div style={{ textAlign: "center", padding: "48px 24px", color: "var(--color-text-muted)", fontSize: 13 }}>
          No release profiles. Add one to skip releases containing words like CAM, or to require a release group.
        </div>
      )}

      {data && data.length > 0 && (
        <div style={{ background: "var(--color-bg-surface)", border: "1px solid var(--color-border-subtle)", borderRadius: 8 }}>
          {data.map((p, idx) => (
            <div
              key={p.id}
              style={{
                display: "grid",
                gridTemplateColumns: "minmax(0, 1fr) auto",
                gap: 12,
                padding: "14px 16px",
                borderTop: idx === 0 ? "none" : "1px solid var(--color-border-subtle)",
                opacity: p.enabled ? 1 : 0.55,
              }}
            >
              <div style={{ display: "flex", flexDirection: "column", gap: 6, minWidth: 0 }}>
                <div style={{ fontSize: 14, fontWeight: 500, color: "var(--color-text-primary)" }}>
                  {p.name}
                  {!p.enabled && <span style={{ marginLeft: 8, fontSize: 11, color: "var(--color-text-muted)" }}>Disabled</span>}
                </div>
                <div style={{ display: "grid", gridTemplateColumns: "110px minmax(0, 1fr)", gap: "4px 8px", fontSize: 12, color: "var(--color-text-secondary)" }}>
                  <span>Must contain</span>
                  <TermChips terms={p.required} tone="good" />
                  <span>Must not contain</span>
                  <TermChips terms={p.ignored} tone="bad" />
                  <span>Indexer</span>
                  <span>{indexerName(p.indexer_id)}</span>
                </div>
              </div>
              <div style={{ display: "flex", gap: 4, alignItems: "flex-start" }}>
                <button type="button" title={`Edit ${p.name}`} onClick={() => setEditing(p)} style={{ background: "none", border: "none", cursor: "pointer", color: "var(--color-text-muted)", padding: 4 }}>
                  <Pencil size={14} />
                </button>
                <button type="button" title={`Delete ${p.name}`} onClick={() => onDelete(p)} style={{ background: "none", border: "none", cursor: "pointer", color: "var(--color-text-muted)", padding: 4 }}>
                  <Trash2 size={14} />
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {editing !== undefined && <ProfileEditor profile={editing} onClose={() => setEditing(undefined)} />}
    </div>
  );
}
