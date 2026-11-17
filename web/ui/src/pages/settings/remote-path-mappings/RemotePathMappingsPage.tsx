import { useState, type CSSProperties } from "react";
import { toast } from "sonner";
import { ArrowRight, Pencil, Plus, Trash2 } from "lucide-react";
import PageHeader from "@/components/PageHeader";
import Modal from "@beacon-shared/Modal";
import { useConfirm } from "@beacon-shared/ConfirmDialog";
import { useDownloadClients } from "@/api/download-clients";
import { useDeleteRemotePathMapping, useRemotePathMappings, useSaveRemotePathMapping } from "@/api/remote-path-mappings";
import type { RemotePathMapping } from "@/types";

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
const monoInput: CSSProperties = { ...inputStyle, fontFamily: "var(--font-family-mono)" };
const labelStyle: CSSProperties = { display: "flex", flexDirection: "column", gap: 6, fontSize: 12, fontWeight: 500, color: "var(--color-text-secondary)" };
const hintStyle: CSSProperties = { fontSize: 11, fontWeight: 400, color: "var(--color-text-muted)" };
const iconButton: CSSProperties = { background: "none", border: "none", cursor: "pointer", color: "var(--color-text-muted)", padding: 4 };

function MappingEditor({ mapping, onClose }: { mapping: RemotePathMapping | null; onClose: () => void }) {
  const { data: clients } = useDownloadClients();
  const save = useSaveRemotePathMapping();
  const [clientId, setClientId] = useState(mapping?.download_client_id ?? "");
  const [remotePath, setRemotePath] = useState(mapping?.remote_path ?? "");
  const [localPath, setLocalPath] = useState(mapping?.local_path ?? "");

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    try {
      await save.mutateAsync({
        id: mapping?.id,
        body: { download_client_id: clientId, remote_path: remotePath.trim(), local_path: localPath.trim() },
      });
      toast.success(mapping ? "Mapping saved" : "Mapping added");
      onClose();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Could not save the mapping");
    }
  }

  return (
    <Modal onClose={onClose} width={520}>
      <form onSubmit={onSubmit} style={{ padding: 24, display: "flex", flexDirection: "column", gap: 16 }}>
        <h2 style={{ margin: 0, fontSize: 16, fontWeight: 600, color: "var(--color-text-primary)" }}>
          {mapping ? "Edit remote path mapping" : "New remote path mapping"}
        </h2>

        <label style={labelStyle}>
          Download client
          <select id="rpm-client" value={clientId} onChange={(e) => setClientId(e.target.value)} style={inputStyle}>
            <option value="">Any download client</option>
            {(clients ?? []).map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </select>
        </label>

        <label style={labelStyle}>
          Remote path
          <span style={hintStyle}>The folder as the download client reports it, e.g. /downloads or C:\Downloads.</span>
          <input id="rpm-remote" required value={remotePath} onChange={(e) => setRemotePath(e.target.value)} placeholder="/downloads" style={monoInput} />
        </label>

        <label style={labelStyle}>
          Local path
          <span style={hintStyle}>The same folder as Pilot sees it on this machine.</span>
          <input id="rpm-local" required value={localPath} onChange={(e) => setLocalPath(e.target.value)} placeholder="/mnt/media/downloads" style={monoInput} />
        </label>

        <div style={{ display: "flex", justifyContent: "flex-end", gap: 8 }}>
          <button type="button" onClick={onClose} style={{ ...inputStyle, width: "auto", cursor: "pointer" }}>
            Cancel
          </button>
          <button
            type="submit"
            disabled={save.isPending}
            style={{ ...inputStyle, width: "auto", cursor: "pointer", background: "var(--color-accent)", color: "var(--color-accent-fg)", border: "none" }}
          >
            {save.isPending ? "Saving…" : "Save"}
          </button>
        </div>
      </form>
    </Modal>
  );
}

export default function RemotePathMappingsPage() {
  const { data, isLoading, error } = useRemotePathMappings();
  const { data: clients } = useDownloadClients();
  const remove = useDeleteRemotePathMapping();
  const confirm = useConfirm();
  const [editing, setEditing] = useState<RemotePathMapping | null | undefined>(undefined);

  const clientName = (id: string) => (id === "" ? "Any download client" : clients?.find((c) => c.id === id)?.name ?? "Unknown client");

  async function onDelete(m: RemotePathMapping) {
    const ok = await confirm({ title: "Delete mapping", message: `Stop mapping ${m.remote_path} to ${m.local_path}?`, confirmLabel: "Delete", danger: true });
    if (!ok) return;
    try {
      await remove.mutateAsync(m.id);
      toast.success("Mapping deleted");
    } catch {
      toast.error("Could not delete the mapping");
    }
  }

  return (
    <div style={{ padding: 32, maxWidth: 900 }}>
      <PageHeader
        title="Remote Path Mappings"
        description="When a download client runs in a container or on another machine, it reports paths as it sees them. Map each of its folders to the same folder as Pilot sees it, so finished downloads can be imported."
      />

      <div style={{ display: "flex", justifyContent: "flex-end", marginBottom: 12 }}>
        <button
          type="button"
          onClick={() => setEditing(null)}
          style={{ display: "flex", alignItems: "center", gap: 6, padding: "7px 12px", borderRadius: 6, border: "none", background: "var(--color-accent)", color: "var(--color-accent-fg)", fontSize: 13, cursor: "pointer" }}
        >
          <Plus size={14} /> Add mapping
        </button>
      </div>

      {isLoading && <div className="skeleton" style={{ height: 96, borderRadius: 8 }} />}
      {error && <div style={{ color: "var(--color-danger)", fontSize: 13 }}>Could not load mappings: {(error as Error).message}</div>}

      {data && data.length === 0 && (
        <div style={{ textAlign: "center", padding: "48px 24px", color: "var(--color-text-muted)", fontSize: 13 }}>
          No mappings. You only need one if imports fail because a download's path doesn't exist on this machine.
        </div>
      )}

      {data && data.length > 0 && (
        <ul style={{ listStyle: "none", margin: 0, padding: 0, background: "var(--color-bg-surface)", border: "1px solid var(--color-border-subtle)", borderRadius: 8 }}>
          {data.map((m, idx) => (
            <li
              key={m.id}
              style={{
                display: "grid",
                gridTemplateColumns: "minmax(0, 1fr) auto",
                gap: 12,
                padding: "12px 16px",
                borderTop: idx === 0 ? "none" : "1px solid var(--color-border-subtle)",
              }}
            >
              <div style={{ display: "flex", flexDirection: "column", gap: 4, minWidth: 0 }}>
                <div style={{ display: "flex", alignItems: "center", flexWrap: "wrap", gap: 8, fontFamily: "var(--font-family-mono)", fontSize: 13, color: "var(--color-text-primary)", overflowWrap: "anywhere" }}>
                  <span>{m.remote_path}</span>
                  <ArrowRight size={14} aria-label="maps to" style={{ color: "var(--color-text-muted)", flexShrink: 0 }} />
                  <span>{m.local_path}</span>
                </div>
                <span style={{ fontSize: 12, color: "var(--color-text-muted)" }}>{clientName(m.download_client_id)}</span>
              </div>
              <div style={{ display: "flex", gap: 4, alignItems: "flex-start" }}>
                <button type="button" title={`Edit ${m.remote_path}`} onClick={() => setEditing(m)} style={iconButton}>
                  <Pencil size={14} />
                </button>
                <button type="button" title={`Delete ${m.remote_path}`} onClick={() => onDelete(m)} style={iconButton}>
                  <Trash2 size={14} />
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}

      {editing !== undefined && <MappingEditor mapping={editing} onClose={() => setEditing(undefined)} />}
    </div>
  );
}
