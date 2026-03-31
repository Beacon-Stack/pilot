import { useState } from "react";
import { useQueue, useRemoveFromQueue } from "@/api/queue";
import { formatBytes, formatDate } from "@/lib/utils";
import PageHeader from "@/components/PageHeader";
import type { QueueItem } from "@/types";

function progressPct(item: QueueItem): number {
  if (!item.size || item.size === 0) return 0;
  return Math.min(100, Math.round((item.downloaded_bytes / item.size) * 100));
}

function ProgressBar({ pct, status }: { pct: number; status: string }) {
  const color =
    status === "failed"
      ? "var(--color-danger)"
      : status === "completed"
      ? "var(--color-success)"
      : "var(--color-accent)";

  return (
    <div
      style={{
        width: "100%",
        height: 4,
        background: "var(--color-border-subtle)",
        borderRadius: 2,
        overflow: "hidden",
      }}
    >
      <div
        style={{
          width: `${pct}%`,
          height: "100%",
          background: color,
          borderRadius: 2,
          transition: "width 0.4s ease",
        }}
      />
    </div>
  );
}

function StatusBadge({ status }: { status: string }) {
  const map: Record<string, { color: string; bg: string; label: string }> = {
    downloading: {
      color: "var(--color-accent)",
      bg: "color-mix(in srgb, var(--color-accent) 12%, transparent)",
      label: "Downloading",
    },
    completed: {
      color: "var(--color-success)",
      bg: "color-mix(in srgb, var(--color-success) 12%, transparent)",
      label: "Completed",
    },
    queued: {
      color: "var(--color-warning)",
      bg: "color-mix(in srgb, var(--color-warning) 12%, transparent)",
      label: "Queued",
    },
    paused: {
      color: "var(--color-warning)",
      bg: "color-mix(in srgb, var(--color-warning) 12%, transparent)",
      label: "Paused",
    },
    failed: {
      color: "var(--color-danger)",
      bg: "color-mix(in srgb, var(--color-danger) 12%, transparent)",
      label: "Failed",
    },
    removed: {
      color: "var(--color-text-muted)",
      bg: "color-mix(in srgb, var(--color-text-muted) 10%, transparent)",
      label: "Removed",
    },
  };

  const style = map[status] ?? {
    color: "var(--color-text-muted)",
    bg: "color-mix(in srgb, var(--color-text-muted) 10%, transparent)",
    label: status,
  };

  return (
    <span
      style={{
        display: "inline-block",
        padding: "2px 8px",
        borderRadius: 4,
        fontSize: 11,
        fontWeight: 600,
        textTransform: "uppercase",
        letterSpacing: "0.05em",
        background: style.bg,
        color: style.color,
      }}
    >
      {style.label}
    </span>
  );
}

function QueueRow({ item, isLast }: { item: QueueItem; isLast: boolean }) {
  const [confirmRemove, setConfirmRemove] = useState(false);
  const [deleteFiles, setDeleteFiles] = useState(false);
  const remove = useRemoveFromQueue();

  const pct = progressPct(item);

  function handleRemove() {
    remove.mutate(
      { id: item.grab_id, deleteFiles },
      { onSuccess: () => setConfirmRemove(false) }
    );
  }

  return (
    <>
      {confirmRemove && (
        <tr>
          <td
            colSpan={5}
            style={{
              padding: "10px 20px",
              background:
                "color-mix(in srgb, var(--color-danger) 8%, var(--color-bg-surface))",
              borderBottom:
                "1px solid color-mix(in srgb, var(--color-danger) 25%, transparent)",
            }}
          >
            <div
              style={{
                display: "flex",
                alignItems: "center",
                gap: 16,
                flexWrap: "wrap",
              }}
            >
              <span
                style={{
                  fontSize: 13,
                  color: "var(--color-text-primary)",
                  fontWeight: 500,
                }}
              >
                Remove from queue?
              </span>
              <label
                style={{
                  display: "flex",
                  alignItems: "center",
                  gap: 6,
                  fontSize: 12,
                  color: "var(--color-text-secondary)",
                  cursor: "pointer",
                  userSelect: "none",
                }}
              >
                <input
                  type="checkbox"
                  checked={deleteFiles}
                  onChange={(e) => setDeleteFiles(e.target.checked)}
                  style={{ accentColor: "var(--color-danger)" }}
                />
                Also delete downloaded files
              </label>
              <div style={{ display: "flex", gap: 8, marginLeft: "auto" }}>
                <button
                  onClick={() => setConfirmRemove(false)}
                  style={{
                    background: "var(--color-bg-elevated)",
                    border: "1px solid var(--color-border-default)",
                    borderRadius: 5,
                    padding: "4px 12px",
                    fontSize: 12,
                    color: "var(--color-text-secondary)",
                    cursor: "pointer",
                  }}
                >
                  Cancel
                </button>
                <button
                  onClick={handleRemove}
                  disabled={remove.isPending}
                  style={{
                    background: "var(--color-danger)",
                    border: "none",
                    borderRadius: 5,
                    padding: "4px 12px",
                    fontSize: 12,
                    color: "#fff",
                    fontWeight: 600,
                    cursor: remove.isPending ? "not-allowed" : "pointer",
                    opacity: remove.isPending ? 0.7 : 1,
                  }}
                >
                  {remove.isPending ? "Removing..." : "Yes, Remove"}
                </button>
              </div>
            </div>
          </td>
        </tr>
      )}
      <tr
        style={{
          borderBottom: isLast ? "none" : "1px solid var(--color-border-subtle)",
        }}
      >
        {/* Title + progress bar */}
        <td style={{ padding: "12px 20px", verticalAlign: "middle" }}>
          <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
            <span
              style={{
                fontSize: 13,
                color: "var(--color-text-primary)",
                fontWeight: 500,
                overflow: "hidden",
                textOverflow: "ellipsis",
                whiteSpace: "nowrap",
                maxWidth: 380,
                display: "block",
              }}
              title={item.release_title}
            >
              {item.release_title}
            </span>
            <ProgressBar pct={pct} status={item.status} />
          </div>
        </td>

        {/* Status */}
        <td
          style={{
            padding: "12px 20px",
            verticalAlign: "middle",
            whiteSpace: "nowrap",
          }}
        >
          <StatusBadge status={item.status} />
        </td>

        {/* Progress */}
        <td
          style={{
            padding: "12px 20px",
            verticalAlign: "middle",
            fontSize: 12,
            color: "var(--color-text-muted)",
            fontFamily: "var(--font-family-mono)",
            whiteSpace: "nowrap",
          }}
        >
          {formatBytes(item.downloaded_bytes)} / {formatBytes(item.size)}
          {item.size > 0 && (
            <span
              style={{
                marginLeft: 6,
                color: "var(--color-text-muted)",
                opacity: 0.7,
              }}
            >
              ({pct}%)
            </span>
          )}
        </td>

        {/* Age */}
        <td
          style={{
            padding: "12px 20px",
            verticalAlign: "middle",
            fontSize: 12,
            color: "var(--color-text-muted)",
            whiteSpace: "nowrap",
          }}
        >
          {formatDate(item.grabbed_at)}
        </td>

        {/* Actions */}
        <td
          style={{
            padding: "12px 20px",
            verticalAlign: "middle",
            textAlign: "right",
            whiteSpace: "nowrap",
          }}
        >
          <button
            onClick={() => setConfirmRemove(true)}
            style={{
              background: "transparent",
              border: "1px solid var(--color-border-default)",
              borderRadius: 5,
              padding: "3px 10px",
              fontSize: 12,
              color: "var(--color-text-secondary)",
              cursor: "pointer",
            }}
          >
            Remove
          </button>
        </td>
      </tr>
    </>
  );
}

export default function Queue() {
  const { data, isLoading } = useQueue();
  const items = data ?? [];

  const activeCount = items.filter(
    (i) => i.status === "downloading" || i.status === "queued"
  ).length;

  return (
    <div style={{ padding: 24, maxWidth: 900 }}>
      <PageHeader
        title="Queue"
        description={
          isLoading
            ? "Loading..."
            : items.length === 0
            ? "No active downloads."
            : `${items.length} item${items.length !== 1 ? "s" : ""} — ${activeCount} active`
        }
        action={
          activeCount > 0 ? (
            <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
              <span
                style={{
                  width: 8,
                  height: 8,
                  borderRadius: "50%",
                  background: "var(--color-accent)",
                  display: "inline-block",
                }}
              />
              <span
                style={{
                  fontSize: 12,
                  color: "var(--color-accent)",
                  fontWeight: 500,
                }}
              >
                Downloading
              </span>
            </div>
          ) : undefined
        }
      />

      <div
        style={{
          background: "var(--color-bg-surface)",
          border: "1px solid var(--color-border-subtle)",
          borderRadius: 8,
          boxShadow: "var(--shadow-card)",
          overflow: "hidden",
        }}
      >
        {isLoading ? (
          <div
            style={{ padding: 20, display: "flex", flexDirection: "column", gap: 16 }}
          >
            {[1, 2, 3].map((i) => (
              <div key={i} style={{ display: "flex", flexDirection: "column", gap: 8 }}>
                <div
                  className="skeleton"
                  style={{ height: 14, width: "60%", borderRadius: 3 }}
                />
                <div className="skeleton" style={{ height: 4, borderRadius: 2 }} />
              </div>
            ))}
          </div>
        ) : items.length === 0 ? (
          <div
            style={{
              padding: 48,
              textAlign: "center",
              color: "var(--color-text-muted)",
              fontSize: 14,
            }}
          >
            <div style={{ fontSize: 32, marginBottom: 12, opacity: 0.4 }}>
              &#8595;
            </div>
            <div
              style={{
                fontWeight: 500,
                color: "var(--color-text-secondary)",
                marginBottom: 4,
              }}
            >
              No active downloads
            </div>
            <div style={{ fontSize: 12 }}>
              Grab a release from a series detail page to start downloading.
            </div>
          </div>
        ) : (
          <table
            style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}
          >
            <thead>
              <tr style={{ borderBottom: "1px solid var(--color-border-subtle)" }}>
                {["Title", "Status", "Progress", "Age", ""].map((h) => (
                  <th
                    key={h}
                    style={{
                      textAlign: "left",
                      padding: "8px 20px",
                      fontSize: 11,
                      fontWeight: 600,
                      letterSpacing: "0.08em",
                      textTransform: "uppercase",
                      color: "var(--color-text-muted)",
                      whiteSpace: "nowrap",
                    }}
                  >
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {items.map((item, idx) => (
                <QueueRow
                  key={item.grab_id}
                  item={item}
                  isLast={idx === items.length - 1}
                />
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
