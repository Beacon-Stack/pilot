import { useEffect, useState, type CSSProperties } from "react";
import { toast } from "sonner";
import { ArchiveRestore, Download, Plus, Trash2 } from "lucide-react";
import { useConfirm } from "@beacon-shared/ConfirmDialog";
import {
  backupDownloadURL,
  formatBytes,
  useBackupSettings,
  useBackups,
  useCreateBackup,
  useDeleteBackup,
  useRestoreBackup,
  useSaveBackupSettings,
} from "@/api/backups";
import type { BackupInfo, BackupSettings } from "@/types";

const card: CSSProperties = {
  background: "var(--color-bg-surface)",
  border: "1px solid var(--color-border-subtle)",
  borderRadius: 8,
  padding: 20,
  boxShadow: "var(--shadow-card)",
  display: "flex",
  flexDirection: "column",
  gap: 16,
};
const sectionHeader: CSSProperties = {
  fontSize: 11,
  fontWeight: 600,
  letterSpacing: "0.08em",
  textTransform: "uppercase",
  color: "var(--color-text-muted)",
  margin: 0,
};
const labelStyle: CSSProperties = { display: "flex", flexDirection: "column", gap: 6, fontSize: 12, fontWeight: 500, color: "var(--color-text-secondary)" };
const input: CSSProperties = {
  width: 100,
  padding: "7px 10px",
  borderRadius: 6,
  border: "1px solid var(--color-border-default)",
  background: "var(--color-bg-elevated)",
  color: "var(--color-text-primary)",
  fontSize: 13,
};
const button: CSSProperties = {
  display: "inline-flex",
  alignItems: "center",
  gap: 6,
  background: "var(--color-bg-elevated)",
  border: "1px solid var(--color-border-default)",
  borderRadius: 6,
  padding: "7px 14px",
  fontSize: 13,
  color: "var(--color-text-secondary)",
  cursor: "pointer",
};
const iconButton: CSSProperties = { background: "none", border: "none", cursor: "pointer", color: "var(--color-text-muted)", padding: 4, display: "inline-flex" };

/** Returns an error message, or null when the schedule can be saved. */
export function validateBackupSettings(s: BackupSettings): string | null {
  if (!Number.isInteger(s.interval_days) || s.interval_days < 0 || s.interval_days > 365) return "The interval must be 0 (off) to 365 days.";
  if (!Number.isInteger(s.retention_days) || s.retention_days < 1 || s.retention_days > 3650) return "Keep backups for 1 to 3650 days.";
  return null;
}

function BackupRow({ b, last }: { b: BackupInfo; last: boolean }) {
  const confirm = useConfirm();
  const remove = useDeleteBackup();
  const restore = useRestoreBackup();
  const when = new Date(b.created_at).toLocaleString();

  async function onRestore() {
    const ok = await confirm({
      title: "Restore backup",
      message: `Replace the current database with the backup from ${when}? Everything since then is lost. The restore happens when Pilot restarts.`,
      confirmLabel: "Restore",
      danger: true,
    });
    if (!ok) return;
    try {
      await restore.mutateAsync(b.name);
      toast.success("Restore staged. Restart Pilot to apply it.");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Could not stage the restore");
    }
  }

  async function onDelete() {
    const ok = await confirm({ title: "Delete backup", message: `Delete the backup from ${when}?`, confirmLabel: "Delete", danger: true });
    if (!ok) return;
    try {
      await remove.mutateAsync(b.name);
      toast.success("Backup deleted");
    } catch {
      toast.error("Could not delete the backup");
    }
  }

  return (
    <li
      style={{
        display: "grid",
        gridTemplateColumns: "minmax(0, 1fr) auto",
        alignItems: "center",
        gap: 12,
        padding: "10px 12px",
        borderBottom: last ? "none" : "1px solid var(--color-border-subtle)",
      }}
    >
      <div style={{ display: "flex", flexDirection: "column", gap: 2, minWidth: 0 }}>
        <span style={{ fontSize: 13, color: "var(--color-text-primary)" }}>{when}</span>
        <span style={{ fontSize: 11, color: "var(--color-text-muted)" }}>
          {b.kind === "scheduled" ? "Scheduled" : "Manual"} · {formatBytes(b.size)}
        </span>
      </div>
      <div style={{ display: "flex", gap: 2 }}>
        <a href={backupDownloadURL(b.name)} title={`Download backup from ${when}`} style={iconButton}>
          <Download size={14} />
        </a>
        <button type="button" title={`Restore backup from ${when}`} onClick={() => void onRestore()} style={iconButton}>
          <ArchiveRestore size={14} />
        </button>
        <button type="button" title={`Delete backup from ${when}`} onClick={() => void onDelete()} style={iconButton}>
          <Trash2 size={14} />
        </button>
      </div>
    </li>
  );
}

export default function ScheduledBackups() {
  const { data: settings } = useBackupSettings();
  const { data: list, isLoading } = useBackups();
  const save = useSaveBackupSettings();
  const create = useCreateBackup();
  const [form, setForm] = useState<BackupSettings | null>(null);

  useEffect(() => {
    if (settings) setForm(settings);
  }, [settings]);

  async function onSave(e: React.FormEvent) {
    e.preventDefault();
    if (!form) return;
    const problem = validateBackupSettings(form);
    if (problem) {
      toast.error(problem);
      return;
    }
    try {
      await save.mutateAsync(form);
      toast.success("Backup schedule saved");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Could not save the backup schedule");
    }
  }

  async function onBackUpNow() {
    try {
      await create.mutateAsync();
      toast.success("Backup made");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Backup failed");
    }
  }

  return (
    <section aria-label="Scheduled backups" style={card}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
        <p style={sectionHeader}>Backups</p>
        <button type="button" onClick={() => void onBackUpNow()} disabled={create.isPending} style={button}>
          <Plus size={14} strokeWidth={2} />
          {create.isPending ? "Backing up…" : "Back up now"}
        </button>
      </div>

      {form && (
        <form onSubmit={onSave} style={{ display: "flex", alignItems: "flex-end", gap: 16, flexWrap: "wrap" }}>
          <label style={labelStyle}>
            Every (days)
            <input id="bk-interval" type="number" min={0} max={365} value={form.interval_days} onChange={(e) => setForm({ ...form, interval_days: Number(e.target.value) })} style={input} />
          </label>
          <label style={labelStyle}>
            Keep for (days)
            <input id="bk-retention" type="number" min={1} max={3650} value={form.retention_days} onChange={(e) => setForm({ ...form, retention_days: Number(e.target.value) })} style={input} />
          </label>
          <button type="submit" disabled={save.isPending} style={{ ...button, background: "var(--color-accent)", color: "var(--color-accent-fg)", border: "none" }}>
            {save.isPending ? "Saving…" : "Save schedule"}
          </button>
        </form>
      )}
      <p style={{ fontSize: 12, color: "var(--color-text-muted)", margin: 0 }}>
        {form?.interval_days === 0 ? "Scheduled backups are off." : "Scheduled backups older than the retention period are deleted; the newest is always kept."} Backups you make yourself are never deleted automatically.
        {list?.folder && (
          <>
            {" "}Stored in <code style={{ overflowWrap: "anywhere" }}>{list.folder}</code>.
          </>
        )}
      </p>

      {isLoading && <div className="skeleton" style={{ height: 48, borderRadius: 6 }} />}
      {list && list.backups.length === 0 && <p style={{ fontSize: 13, color: "var(--color-text-muted)", margin: 0 }}>No backups yet.</p>}
      {list && list.backups.length > 0 && (
        <ul style={{ listStyle: "none", margin: 0, padding: 0, border: "1px solid var(--color-border-subtle)", borderRadius: 6 }}>
          {list.backups.map((b, i) => (
            <BackupRow key={b.name} b={b} last={i === list.backups.length - 1} />
          ))}
        </ul>
      )}
    </section>
  );
}
