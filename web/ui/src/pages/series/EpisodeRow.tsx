import { useState } from "react";
import { ChevronDown, Search, Trash2, CheckCircle2, Circle, Zap, Info } from "lucide-react";
import { formatBytes } from "@/lib/utils";
import type { Episode, EpisodeFile } from "@/types";

interface Props {
  episode: Episode;
  file?: EpisodeFile;
  seasonNumber: number;
  selected: boolean;
  onToggleSelect: () => void;
  onToggleMonitor: () => void;
  onSearch: () => void;
  onAutoSearch: () => void;
  onDeleteFile?: () => void;
}

export default function EpisodeRow({
  episode, file, seasonNumber, selected,
  onToggleSelect, onToggleMonitor, onSearch, onAutoSearch, onDeleteFile,
}: Props) {
  const [expanded, setExpanded] = useState(false);
  const ep = episode;
  const aired = ep.air_date ? new Date(ep.air_date) <= new Date() : false;
  const epNum = `${seasonNumber}x${String(ep.episode_number).padStart(2, "0")}`;

  return (
    <div
      style={{
        borderBottom: "1px solid var(--color-border-subtle)",
        background: selected ? "var(--color-accent-muted)" : "transparent",
        transition: "background 100ms ease",
      }}
    >
      {/* Collapsed row */}
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: 10,
          padding: "8px 12px",
        }}
      >
        {/* Checkbox */}
        <input
          type="checkbox"
          checked={selected}
          onChange={onToggleSelect}
          style={{ width: 14, height: 14, accentColor: "var(--color-accent)", cursor: "pointer", flexShrink: 0 }}
        />

        {/* Monitor indicator */}
        <button
          onClick={onToggleMonitor}
          style={{ background: "none", border: "none", cursor: "pointer", padding: 0, display: "flex", flexShrink: 0 }}
          title={ep.monitored ? "Monitored — click to unmonitor" : "Unmonitored — click to monitor"}
        >
          {ep.monitored
            ? <CheckCircle2 size={14} style={{ color: "var(--color-accent)" }} />
            : <Circle size={14} style={{ color: "var(--color-text-muted)" }} />
          }
        </button>

        {/* Episode number */}
        <span style={{ fontSize: 13, color: "var(--color-text-muted)", fontFamily: "var(--font-family-mono)", width: 48, flexShrink: 0 }}>
          {epNum}
        </span>

        {/* Title */}
        <span style={{ fontSize: 13, color: "var(--color-text-primary)", fontWeight: 500, flex: 1, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
          {ep.title || "TBA"}
        </span>

        {/* Air date */}
        <span style={{ fontSize: 12, color: "var(--color-text-muted)", width: 90, flexShrink: 0, textAlign: "right" }}>
          {ep.air_date
            ? new Date(ep.air_date).toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" })
            : "TBA"
          }
        </span>

        {/* Status badge */}
        <div style={{ width: 160, flexShrink: 0, textAlign: "right" }}>
          <StatusBadge episode={ep} file={file} aired={aired} />
        </div>

        {/* Action buttons */}
        <div style={{ display: "flex", alignItems: "center", gap: 2, flexShrink: 0 }}>
          {/* Expand — episode details */}
          <button
            onClick={() => setExpanded(!expanded)}
            style={iconBtn}
            title="Episode details"
          >
            {expanded
              ? <ChevronDown size={15} style={{ color: "var(--color-text-muted)" }} />
              : <Info size={14} style={{ color: "var(--color-text-muted)" }} />
            }
          </button>

          {/* Auto search — grabs the best match */}
          <button
            onClick={onAutoSearch}
            style={iconBtn}
            title="Auto search — grab the best match"
          >
            <Zap size={14} style={{ color: "var(--color-accent)" }} />
          </button>

          {/* Interactive search — opens the search modal */}
          <button
            onClick={onSearch}
            style={iconBtn}
            title="Interactive search — browse and pick a release"
          >
            <Search size={14} style={{ color: "var(--color-text-secondary)" }} />
          </button>
        </div>
      </div>

      {/* Expanded detail */}
      {expanded && (
        <div style={{ padding: "10px 12px 16px 52px", fontSize: 13 }}>
          <div style={{ display: "flex", gap: 16 }}>
            {/* Episode still image */}
            {ep.still_path && (
              <img
                src={`https://image.tmdb.org/t/p/w300${ep.still_path}`}
                alt={ep.title}
                style={{
                  width: 200, borderRadius: 6, flexShrink: 0,
                  aspectRatio: "16/9", objectFit: "cover",
                  background: "var(--color-bg-elevated)",
                }}
              />
            )}

            {/* Episode metadata */}
            <div style={{ flex: 1, minWidth: 0 }}>
              {/* Meta row */}
              <div style={{ display: "flex", flexWrap: "wrap", gap: 12, marginBottom: 8, fontSize: 12, color: "var(--color-text-muted)" }}>
                {ep.air_date && (
                  <span>
                    {new Date(ep.air_date).toLocaleDateString(undefined, { weekday: "short", year: "numeric", month: "long", day: "numeric" })}
                  </span>
                )}
                {ep.runtime_minutes != null && ep.runtime_minutes > 0 && (
                  <span>{ep.runtime_minutes} min</span>
                )}
              </div>

              {/* Overview */}
              {ep.overview && (
                <p style={{ margin: "0 0 12px", color: "var(--color-text-secondary)", lineHeight: 1.55, fontSize: 13 }}>
                  {ep.overview}
                </p>
              )}

              {/* File info */}
              {file && (
                <div style={{
                  display: "flex", flexDirection: "column", gap: 3, marginBottom: 12,
                  padding: "8px 10px", borderRadius: 6,
                  background: "var(--color-bg-elevated)",
                  border: "1px solid var(--color-border-subtle)",
                }}>
                  <div style={{ color: "var(--color-text-muted)", fontSize: 12, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                    <span style={{ fontWeight: 500 }}>File:</span>{" "}
                    <span style={{ fontFamily: "var(--font-family-mono)", fontSize: 11 }}>{file.path.split("/").pop()}</span>
                  </div>
                  <div style={{ color: "var(--color-text-muted)", fontSize: 12 }}>
                    <span style={{ fontWeight: 500 }}>Quality:</span>{" "}
                    {[file.quality?.source, file.quality?.resolution, file.quality?.codec].filter(Boolean).join(" · ") || "Unknown"}
                    {" · "}{formatBytes(file.size_bytes)}
                  </div>
                </div>
              )}

              {/* Actions */}
              <div style={{ display: "flex", gap: 8 }}>
                {file && onDeleteFile && (
                  <button onClick={onDeleteFile} style={{ ...actionBtnStyle, color: "var(--color-danger)" }}>
                    <Trash2 size={13} /> Delete File
                  </button>
                )}
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

const iconBtn: React.CSSProperties = {
  background: "none",
  border: "none",
  cursor: "pointer",
  padding: 6,
  borderRadius: 4,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
};

const actionBtnStyle: React.CSSProperties = {
  display: "flex", alignItems: "center", gap: 4,
  padding: "4px 10px", borderRadius: 5,
  border: "1px solid var(--color-border-default)",
  background: "none", cursor: "pointer",
  fontSize: 12, fontWeight: 500, color: "var(--color-text-secondary)",
};

function StatusBadge({ episode, file, aired }: { episode: Episode; file?: EpisodeFile; aired: boolean }) {
  // Downloaded
  if (episode.has_file && file) {
    const qual = [file.quality.resolution, file.quality.codec].filter(Boolean).join(" ");
    return (
      <span style={{
        display: "inline-flex", alignItems: "center", gap: 4,
        padding: "2px 8px", borderRadius: 4, fontSize: 11, fontWeight: 500,
        background: "color-mix(in srgb, var(--color-success) 12%, transparent)",
        color: "var(--color-success)",
      }}>
        {qual || "Downloaded"} · {formatBytes(file.size_bytes)}
      </span>
    );
  }

  // Has file but no file info
  if (episode.has_file) {
    return (
      <span style={{ fontSize: 11, color: "var(--color-success)", fontWeight: 500 }}>Downloaded</span>
    );
  }

  // Unmonitored
  if (!episode.monitored) {
    return (
      <span style={{ fontSize: 11, color: "var(--color-text-muted)" }}>Unmonitored</span>
    );
  }

  // Unaired
  if (!aired) {
    return (
      <span style={{ fontSize: 11, color: "var(--color-text-muted)" }}>
        {episode.air_date ? `Unaired · ${new Date(episode.air_date).toLocaleDateString(undefined, { month: "short", day: "numeric" })}` : "TBA"}
      </span>
    );
  }

  // Missing (aired + monitored + no file)
  return (
    <span style={{
      display: "inline-flex", alignItems: "center",
      padding: "2px 8px", borderRadius: 4, fontSize: 11, fontWeight: 500,
      background: "color-mix(in srgb, var(--color-danger) 10%, transparent)",
      color: "var(--color-danger)",
    }}>
      Missing
    </span>
  );
}
