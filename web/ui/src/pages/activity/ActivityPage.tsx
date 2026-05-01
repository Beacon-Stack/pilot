// Activity page — "what's happening right now"
//
// Four rails on a single page (no tabs):
//   1. Currently downloading      — live queue
//   2. Recently imported          — last 24-48h grab_history (completed)
//   3. Needs attention            — failed grabs + failed imports + stalled
//   4. Active background tasks    — placeholder rail (no per-run state yet)
//
// The legacy event-firehose timeline isn't surfaced here anymore; it's
// still reachable via the /api/v1/activity REST endpoint for debugging.
import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import {
  AlertTriangle,
  ArrowDownToLine,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  Clock,
  ExternalLink,
  PauseCircle,
} from "lucide-react";

import PageHeader from "@/components/PageHeader";
import TableScroll from "@beacon-shared/TableScroll";
import { useQueue } from "@/api/queue";
import {
  useActivity,
  useNeedsAttention,
  type ActivityEntry,
  type AttentionItem,
} from "@/api/activity";
import { useSeriesList } from "@/api/series";
import { formatBytes, timeAgo } from "@/lib/utils";
import type { QueueItem, Series } from "@/types";

// ── Shared bits ──────────────────────────────────────────────────────────────

function Rail({
  title,
  count,
  children,
  collapsible = false,
  defaultOpen = true,
}: {
  title: string;
  count?: number;
  children: React.ReactNode;
  collapsible?: boolean;
  defaultOpen?: boolean;
}) {
  const [open, setOpen] = useState(defaultOpen);
  const isOpen = collapsible ? open : true;

  return (
    <section
      style={{
        background: "var(--color-bg-surface)",
        border: "1px solid var(--color-border-subtle)",
        borderRadius: 8,
        boxShadow: "var(--shadow-card)",
        overflow: "hidden",
      }}
    >
      <header
        style={{
          display: "flex",
          alignItems: "center",
          gap: 8,
          padding: "12px 20px",
          borderBottom: isOpen ? "1px solid var(--color-border-subtle)" : "none",
          cursor: collapsible ? "pointer" : "default",
          userSelect: "none",
        }}
        onClick={collapsible ? () => setOpen((o) => !o) : undefined}
      >
        {collapsible &&
          (isOpen ? (
            <ChevronDown size={14} style={{ color: "var(--color-text-muted)" }} />
          ) : (
            <ChevronRight size={14} style={{ color: "var(--color-text-muted)" }} />
          ))}
        <span
          style={{
            fontSize: 11,
            fontWeight: 600,
            letterSpacing: "0.08em",
            textTransform: "uppercase",
            color: "var(--color-text-muted)",
          }}
        >
          {title}
        </span>
        {typeof count === "number" && (
          <span
            style={{
              marginLeft: "auto",
              fontSize: 12,
              color: "var(--color-text-muted)",
            }}
          >
            {count}
          </span>
        )}
      </header>
      {isOpen && <div>{children}</div>}
    </section>
  );
}

function Empty({ children }: { children: React.ReactNode }) {
  return (
    <p
      style={{
        margin: 0,
        padding: "20px",
        fontSize: 13,
        color: "var(--color-text-muted)",
        textAlign: "center",
      }}
    >
      {children}
    </p>
  );
}

function useSeriesIndex() {
  const { data } = useSeriesList();
  return useMemo(() => {
    const map = new Map<string, Series>();
    for (const s of data?.series ?? []) map.set(s.id, s);
    return map;
  }, [data]);
}

function SeriesLabel({ id, fallback, idx }: { id?: string; fallback: string; idx: Map<string, Series> }) {
  if (!id) return <span>{fallback}</span>;
  const s = idx.get(id);
  const title = s?.title ?? fallback;
  return (
    <Link
      to={`/series/${id}`}
      style={{
        color: "var(--color-text-primary)",
        textDecoration: "none",
        fontWeight: 500,
      }}
      onMouseEnter={(e) => ((e.currentTarget as HTMLAnchorElement).style.color = "var(--color-accent)")}
      onMouseLeave={(e) => ((e.currentTarget as HTMLAnchorElement).style.color = "var(--color-text-primary)")}
    >
      {title}
    </Link>
  );
}

// ── 1. Currently downloading ─────────────────────────────────────────────────

function progressPct(item: QueueItem): number {
  if (!item.size || item.size === 0) return 0;
  return Math.min(100, Math.round((item.downloaded_bytes / item.size) * 100));
}

// ETA isn't displayed here yet: the download client reports current
// status (bytes + state) but Pilot doesn't persist a sliding window of
// samples to compute a rate, and React-side state would reset on every
// refetch tick. Surfacing rate/ETA properly belongs in queue.Service
// (server-side EWMA over the polling loop). For now we just show the
// current size and progress — same data the existing /queue page shows.

function DownloadingRail({ idx }: { idx: Map<string, Series> }) {
  const { data, isLoading } = useQueue();
  const items = (data ?? []).filter((i) => i.status === "downloading" || i.status === "queued");

  return (
    <Rail title="Currently downloading" count={items.length}>
      {isLoading ? (
        <Empty>Loading…</Empty>
      ) : items.length === 0 ? (
        <Empty>Nothing downloading right now.</Empty>
      ) : (
        <TableScroll minWidth={700}>
          <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}>
            <thead>
              <tr style={{ borderBottom: "1px solid var(--color-border-subtle)" }}>
                {["Release", "Progress", "Downloaded", "Size", "Protocol"].map((h) => (
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
            {items.map((item, i) => {
              const pct = progressPct(item);
              return (
                <tr
                  key={(item as unknown as { id?: string }).id ?? item.grab_id}
                  style={{
                    borderBottom:
                      i === items.length - 1
                        ? "none"
                        : "1px solid var(--color-border-subtle)",
                  }}
                >
                  <td style={{ padding: "10px 20px", verticalAlign: "middle" }}>
                    <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
                      <span
                        style={{
                          color: "var(--color-text-primary)",
                          overflow: "hidden",
                          textOverflow: "ellipsis",
                          whiteSpace: "nowrap",
                          maxWidth: 460,
                          fontWeight: 500,
                        }}
                        title={item.release_title}
                      >
                        {item.release_title}
                      </span>
                      <span style={{ fontSize: 11, color: "var(--color-text-muted)" }}>
                        <SeriesLabel id={item.series_id} fallback="Unknown series" idx={idx} />
                      </span>
                    </div>
                  </td>
                  <td style={{ padding: "10px 20px", minWidth: 160 }}>
                    <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
                      <div
                        style={{
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
                            background:
                              item.status === "queued"
                                ? "var(--color-warning)"
                                : "var(--color-accent)",
                            transition: "width 0.4s ease",
                          }}
                        />
                      </div>
                      <span style={{ fontSize: 11, color: "var(--color-text-muted)" }}>{pct}%</span>
                    </div>
                  </td>
                  <td
                    style={{
                      padding: "10px 20px",
                      fontSize: 12,
                      color: "var(--color-text-muted)",
                      fontFamily: "var(--font-family-mono)",
                      whiteSpace: "nowrap",
                    }}
                  >
                    {formatBytes(item.downloaded_bytes)}
                  </td>
                  <td
                    style={{
                      padding: "10px 20px",
                      fontSize: 12,
                      color: "var(--color-text-muted)",
                      fontFamily: "var(--font-family-mono)",
                      whiteSpace: "nowrap",
                    }}
                  >
                    {formatBytes(item.size)}
                  </td>
                  <td
                    style={{
                      padding: "10px 20px",
                      fontSize: 12,
                      color: "var(--color-text-muted)",
                      whiteSpace: "nowrap",
                    }}
                  >
                    {item.protocol}
                  </td>
                </tr>
              );
            })}
          </tbody>
          </table>
        </TableScroll>
      )}
    </Rail>
  );
}

// ── 2. Recently imported ─────────────────────────────────────────────────────
//
// Sourced from activity_log (category=import_succeeded, type=import_complete)
// rather than grab_history. activity_log.created_at is the actual import-
// completion time; grab_history.grabbed_at is the time the grab was
// requested, which can be days earlier when the download is large.
// Filtering on grabbed_at hid imports that just landed but were grabbed
// outside the 48h window.

function RecentlyImportedRail({ idx }: { idx: Map<string, Series> }) {
  const { data, isLoading } = useActivity({
    category: "import_succeeded",
    limit: 100,
  });
  const cutoff = useMemo(() => Date.now() - 48 * 3600 * 1000, []);

  const items = useMemo<ActivityEntry[]>(() => {
    const list = data?.activities ?? [];
    return list.filter(
      (a) =>
        a.type === "import_complete" &&
        new Date(a.created_at).getTime() >= cutoff,
    );
  }, [data, cutoff]);

  return (
    <Rail title="Recently imported (last 48h)" count={items.length}>
      {isLoading ? (
        <Empty>Loading…</Empty>
      ) : items.length === 0 ? (
        <Empty>No imports in the last 48 hours.</Empty>
      ) : (
        <ul style={{ margin: 0, padding: 0, listStyle: "none" }}>
          {items.map((a, i) => {
            const series = a.series_id ? idx.get(a.series_id) : undefined;
            const detail = (a.detail ?? {}) as Record<string, unknown>;
            const quality =
              typeof detail.quality === "string" ? detail.quality : "";
            // Prefer the resolved series title from the index; fall
            // back to whatever classify() wrote into the title column.
            // Older activity rows pre-date the title fix and only have
            // "Imported release" — the idx lookup recovers from that.
            const headline =
              series?.title ??
              a.title.replace(/^Imported\s*/, "").replace(/\s*—.*$/, "") ??
              "Imported";
            return (
              <li
                key={a.id}
                style={{
                  padding: "10px 20px",
                  borderBottom:
                    i === items.length - 1
                      ? "none"
                      : "1px solid var(--color-border-subtle)",
                  display: "flex",
                  gap: 12,
                  alignItems: "flex-start",
                }}
              >
                <CheckCircle2
                  size={15}
                  style={{
                    color: "var(--color-success)",
                    flexShrink: 0,
                    marginTop: 2,
                  }}
                />
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div
                    style={{
                      fontSize: 13,
                      color: "var(--color-text-primary)",
                      overflow: "hidden",
                      textOverflow: "ellipsis",
                      whiteSpace: "nowrap",
                    }}
                  >
                    <SeriesLabel
                      id={a.series_id}
                      fallback={headline}
                      idx={idx}
                    />
                  </div>
                  <div
                    style={{
                      fontSize: 11,
                      color: "var(--color-text-muted)",
                      marginTop: 2,
                    }}
                  >
                    {quality && <>{quality} · </>}
                    {timeAgo(a.created_at)}
                  </div>
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </Rail>
  );
}

// ── 3. Needs attention ───────────────────────────────────────────────────────

function attentionIcon(kind: string) {
  switch (kind) {
    case "stalled":
      return PauseCircle;
    case "import_failed":
      return ArrowDownToLine;
    default:
      return AlertTriangle;
  }
}

function attentionColor(kind: string): string {
  return kind === "stalled" ? "var(--color-warning)" : "var(--color-danger)";
}

function NeedsAttentionRail({ idx }: { idx: Map<string, Series> }) {
  const { data, isLoading } = useNeedsAttention(48);
  const items: AttentionItem[] = data?.items ?? [];
  const counts = data?.counts ?? { grab_failed: 0, import_failed: 0, stalled: 0 };

  // Sort newest first (server returns by-bucket; merge & resort).
  const sorted = useMemo(
    () =>
      [...items].sort(
        (a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
      ),
    [items]
  );

  const summary =
    counts.grab_failed + counts.import_failed + counts.stalled === 0
      ? undefined
      : `${counts.grab_failed} failed grab${counts.grab_failed === 1 ? "" : "s"} · ${
          counts.import_failed
        } failed import${counts.import_failed === 1 ? "" : "s"} · ${counts.stalled} stalled`;

  return (
    <Rail title="Needs attention" count={sorted.length}>
      {summary && (
        <div
          style={{
            padding: "8px 20px",
            fontSize: 12,
            color: "var(--color-text-secondary)",
            borderBottom: "1px solid var(--color-border-subtle)",
            background:
              "color-mix(in srgb, var(--color-warning) 5%, transparent)",
          }}
        >
          {summary}
        </div>
      )}
      {isLoading ? (
        <Empty>Loading…</Empty>
      ) : sorted.length === 0 ? (
        <Empty>Nothing needs attention. The last 48 hours are clean.</Empty>
      ) : (
        <ul style={{ margin: 0, padding: 0, listStyle: "none" }}>
          {sorted.map((it, i) => {
            const Icon = attentionIcon(it.kind);
            const color = attentionColor(it.kind);
            return (
              <li
                key={`${it.kind}:${it.grab_id ?? it.created_at}:${i}`}
                style={{
                  padding: "10px 20px",
                  borderBottom:
                    i === sorted.length - 1
                      ? "none"
                      : "1px solid var(--color-border-subtle)",
                  display: "flex",
                  gap: 12,
                  alignItems: "flex-start",
                }}
              >
                <Icon size={15} style={{ color, flexShrink: 0, marginTop: 2 }} />
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div
                    style={{
                      fontSize: 13,
                      color: "var(--color-text-primary)",
                      overflow: "hidden",
                      textOverflow: "ellipsis",
                      whiteSpace: "nowrap",
                    }}
                    title={it.release_title}
                  >
                    {it.release_title}
                  </div>
                  <div style={{ fontSize: 11, color: "var(--color-text-muted)", marginTop: 2 }}>
                    <KindLabel kind={it.kind} />
                    {it.series_id && (
                      <>
                        {" · "}
                        <SeriesLabel id={it.series_id} fallback="" idx={idx} />
                      </>
                    )}
                    {it.detail && <> · {it.detail}</>}
                    <> · {timeAgo(it.created_at)}</>
                  </div>
                </div>
                {it.kind === "stalled" && it.info_hash && (
                  <a
                    href={`#/haul/${it.info_hash}`}
                    onClick={(e) => {
                      // Deep link is best-effort: we don't know the
                      // user's Haul URL from inside Pilot. Surface the
                      // info_hash so the user can open Haul manually.
                      e.preventDefault();
                      navigator.clipboard?.writeText(it.info_hash ?? "");
                    }}
                    title="Copy info_hash (open in Haul)"
                    style={{
                      flexShrink: 0,
                      display: "inline-flex",
                      alignItems: "center",
                      gap: 4,
                      fontSize: 11,
                      color: "var(--color-text-muted)",
                      textDecoration: "none",
                      padding: "3px 8px",
                      border: "1px solid var(--color-border-default)",
                      borderRadius: 5,
                      background: "var(--color-bg-elevated)",
                    }}
                  >
                    Open in Haul <ExternalLink size={11} />
                  </a>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </Rail>
  );
}

function KindLabel({ kind }: { kind: string }) {
  const label =
    kind === "grab_failed"
      ? "Grab failed"
      : kind === "import_failed"
      ? "Import failed"
      : kind === "stalled"
      ? "Stalled"
      : kind;
  const color =
    kind === "stalled" ? "var(--color-warning)" : "var(--color-danger)";
  return (
    <span style={{ color, fontWeight: 500 }}>{label}</span>
  );
}

// ── 4. Active background tasks ───────────────────────────────────────────────
//
// Pilot's scheduler tracks job intervals but not per-run state — it has
// no concept of "task X is running right now." Surfacing that requires
// a small task tracker (in-memory map keyed by task name with started_at
// / progress fields) wired into scheduler.RunNow / the recurring tick.
// Skipping for this PR; this rail is a placeholder so the layout doesn't
// drift if the tracker lands later.

function BackgroundTasksRail() {
  return (
    <Rail title="Active background tasks" count={0} collapsible defaultOpen={false}>
      <Empty>
        Per-run task state isn't tracked yet. Once the scheduler exposes it, this rail
        will show manual searches, library refreshes, and on-demand RSS scans in
        progress.
      </Empty>
    </Rail>
  );
}

// ── Page ─────────────────────────────────────────────────────────────────────

export default function ActivityPage() {
  const idx = useSeriesIndex();
  return (
    <div style={{ padding: 24, maxWidth: 980, display: "flex", flexDirection: "column", gap: 16 }}>
      <PageHeader
        title="Activity"
        description="What's happening right now — current downloads, recent imports, and items that need a look."
        action={
          <span
            style={{
              display: "inline-flex",
              alignItems: "center",
              gap: 6,
              fontSize: 12,
              color: "var(--color-text-muted)",
            }}
          >
            <Clock size={12} /> auto-refresh
          </span>
        }
      />

      <BackgroundTasksRail />
      <DownloadingRail idx={idx} />
      <NeedsAttentionRail idx={idx} />
      <RecentlyImportedRail idx={idx} />
    </div>
  );
}
