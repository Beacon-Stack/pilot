import { useState, type CSSProperties } from "react";
import { toast } from "sonner";
import { Eye, EyeOff, Trash2, X } from "lucide-react";
import Modal from "@beacon-shared/Modal";
import { useQualityProfiles } from "@/api/quality";
import { useLibraries } from "@/api/libraries";
import { useBulkDeleteSeries, useBulkEditSeries, type SeriesBulkEdit } from "@/api/series-bulk";

const bar: CSSProperties = {
  position: "fixed",
  bottom: 20,
  left: "50%",
  transform: "translateX(-50%)",
  zIndex: 90,
  display: "flex",
  alignItems: "center",
  flexWrap: "wrap",
  justifyContent: "center",
  gap: 8,
  maxWidth: "calc(100vw - 32px)",
  padding: "8px 14px",
  borderRadius: 10,
  background: "var(--color-bg-elevated)",
  border: "1px solid var(--color-border-default)",
  boxShadow: "0 8px 32px rgba(0,0,0,0.4)",
  fontSize: 13,
};
const btn: CSSProperties = {
  display: "inline-flex",
  alignItems: "center",
  gap: 5,
  padding: "5px 10px",
  borderRadius: 6,
  border: "1px solid var(--color-border-default)",
  background: "var(--color-bg-surface)",
  color: "var(--color-text-primary)",
  fontSize: 12,
  cursor: "pointer",
};
const select: CSSProperties = { ...btn, paddingRight: 6 };

function plural(n: number) {
  return `${n} series`;
}

function DeleteDialog({ ids, onClose, onDone }: { ids: string[]; onClose: () => void; onDone: () => void }) {
  const del = useBulkDeleteSeries();
  const [withFiles, setWithFiles] = useState(false);

  async function onDelete() {
    try {
      const r = await del.mutateAsync({ ids, delete_files: withFiles });
      toast.success(`Deleted ${plural(r.count)}`);
      onDone();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Delete failed");
    }
  }

  return (
    <Modal onClose={onClose} width={440}>
      <div style={{ padding: 24, display: "flex", flexDirection: "column", gap: 14 }}>
        <h2 style={{ margin: 0, fontSize: 16, fontWeight: 600, color: "var(--color-text-primary)" }}>Delete {plural(ids.length)}?</h2>
        <p style={{ margin: 0, fontSize: 13, color: "var(--color-text-secondary)" }}>
          They are removed from Pilot with their episodes and history.
        </p>
        <label style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 13, color: "var(--color-text-primary)" }}>
          <input type="checkbox" checked={withFiles} onChange={(e) => setWithFiles(e.target.checked)} />
          Also delete their episode files (moved to the recycle bin if one is set)
        </label>
        <div style={{ display: "flex", justifyContent: "flex-end", gap: 8 }}>
          <button type="button" onClick={onClose} style={btn}>
            Cancel
          </button>
          <button type="button" onClick={() => void onDelete()} disabled={del.isPending} style={{ ...btn, background: "var(--color-danger)", color: "white", border: "none" }}>
            {del.isPending ? "Deleting…" : "Delete"}
          </button>
        </div>
      </div>
    </Modal>
  );
}

export default function SeriesBulkBar({ ids, onClear }: { ids: string[]; onClear: () => void }) {
  const { data: profiles } = useQualityProfiles();
  const { data: libraries } = useLibraries();
  const edit = useBulkEditSeries();
  const [deleting, setDeleting] = useState(false);

  if (ids.length === 0) return null;

  async function apply(change: Omit<SeriesBulkEdit, "ids">, what: string) {
    try {
      const r = await edit.mutateAsync({ ids, ...change });
      toast.success(`${what}: ${plural(r.count)}`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Bulk change failed");
    }
  }

  return (
    <div role="toolbar" aria-label="Selected series" style={bar}>
      <span style={{ fontWeight: 600, color: "var(--color-text-primary)" }}>{plural(ids.length)}</span>
      <button type="button" style={btn} disabled={edit.isPending} onClick={() => void apply({ monitored: true }, "Monitored")}>
        <Eye size={13} /> Monitor
      </button>
      <button type="button" style={btn} disabled={edit.isPending} onClick={() => void apply({ monitored: false }, "Unmonitored")}>
        <EyeOff size={13} /> Unmonitor
      </button>
      <select
        aria-label="Set quality profile"
        style={select}
        value=""
        disabled={edit.isPending}
        onChange={(e) => e.target.value && void apply({ quality_profile_id: e.target.value }, "Quality profile set")}
      >
        <option value="">Quality profile…</option>
        {(profiles ?? []).map((p) => (
          <option key={p.id} value={p.id}>
            {p.name}
          </option>
        ))}
      </select>
      <select
        aria-label="Set library"
        title="Re-points the series to another library. Files are not moved."
        style={select}
        value=""
        disabled={edit.isPending}
        onChange={(e) => e.target.value && void apply({ library_id: e.target.value }, "Library set")}
      >
        <option value="">Library…</option>
        {(libraries ?? []).map((l) => (
          <option key={l.id} value={l.id}>
            {l.name}
          </option>
        ))}
      </select>
      <select
        aria-label="Set series type"
        style={select}
        value=""
        disabled={edit.isPending}
        onChange={(e) => e.target.value && void apply({ series_type: e.target.value as SeriesBulkEdit["series_type"] }, "Series type set")}
      >
        <option value="">Type…</option>
        <option value="standard">Standard</option>
        <option value="anime">Anime</option>
        <option value="daily">Daily</option>
      </select>
      <button type="button" style={{ ...btn, color: "var(--color-danger)" }} onClick={() => setDeleting(true)}>
        <Trash2 size={13} /> Delete…
      </button>
      <button type="button" aria-label="Clear selection" style={{ ...btn, border: "none", background: "none" }} onClick={onClear}>
        <X size={14} />
      </button>
      {deleting && (
        <DeleteDialog
          ids={ids}
          onClose={() => setDeleting(false)}
          onDone={() => {
            setDeleting(false);
            onClear();
          }}
        />
      )}
    </div>
  );
}
