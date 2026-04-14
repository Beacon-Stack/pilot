import { useState, useMemo } from "react";
import { X, Download, Wifi, HardDrive, Loader2, AlertTriangle, ArrowUp, ArrowDown } from "lucide-react";
import { toast } from "sonner";
import { useSearchReleases, useGrabRelease } from "@/api/releases";
import Modal from "@beacon-shared/Modal";
import type { ReleaseResult } from "@/types";

type SortField = "seeds" | "size";
type SortDir = "asc" | "desc";

type PackFilter = "season" | "episodes" | "all";

// ── Helpers ───────────────────────────────────────────────────────────────────

function formatBytes(bytes: number): string {
  if (bytes === 0) return "—";
  if (bytes < 1_000_000) return `${(bytes / 1_000).toFixed(0)} KB`;
  if (bytes < 1_000_000_000) return `${(bytes / 1_000_000).toFixed(1)} MB`;
  return `${(bytes / 1_000_000_000).toFixed(2)} GB`;
}

function formatAge(days: number): string {
  if (days < 1) return "Today";
  if (days === 1) return "1d";
  if (days < 30) return `${Math.floor(days)}d`;
  const months = Math.floor(days / 30);
  if (months < 12) return `${months}mo`;
  return `${Math.floor(months / 12)}y`;
}

function seedHealth(seeds: number): { color: string; label: string } {
  if (seeds === 0) return { color: "var(--color-danger)", label: "Dead" };
  if (seeds <= 2) return { color: "var(--color-warning)", label: "Poor" };
  if (seeds <= 10) return { color: "var(--color-text-secondary)", label: "OK" };
  if (seeds <= 50) return { color: "var(--color-success)", label: "Good" };
  return { color: "var(--color-success)", label: "Great" };
}

// shouldCloseOnGrab returns the user's preference for whether the modal
// should close after a successful grab. Persisted in localStorage so users
// can flip the default (stay-open) without a backend setting. Mirrors the
// same key Prism's modal reads.
function shouldCloseOnGrab(): boolean {
  if (typeof window === "undefined") return false;
  return window.localStorage.getItem("manualSearchModal.closeOnGrab") === "true";
}

// ── Filter pill ───────────────────────────────────────────────────────────────

function FilterPill({
  active,
  onClick,
  label,
  count,
}: {
  active: boolean;
  onClick: () => void;
  label: string;
  count: number;
}) {
  return (
    <button
      onClick={onClick}
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: 6,
        padding: "5px 12px",
        borderRadius: 20,
        border: active
          ? "1px solid var(--color-accent)"
          : "1px solid var(--color-border-default)",
        background: active ? "var(--color-accent-muted)" : "transparent",
        color: active ? "var(--color-accent)" : "var(--color-text-secondary)",
        fontSize: 12,
        fontWeight: 500,
        cursor: "pointer",
        transition: "all 100ms ease",
      }}
    >
      {label}
      <span
        style={{
          fontSize: 10,
          fontWeight: 600,
          opacity: 0.7,
        }}
      >
        {count}
      </span>
    </button>
  );
}

// ── Pack type badge ───────────────────────────────────────────────────────────

function PackTypeBadge({ release }: { release: ReleaseResult }) {
  const pt = release.pack_type;
  if (pt === "season") {
    return (
      <span
        style={{
          display: "inline-flex",
          alignItems: "center",
          padding: "2px 7px",
          borderRadius: 4,
          fontSize: 10,
          fontWeight: 700,
          letterSpacing: "0.04em",
          textTransform: "uppercase",
          color: "var(--color-success)",
          background: "color-mix(in srgb, var(--color-success) 14%, transparent)",
          whiteSpace: "nowrap",
        }}
      >
        Season
      </span>
    );
  }
  if (pt === "multi_episode") {
    const n = release.episode_count ?? 0;
    return (
      <span
        style={{
          display: "inline-flex",
          alignItems: "center",
          padding: "2px 7px",
          borderRadius: 4,
          fontSize: 10,
          fontWeight: 700,
          letterSpacing: "0.04em",
          textTransform: "uppercase",
          color: "var(--color-warning)",
          background: "color-mix(in srgb, var(--color-warning) 14%, transparent)",
          whiteSpace: "nowrap",
        }}
      >
        {n > 0 ? `${n} eps` : "Multi"}
      </span>
    );
  }
  if (pt === "episode") {
    return (
      <span
        style={{
          display: "inline-flex",
          alignItems: "center",
          padding: "2px 7px",
          borderRadius: 4,
          fontSize: 10,
          fontWeight: 600,
          letterSpacing: "0.04em",
          textTransform: "uppercase",
          color: "var(--color-text-secondary)",
          background: "var(--color-bg-elevated)",
          whiteSpace: "nowrap",
        }}
      >
        Episode
      </span>
    );
  }
  return (
    <span
      style={{
        display: "inline-flex",
        alignItems: "center",
        padding: "2px 7px",
        borderRadius: 4,
        fontSize: 10,
        fontWeight: 600,
        color: "var(--color-text-muted)",
        background: "var(--color-bg-elevated)",
        whiteSpace: "nowrap",
      }}
    >
      —
    </span>
  );
}

// ── Quality badge ─────────────────────────────────────────────────────────────

function QualityBadge({ quality }: { quality: ReleaseResult["quality"] }) {
  const label = quality.name || quality.resolution || "Unknown";
  const isHD = ["1080p", "2160p"].includes(quality.resolution);
  return (
    <span
      style={{
        display: "inline-flex",
        alignItems: "center",
        padding: "2px 7px",
        borderRadius: 4,
        fontSize: 11,
        fontWeight: 600,
        letterSpacing: "0.03em",
        color: isHD ? "var(--color-accent)" : "var(--color-text-secondary)",
        background: isHD
          ? "color-mix(in srgb, var(--color-accent) 12%, transparent)"
          : "color-mix(in srgb, var(--color-text-muted) 10%, transparent)",
        whiteSpace: "nowrap",
      }}
    >
      {label}
    </span>
  );
}

// ── Release row ───────────────────────────────────────────────────────────────

interface ReleaseRowProps {
  release: ReleaseResult;
  onGrab: (release: ReleaseResult, override?: boolean) => void;
  isGrabbing: boolean;
}

function ReleaseRow({ release, onGrab, isGrabbing }: ReleaseRowProps) {
  const dead = release.seeds === 0;
  const health = seedHealth(release.seeds);
  const filtered = (release.filter_reasons?.length ?? 0) > 0;

  return (
    <tr
      style={{
        borderBottom: "1px solid var(--color-border-subtle)",
        // Grayed rows = filtered out by the backend safety filters
        // (min_seeders, previously stalled, ...). Dim them visibly so
        // the user knows they're not in the recommended set, but keep
        // them visible and grabbable via the override button.
        opacity: filtered ? 0.45 : dead ? 0.5 : 1,
        background: filtered ? "var(--color-bg-elevated)" : undefined,
      }}
    >
      {/* Title */}
      <td
        style={{
          padding: "10px 12px",
          fontSize: 12,
          color: "var(--color-text-primary)",
          maxWidth: 320,
        }}
      >
        <div
          style={{
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
          }}
          title={release.title}
        >
          {release.multi_indexer && (
            <span title="Found on multiple indexers — high confidence" style={{ color: "var(--color-warning)", marginRight: 4 }}>★</span>
          )}
          {release.title}
        </div>
        <div
          style={{
            fontSize: 11,
            color: "var(--color-text-muted)",
            marginTop: 2,
            display: "flex",
            alignItems: "center",
            gap: 6,
            flexWrap: "wrap",
          }}
        >
          {release.indexer}
          {release.low_confidence && !filtered && (
            <span style={{ display: "inline-flex", alignItems: "center", gap: 2, color: "var(--color-warning)", fontSize: 10 }} title="Low seed count — indexer data may be stale">
              <AlertTriangle size={10} /> Low seeds
            </span>
          )}
          {dead && !filtered && (
            <span style={{ display: "inline-flex", alignItems: "center", gap: 2, color: "var(--color-danger)", fontSize: 10 }}>
              <AlertTriangle size={10} /> No seeders
            </span>
          )}
          {filtered && release.filter_reasons?.map((reason, idx) => (
            <span
              key={idx}
              style={{
                display: "inline-flex",
                alignItems: "center",
                gap: 2,
                padding: "1px 5px",
                background: "color-mix(in srgb, var(--color-warning) 15%, transparent)",
                color: "var(--color-warning)",
                border: "1px solid color-mix(in srgb, var(--color-warning) 30%, transparent)",
                borderRadius: 3,
                fontSize: 10,
              }}
            >
              <AlertTriangle size={9} /> {reason}
            </span>
          ))}
        </div>
      </td>

      {/* Type */}
      <td style={{ padding: "10px 12px", width: 88 }}>
        <PackTypeBadge release={release} />
      </td>

      {/* Size */}
      <td
        style={{
          padding: "10px 12px",
          fontSize: 12,
          color: "var(--color-text-secondary)",
          whiteSpace: "nowrap",
          width: 80,
        }}
      >
        {formatBytes(release.size)}
      </td>

      {/* Quality */}
      <td style={{ padding: "10px 12px", width: 120 }}>
        <QualityBadge quality={release.quality} />
      </td>

      {/* Seeds */}
      <td
        style={{
          padding: "10px 12px",
          width: 72,
          fontSize: 12,
          fontWeight: 600,
          color: health.color,
          whiteSpace: "nowrap",
        }}
        title={`${release.seeds} seeders / ${release.peers} peers — ${health.label}`}
      >
        <span style={{ display: "inline-flex", alignItems: "center", gap: 4 }}>
          {release.protocol === "torrent"
            ? <Wifi size={12} strokeWidth={1.5} />
            : <HardDrive size={12} strokeWidth={1.5} />
          }
          {release.seeds}
        </span>
      </td>

      {/* Age */}
      <td
        style={{
          padding: "10px 12px",
          fontSize: 12,
          color: "var(--color-text-muted)",
          whiteSpace: "nowrap",
          width: 52,
        }}
      >
        {formatAge(release.age_days)}
      </td>

      {/* Grab */}
      <td style={{ padding: "10px 12px", width: filtered ? 96 : 52 }}>
        {filtered ? (
          <button
            onClick={() => onGrab(release, true)}
            disabled={isGrabbing}
            style={{
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              gap: 4,
              padding: "5px 10px",
              background: "var(--color-bg-surface)",
              border: "1px solid var(--color-border-default)",
              borderRadius: 5,
              cursor: isGrabbing ? "not-allowed" : "pointer",
              color: "var(--color-text-secondary)",
              fontSize: 11,
              fontWeight: 600,
              opacity: isGrabbing ? 0.6 : 1,
            }}
            title={`Override filter and grab anyway: ${release.filter_reasons?.join(", ") ?? "filtered"}`}
          >
            <Download size={12} strokeWidth={2} /> Override
          </button>
        ) : (
          <button
            onClick={() => onGrab(release)}
            disabled={isGrabbing || dead}
            style={{
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              padding: "5px 8px",
              background: dead ? "var(--color-bg-elevated)" : "var(--color-accent)",
              border: "none",
              borderRadius: 5,
              cursor: isGrabbing || dead ? "not-allowed" : "pointer",
              color: dead ? "var(--color-text-muted)" : "var(--color-accent-fg)",
              opacity: isGrabbing ? 0.6 : 1,
            }}
            title={dead ? "No seeders — this release cannot be downloaded" : "Grab this release"}
          >
            <Download size={13} strokeWidth={2} />
          </button>
        )}
      </td>
    </tr>
  );
}

// ── Modal ─────────────────────────────────────────────────────────────────────

interface ManualSearchModalProps {
  seriesId: string;
  seasonNumber?: number;
  episodeNumber?: number;
  onClose: () => void;
}

export default function ManualSearchModal({
  seriesId,
  seasonNumber,
  episodeNumber,
  onClose,
}: ManualSearchModalProps) {
  const { data: releases, isLoading, isError, error, refetch } = useSearchReleases(
    { seriesId, season: seasonNumber, episode: episodeNumber },
    true
  );
  const grab = useGrabRelease(seriesId);
  const [sort, setSort] = useState<{ field: SortField; dir: SortDir } | null>(null);

  // Pack-type filter. When the search is season-scoped, default to showing
  // only season packs — that's why the user clicked "Interactive Search" at
  // the season level. Episode-scoped searches default to all.
  const isSeasonScoped = seasonNumber !== undefined && episodeNumber === undefined;
  const [packFilter, setPackFilter] = useState<PackFilter>(isSeasonScoped ? "season" : "all");

  const sorted = useMemo(() => {
    if (!releases) return [];
    if (!sort) return releases;
    return [...releases].sort((a, b) => {
      const av = a[sort.field] ?? 0;
      const bv = b[sort.field] ?? 0;
      return sort.dir === "desc" ? bv - av : av - bv;
    });
  }, [releases, sort]);

  // Apply the pack-type filter. "season" keeps only season packs;
  // "episodes" keeps single + multi-episode releases; "all" keeps everything.
  const packCounts = useMemo(() => {
    const c = { season: 0, episodes: 0, all: sorted.length };
    for (const r of sorted) {
      if (r.pack_type === "season") c.season++;
      else if (r.pack_type === "episode" || r.pack_type === "multi_episode") c.episodes++;
    }
    return c;
  }, [sorted]);

  const packFiltered = useMemo(() => {
    if (packFilter === "all") return sorted;
    if (packFilter === "season") return sorted.filter((r) => r.pack_type === "season");
    return sorted.filter((r) => r.pack_type === "episode" || r.pack_type === "multi_episode");
  }, [sorted, packFilter]);

  // Split into active (top of list) and filtered (grayed, shown below).
  // Keeping filtered results visible with an override button prevents
  // "content loss traps" when false-positive blocklists hide a release
  // forever — the user can always see what was filtered and why.
  const active = packFiltered.filter((r) => !r.filter_reasons?.length);
  const filtered = packFiltered.filter((r) => r.filter_reasons && r.filter_reasons.length > 0);

  function toggleSort(field: SortField) {
    setSort((prev) => {
      if (prev?.field !== field) return { field, dir: "desc" };
      if (prev.dir === "desc") return { field, dir: "asc" };
      return null; // third click resets
    });
  }

  function handleGrab(release: ReleaseResult, override?: boolean) {
    grab.mutate({
      guid: release.guid,
      title: release.title,
      indexer_id: release.indexer_id,
      protocol: release.protocol,
      download_url: release.download_url,
      size: release.size,
      quality: release.quality,
      season_number: seasonNumber,
      override,
    }, {
      onSuccess: () => {
        const isSeasonPack = episodeNumber === undefined;
        if (override) {
          toast.warning(
            `Overriding filter — sent ${release.title.slice(0, 40)}... to download client. If it stalls, it'll be re-blocklisted.`,
          );
        } else {
          toast.success(isSeasonPack ? "Season pack sent to download client" : "Release sent to download client");
        }
        if (shouldCloseOnGrab()) {
          onClose();
        }
      },
      onError: (err) => toast.error(err.message),
    });
  }

  const title = episodeNumber !== undefined
    ? `Search — S${String(seasonNumber).padStart(2, "0")}E${String(episodeNumber).padStart(2, "0")}`
    : seasonNumber !== undefined
      ? `Search — Season ${seasonNumber}`
      : "Search Releases";

  const liveCount = releases?.filter((r) => r.seeds > 0).length ?? 0;

  const sortIcon = (field: SortField) => {
    if (sort?.field !== field) return null;
    return sort.dir === "desc"
      ? <ArrowDown size={10} strokeWidth={2.5} />
      : <ArrowUp size={10} strokeWidth={2.5} />;
  };

  const thStyle: React.CSSProperties = {
    textAlign: "left",
    padding: "8px 12px",
    fontSize: 11,
    fontWeight: 600,
    letterSpacing: "0.06em",
    textTransform: "uppercase",
    color: "var(--color-text-muted)",
    borderBottom: "1px solid var(--color-border-subtle)",
    position: "sticky",
    top: 0,
    background: "var(--color-bg-surface)",
  };

  const sortableThStyle = (field: SortField): React.CSSProperties => ({
    ...thStyle,
    cursor: "pointer",
    userSelect: "none",
    color: sort?.field === field ? "var(--color-accent)" : "var(--color-text-muted)",
  });

  return (
    <Modal onClose={onClose} width={1040} maxHeight="calc(100vh - 64px)">
      {/* Header */}
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          padding: "18px 20px",
          borderBottom: "1px solid var(--color-border-subtle)",
          flexShrink: 0,
        }}
      >
        <div>
          <h2 style={{ margin: 0, fontSize: 16, fontWeight: 600, color: "var(--color-text-primary)" }}>
            {title}
          </h2>
          {!isLoading && releases && releases.length > 0 && (
            <div style={{ fontSize: 12, color: "var(--color-text-muted)", marginTop: 2 }}>
              {releases.length} results · {liveCount} with seeders
            </div>
          )}
        </div>
        <button
          onClick={onClose}
          style={{
            background: "none",
            border: "none",
            cursor: "pointer",
            color: "var(--color-text-muted)",
            display: "flex",
            padding: 4,
          }}
        >
          <X size={18} />
        </button>
      </div>

      {/* Pack-type filter pills */}
      {!isLoading && !isError && releases && releases.length > 0 && (
        <div
          style={{
            display: "flex",
            gap: 6,
            padding: "10px 16px",
            borderBottom: "1px solid var(--color-border-subtle)",
            flexShrink: 0,
            background: "var(--color-bg-surface)",
          }}
        >
          <FilterPill
            active={packFilter === "season"}
            onClick={() => setPackFilter("season")}
            label="Season Packs"
            count={packCounts.season}
          />
          <FilterPill
            active={packFilter === "episodes"}
            onClick={() => setPackFilter("episodes")}
            label="Episodes"
            count={packCounts.episodes}
          />
          <FilterPill
            active={packFilter === "all"}
            onClick={() => setPackFilter("all")}
            label="All"
            count={packCounts.all}
          />
        </div>
      )}

      {/* Body */}
      <div style={{ overflow: "auto", flex: 1 }}>
        {/* Loading */}
        {isLoading && (
          <div
            style={{
              display: "flex",
              flexDirection: "column",
              alignItems: "center",
              justifyContent: "center",
              padding: "56px 24px",
              gap: 12,
              color: "var(--color-text-muted)",
              fontSize: 13,
            }}
          >
            <Loader2
              size={28}
              strokeWidth={1.5}
              style={{
                color: "var(--color-accent)",
                animation: "spin 1s linear infinite",
              }}
            />
            Searching indexers...
          </div>
        )}

        {/* Error */}
        {isError && (
          <div style={{ margin: 20 }}>
            <div
              style={{
                padding: 16,
                background: "color-mix(in srgb, var(--color-danger) 10%, transparent)",
                border: "1px solid color-mix(in srgb, var(--color-danger) 30%, transparent)",
                borderRadius: 8,
                color: "var(--color-danger)",
                fontSize: 13,
              }}
            >
              {(error as Error).message ?? "Search failed. Check that indexers are configured and reachable."}
            </div>
            <div style={{ display: "flex", justifyContent: "center", marginTop: 12 }}>
              <button
                onClick={() => refetch()}
                style={{
                  background: "var(--color-bg-elevated)",
                  border: "1px solid var(--color-border-default)",
                  borderRadius: 6,
                  padding: "6px 14px",
                  fontSize: 12,
                  fontWeight: 500,
                  color: "var(--color-text-secondary)",
                  cursor: "pointer",
                }}
              >
                Retry
              </button>
            </div>
          </div>
        )}

        {/* Empty */}
        {!isLoading && !isError && releases && releases.length === 0 && (
          <div
            style={{
              display: "flex",
              flexDirection: "column",
              alignItems: "center",
              padding: "56px 24px",
              gap: 12,
              fontSize: 14,
              color: "var(--color-text-muted)",
            }}
          >
            No releases found.
            <button
              onClick={() => refetch()}
              style={{
                background: "var(--color-bg-elevated)",
                border: "1px solid var(--color-border-default)",
                borderRadius: 6,
                padding: "6px 14px",
                fontSize: 12,
                fontWeight: 500,
                color: "var(--color-text-secondary)",
                cursor: "pointer",
              }}
            >
              Search Again
            </button>
          </div>
        )}

        {/* Empty filter */}
        {!isLoading && !isError && releases && releases.length > 0 && packFiltered.length === 0 && (
          <div
            style={{
              display: "flex",
              flexDirection: "column",
              alignItems: "center",
              padding: "48px 24px",
              gap: 10,
              fontSize: 13,
              color: "var(--color-text-muted)",
            }}
          >
            <div>No {packFilter === "season" ? "season packs" : packFilter === "episodes" ? "episodes" : "results"} match this filter.</div>
            <button
              onClick={() => setPackFilter("all")}
              style={{
                background: "var(--color-bg-elevated)",
                border: "1px solid var(--color-border-default)",
                borderRadius: 6,
                padding: "6px 14px",
                fontSize: 12,
                fontWeight: 500,
                color: "var(--color-text-secondary)",
                cursor: "pointer",
              }}
            >
              Show all ({releases.length})
            </button>
          </div>
        )}

        {/* Results table */}
        {!isLoading && !isError && releases && releases.length > 0 && packFiltered.length > 0 && (
          <table
            style={{
              width: "100%",
              borderCollapse: "collapse",
              fontSize: 13,
            }}
          >
            <thead>
              <tr>
                <th style={thStyle}>Release</th>
                <th style={thStyle}>Type</th>
                <th style={sortableThStyle("size")} onClick={() => toggleSort("size")}>
                  <span style={{ display: "inline-flex", alignItems: "center", gap: 4 }}>Size {sortIcon("size")}</span>
                </th>
                <th style={thStyle}>Quality</th>
                <th style={sortableThStyle("seeds")} onClick={() => toggleSort("seeds")}>
                  <span style={{ display: "inline-flex", alignItems: "center", gap: 4 }}>Seeds (indexer) {sortIcon("seeds")}</span>
                </th>
                <th style={thStyle}>Age</th>
                <th style={thStyle}></th>
              </tr>
            </thead>
            <tbody>
              {active.map((release) => (
                <ReleaseRow
                  key={release.guid}
                  release={release}
                  onGrab={handleGrab}
                  isGrabbing={grab.isPending}
                />
              ))}
              {filtered.length > 0 && (
                <tr>
                  <td
                    colSpan={7}
                    style={{
                      padding: "12px 16px 6px",
                      fontSize: 11,
                      fontWeight: 600,
                      letterSpacing: "0.06em",
                      textTransform: "uppercase",
                      color: "var(--color-text-muted)",
                      borderTop: "1px solid var(--color-border-subtle)",
                      background: "var(--color-bg-elevated)",
                    }}
                  >
                    Filtered ({filtered.length}) — below min_seeders, outside quality profile, or previously stalled. Click Override to grab anyway.
                  </td>
                </tr>
              )}
              {filtered.map((release) => (
                <ReleaseRow
                  key={release.guid}
                  release={release}
                  onGrab={handleGrab}
                  isGrabbing={grab.isPending}
                />
              ))}
            </tbody>
          </table>
        )}
      </div>
    </Modal>
  );
}
