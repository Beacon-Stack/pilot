import { X, Download, Wifi, HardDrive, Loader2 } from "lucide-react";
import { toast } from "sonner";
import { useSearchReleases, useGrabRelease } from "@/api/releases";
import Modal from "@/components/Modal";
import type { ReleaseResult } from "@/types";

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
  if (days < 30) return `${days}d`;
  const months = Math.floor(days / 30);
  if (months < 12) return `${months}mo`;
  return `${Math.floor(months / 12)}y`;
}

// ── Quality badge ─────────────────────────────────────────────────────────────

function QualityBadge({ quality }: { quality: ReleaseResult["quality"] }) {
  const label = quality.name || quality.resolution || "Unknown";
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
        color: "var(--color-accent)",
        background: "color-mix(in srgb, var(--color-accent) 12%, transparent)",
        whiteSpace: "nowrap",
      }}
    >
      {label}
    </span>
  );
}

// ── Protocol icon ─────────────────────────────────────────────────────────────

function ProtocolIcon({ protocol, seeders }: { protocol: string; seeders: number }) {
  const isTorrent = protocol === "torrent";
  return (
    <span
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: 4,
        fontSize: 12,
        color: isTorrent ? "var(--color-warning)" : "var(--color-text-secondary)",
        whiteSpace: "nowrap",
      }}
      title={isTorrent ? `Torrent — ${seeders} seeders` : "Usenet"}
    >
      {isTorrent ? <Wifi size={13} strokeWidth={1.5} /> : <HardDrive size={13} strokeWidth={1.5} />}
      {isTorrent ? seeders : "NZB"}
    </span>
  );
}

// ── Release row ───────────────────────────────────────────────────────────────

interface ReleaseRowProps {
  release: ReleaseResult;
  onGrab: (guid: string) => void;
  isGrabbing: boolean;
}

function ReleaseRow({ release, onGrab, isGrabbing }: ReleaseRowProps) {
  return (
    <tr
      style={{
        borderBottom: "1px solid var(--color-border-subtle)",
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
          }}
        >
          {release.indexer_name}
        </div>
      </td>

      {/* Size */}
      <td
        style={{
          padding: "10px 12px",
          fontSize: 12,
          color: "var(--color-text-secondary)",
          whiteSpace: "nowrap",
          width: 72,
        }}
      >
        {formatBytes(release.size)}
      </td>

      {/* Quality */}
      <td style={{ padding: "10px 12px", width: 96 }}>
        <QualityBadge quality={release.quality} />
      </td>

      {/* Protocol */}
      <td style={{ padding: "10px 12px", width: 72 }}>
        <ProtocolIcon protocol={release.protocol} seeders={release.seeders} />
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
          onClick={() => onGrab(release.guid)}
          disabled={isGrabbing}
          style={{
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            padding: "5px 8px",
            background: "var(--color-accent)",
            border: "none",
            borderRadius: 5,
            cursor: isGrabbing ? "not-allowed" : "pointer",
            color: "var(--color-accent-fg)",
            opacity: isGrabbing ? 0.6 : 1,
          }}
          title="Grab this release"
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

  function handleGrab(guid: string) {
    grab.mutate(guid, {
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

  const colHeaders = ["Release", "Size", "Quality", "Protocol", "Age", ""];

  return (
    <Modal onClose={onClose} width={760} maxHeight="calc(100vh - 64px)">
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
        <h2 style={{ margin: 0, fontSize: 16, fontWeight: 600, color: "var(--color-text-primary)" }}>
          {title}
        </h2>
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
                {colHeaders.map((h) => (
                  <th
                    key={h}
                    style={{
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
                    }}
                  >
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {releases.map((release) => (
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
