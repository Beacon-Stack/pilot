import { useState } from "react";
import { Link } from "react-router-dom";
import { useHistory, type GrabHistoryItem } from "@/api/history";
import { formatBytes, formatDate } from "@/lib/utils";
import TableScroll from "@beacon-shared/TableScroll";

const SELECT_STYLE: React.CSSProperties = {
  background: "var(--color-bg-elevated)",
  border: "1px solid var(--color-border-default)",
  borderRadius: 5,
  padding: "5px 10px",
  fontSize: 12,
  color: "var(--color-text-primary)",
  cursor: "pointer",
  outline: "none",
};

const STATUS_COLORS: Record<string, string> = {
  completed: "var(--color-success)",
  failed: "var(--color-danger, #ef4444)",
  pending: "var(--color-warning)",
  downloading: "var(--color-accent)",
  queued: "var(--color-warning)",
  removed: "var(--color-text-muted)",
};

function StatusBadge({ status }: { status: string }) {
  const color = STATUS_COLORS[status.toLowerCase()] ?? "var(--color-text-muted)";
  return (
    <span
      style={{
        display: "inline-block",
        padding: "2px 8px",
        borderRadius: 4,
        fontSize: 11,
        fontWeight: 600,
        background: `color-mix(in srgb, ${color} 14%, transparent)`,
        color,
        textTransform: "capitalize",
        border: `1px solid color-mix(in srgb, ${color} 30%, transparent)`,
      }}
    >
      {status}
    </span>
  );
}

function QualityBadge({ resolution, source }: { resolution: string; source: string }) {
  if (!resolution && !source) return <span style={{ color: "var(--color-text-muted)", fontSize: 12 }}>—</span>;
  return (
    <span
      style={{
        display: "inline-flex",
        gap: 4,
        alignItems: "center",
        fontSize: 11,
        fontWeight: 600,
        color: "var(--color-text-secondary)",
        background: "var(--color-bg-elevated)",
        border: "1px solid var(--color-border-subtle)",
        borderRadius: 4,
        padding: "2px 7px",
      }}
    >
      {resolution && <span>{resolution}</span>}
      {resolution && source && <span style={{ opacity: 0.4 }}>·</span>}
      {source && <span style={{ fontWeight: 400 }}>{source}</span>}
    </span>
  );
}

function HistoryRow({ item, isLast }: { item: GrabHistoryItem; isLast: boolean }) {
  return (
    <tr style={{ borderBottom: isLast ? "none" : "1px solid var(--color-border-subtle)" }}>
      <td style={{ padding: "12px 20px", verticalAlign: "middle" }}>
        <Link
          to={`/series/${item.series_id}`}
          style={{
            fontSize: 13,
            color: "var(--color-text-primary)",
            fontWeight: 500,
            textDecoration: "none",
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
            maxWidth: 380,
            display: "block",
          }}
          title={item.release_title}
        >
          {item.release_title}
        </Link>
        {(item.season_number !== undefined) && (
          <div style={{ fontSize: 11, color: "var(--color-text-muted)", marginTop: 2 }}>
            Season {item.season_number}
          </div>
        )}
      </td>
      <td style={{ padding: "12px 20px", verticalAlign: "middle", whiteSpace: "nowrap" }}>
        <QualityBadge resolution={item.release_resolution} source={item.release_source} />
      </td>
      <td style={{ padding: "12px 20px", verticalAlign: "middle", fontSize: 12, color: "var(--color-text-muted)", textTransform: "capitalize", whiteSpace: "nowrap" }}>
        {item.protocol || "—"}
      </td>
      <td style={{ padding: "12px 20px", verticalAlign: "middle", fontSize: 12, color: "var(--color-text-muted)", fontFamily: "var(--font-family-mono, monospace)", whiteSpace: "nowrap" }}>
        {item.size > 0 ? formatBytes(item.size) : "—"}
      </td>
      <td style={{ padding: "12px 20px", verticalAlign: "middle", whiteSpace: "nowrap" }}>
        <StatusBadge status={item.download_status} />
      </td>
      <td style={{ padding: "12px 20px", verticalAlign: "middle", fontSize: 12, color: "var(--color-text-muted)", whiteSpace: "nowrap" }}>
        {formatDate(item.grabbed_at)}
      </td>
    </tr>
  );
}

const PER_PAGE = 50;

export default function HistoryPage() {
  const [page, setPage] = useState(1);
  const [statusFilter, setStatusFilter] = useState("");

  const { data, isLoading, error } = useHistory(page, PER_PAGE);
  const items = (data?.items ?? []).filter(
    (item) => !statusFilter || item.download_status.toLowerCase() === statusFilter
  );
  const total = data?.total ?? 0;
  const pageCount = Math.max(1, Math.ceil(total / PER_PAGE));

  return (
    <div style={{ padding: 24, maxWidth: 1100, display: "flex", flexDirection: "column", gap: 24 }}>
      {/* Header */}
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 16, flexWrap: "wrap" }}>
        <div>
          <h1 style={{ margin: 0, fontSize: 20, fontWeight: 600, color: "var(--color-text-primary)", letterSpacing: "-0.01em" }}>
            History
          </h1>
          <p style={{ margin: "4px 0 0", fontSize: 13, color: "var(--color-text-secondary)" }}>
            {isLoading
              ? "Loading…"
              : error
              ? "Failed to load history."
              : total === 0
              ? "No grabs recorded yet."
              : `${total.toLocaleString()} grab${total !== 1 ? "s" : ""}`}
          </p>
        </div>

        {/* Filters */}
        <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
          <select value={statusFilter} onChange={(e) => { setStatusFilter(e.target.value); setPage(1); }} style={SELECT_STYLE}>
            <option value="">All statuses</option>
            <option value="completed">Completed</option>
            <option value="downloading">Downloading</option>
            <option value="queued">Queued</option>
            <option value="failed">Failed</option>
            <option value="removed">Removed</option>
          </select>
          {statusFilter && (
            <button
              onClick={() => { setStatusFilter(""); setPage(1); }}
              style={{ ...SELECT_STYLE, color: "var(--color-text-muted)", border: "none", background: "none" }}
            >
              Clear
            </button>
          )}
        </div>
      </div>

      {/* Table card */}
      <div style={{ background: "var(--color-bg-surface)", border: "1px solid var(--color-border-subtle)", borderRadius: 8, overflow: "hidden" }}>
        {isLoading ? (
          <div style={{ padding: 20, display: "flex", flexDirection: "column", gap: 16 }}>
            {[1, 2, 3, 4, 5].map((i) => (
              <div key={i} className="skeleton" style={{ height: 14, borderRadius: 3 }} />
            ))}
          </div>
        ) : error ? (
          <div style={{ padding: 32, textAlign: "center", color: "var(--color-danger, #ef4444)", fontSize: 13 }}>
            Failed to load history. Please try again.
          </div>
        ) : items.length === 0 ? (
          <div style={{ padding: 48, textAlign: "center", color: "var(--color-text-muted)", fontSize: 14 }}>
            <div style={{ fontSize: 32, marginBottom: 12, opacity: 0.4 }}>📋</div>
            <div style={{ fontWeight: 500, color: "var(--color-text-secondary)", marginBottom: 4 }}>
              {statusFilter ? "No results match the current filter." : "No history yet"}
            </div>
            {!statusFilter && (
              <div style={{ fontSize: 12 }}>Grab a release from a series detail page to get started.</div>
            )}
          </div>
        ) : (
          <TableScroll minWidth={680}>
            <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}>
              <thead>
                <tr style={{ borderBottom: "1px solid var(--color-border-subtle)" }}>
                  {["Release", "Quality", "Protocol", "Size", "Status", "Grabbed"].map((h) => (
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
                  <HistoryRow key={item.id} item={item} isLast={idx === items.length - 1} />
                ))}
              </tbody>
            </table>
          </TableScroll>
        )}
      </div>

      {/* Pagination */}
      {!isLoading && !error && pageCount > 1 && (
        <div style={{ display: "flex", alignItems: "center", justifyContent: "center", gap: 8 }}>
          <button
            disabled={page === 1}
            onClick={() => setPage((p) => p - 1)}
            style={{
              padding: "5px 12px",
              borderRadius: 5,
              border: "1px solid var(--color-border-default)",
              background: "var(--color-bg-elevated)",
              color: page === 1 ? "var(--color-text-muted)" : "var(--color-text-primary)",
              fontSize: 12,
              cursor: page === 1 ? "default" : "pointer",
              opacity: page === 1 ? 0.5 : 1,
            }}
          >
            Previous
          </button>
          <span style={{ fontSize: 12, color: "var(--color-text-muted)" }}>
            Page {page} of {pageCount}
          </span>
          <button
            disabled={page >= pageCount}
            onClick={() => setPage((p) => p + 1)}
            style={{
              padding: "5px 12px",
              borderRadius: 5,
              border: "1px solid var(--color-border-default)",
              background: "var(--color-bg-elevated)",
              color: page >= pageCount ? "var(--color-text-muted)" : "var(--color-text-primary)",
              fontSize: 12,
              cursor: page >= pageCount ? "default" : "pointer",
              opacity: page >= pageCount ? 0.5 : 1,
            }}
          >
            Next
          </button>
        </div>
      )}
    </div>
  );
}
