import { useState, useMemo } from "react";
import { X, Download, Wifi, HardDrive, Loader2, AlertTriangle, ArrowUp, ArrowDown } from "lucide-react";
import { toast } from "sonner";
import { useSearchReleases, useGrabRelease } from "@/api/releases";
import Modal from "@/components/Modal";
import type { ReleaseResult } from "@/types";

type SortField = "seeds" | "size";
type SortDir = "asc" | "desc";

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
  onGrab: (release: ReleaseResult) => void;
  isGrabbing: boolean;
}

function ReleaseRow({ release, onGrab, isGrabbing }: ReleaseRowProps) {
  const dead = release.seeds === 0;
  const health = seedHealth(release.seeds);

  return (
    <tr
      style={{
        borderBottom: "1px solid var(--color-border-subtle)",
        opacity: dead ? 0.5 : 1,
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
          }}
        >
          {release.indexer}
          {dead && (
            <span style={{ display: "inline-flex", alignItems: "center", gap: 2, color: "var(--color-danger)", fontSize: 10 }}>
              <AlertTriangle size={10} /> No seeders
            </span>
          )}
        </div>
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
      <td style={{ padding: "10px 12px", width: 52 }}>
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
  const { data: releases, isLoading, isError, error } = useSearchReleases(
    { seriesId, season: seasonNumber, episode: episodeNumber },
    true
  );
  const grab = useGrabRelease(seriesId);
  const [sort, setSort] = useState<{ field: SortField; dir: SortDir } | null>(null);

  const sorted = useMemo(() => {
    if (!releases) return [];
    if (!sort) return releases;
    return [...releases].sort((a, b) => {
      const av = a[sort.field] ?? 0;
      const bv = b[sort.field] ?? 0;
      return sort.dir === "desc" ? bv - av : av - bv;
    });
  }, [releases, sort]);

  function toggleSort(field: SortField) {
    setSort((prev) => {
      if (prev?.field !== field) return { field, dir: "desc" };
      if (prev.dir === "desc") return { field, dir: "asc" };
      return null; // third click resets
    });
  }

  function handleGrab(release: ReleaseResult) {
    grab.mutate({
      guid: release.guid,
      title: release.title,
      indexer_id: release.indexer_id,
      protocol: release.protocol,
      download_url: release.download_url,
      size: release.size,
      quality: release.quality,
    }, {
      onSuccess: () => {
        toast.success("Release sent to download client");
        onClose();
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
    <Modal onClose={onClose} width={800} maxHeight="calc(100vh - 64px)">
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

      {/* Body */}
      <div style={{ overflowY: "auto", flex: 1 }}>
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
          <div
            style={{
              margin: 20,
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
        )}

        {/* Empty */}
        {!isLoading && !isError && releases && releases.length === 0 && (
          <div
            style={{
              textAlign: "center",
              padding: "56px 24px",
              fontSize: 14,
              color: "var(--color-text-muted)",
            }}
          >
            No releases found.
          </div>
        )}

        {/* Results table */}
        {!isLoading && !isError && releases && releases.length > 0 && (
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
                <th style={sortableThStyle("size")} onClick={() => toggleSort("size")}>
                  <span style={{ display: "inline-flex", alignItems: "center", gap: 4 }}>Size {sortIcon("size")}</span>
                </th>
                <th style={thStyle}>Quality</th>
                <th style={sortableThStyle("seeds")} onClick={() => toggleSort("seeds")}>
                  <span style={{ display: "inline-flex", alignItems: "center", gap: 4 }}>Seeds {sortIcon("seeds")}</span>
                </th>
                <th style={thStyle}>Age</th>
                <th style={thStyle}></th>
              </tr>
            </thead>
            <tbody>
              {sorted.map((release) => (
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
