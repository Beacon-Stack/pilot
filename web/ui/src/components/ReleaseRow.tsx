import { Download, Wifi, HardDrive, AlertTriangle, History } from "lucide-react";
import type { ReleaseResult } from "@/types";
import { filterReasonLabel } from "@/api/language";

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

// Quality badge

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

// Release row

interface ReleaseRowProps {
  release: ReleaseResult;
  onGrab: (release: ReleaseResult, override?: boolean) => void;
  isGrabbing: boolean;
}

export default function ReleaseRow({ release, onGrab, isGrabbing }: ReleaseRowProps) {
  const dead = release.seeds === 0;
  const health = seedHealth(release.seeds);
  const filtered = (release.filter_reasons?.length ?? 0) > 0;
  const alreadyGrabbed = !!release.already_grabbed_at;
  const grabbedAgeLabel = alreadyGrabbed
    ? formatAge(Math.max(0, (Date.now() - new Date(release.already_grabbed_at!).getTime()) / 86400000))
    : "";
  // formatAge returns "Today" for <1 day and N-with-unit ("3d", "2mo")
  // otherwise. "Today ago" reads wrong; the "N ago" forms read right.
  const grabbedAgeText = grabbedAgeLabel === "Today" ? "today" : `${grabbedAgeLabel} ago`;

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
          {alreadyGrabbed && (
            <span
              title={`Previously grabbed ${grabbedAgeText} — status: ${release.already_grabbed_status ?? "unknown"}`}
              style={{
                display: "inline-flex",
                alignItems: "center",
                gap: 3,
                padding: "1px 5px",
                background: "color-mix(in srgb, var(--color-warning) 15%, transparent)",
                color: "var(--color-warning)",
                border: "1px solid color-mix(in srgb, var(--color-warning) 30%, transparent)",
                borderRadius: 3,
                fontSize: 10,
                fontWeight: 500,
              }}
            >
              <History size={10} /> Already grabbed {grabbedAgeText}
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
              <AlertTriangle size={9} /> {filterReasonLabel(reason, release)}
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

// Modal
