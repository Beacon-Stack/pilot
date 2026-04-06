import { useState } from "react";
import { ChevronDown, ChevronRight, Search, Trash2, CheckCircle2, Circle } from "lucide-react";
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
  onDeleteFile?: () => void;
}

export default function EpisodeRow({
  episode, file, seasonNumber, selected,
  onToggleSelect, onToggleMonitor, onSearch, onDeleteFile,
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
          cursor: "pointer",
        }}
        onClick={() => setExpanded(!expanded)}
      >
        {/* Checkbox */}
        <input
          type="checkbox"
          checked={selected}
          onClick={(e) => e.stopPropagation()}
          onChange={onToggleSelect}
          style={{ width: 14, height: 14, accentColor: "var(--color-accent)", cursor: "pointer", flexShrink: 0 }}
        />

        {/* Expand icon */}
        {expanded
          ? <ChevronDown size={14} style={{ color: "var(--color-text-muted)", flexShrink: 0 }} />
          : <ChevronRight size={14} style={{ color: "var(--color-text-muted)", flexShrink: 0 }} />
        }

        {/* Monitor indicator */}
        <button
          onClick={(e) => { e.stopPropagation(); onToggleMonitor(); }}
          style={{ background: "none", border: "none", cursor: "pointer", padding: 0, display: "flex", flexShrink: 0 }}
          title={ep.monitored ? "Monitored" : "Unmonitored"}
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

        {/* Quick search */}
        <button
          onClick={(e) => { e.stopPropagation(); onSearch(); }}
          style={{ background: "none", border: "none", cursor: "pointer", padding: 2, color: "var(--color-text-muted)", flexShrink: 0 }}
          title="Search"
        >
          <Search size={13} />
        </button>
      </div>

      {/* Expanded detail */}
      {expanded && (
        <div style={{ padding: "8px 12px 16px 72px", fontSize: 13 }}>
          {/* Overview */}
          {ep.overview && (
            <p style={{ margin: "0 0 12px", color: "var(--color-text-secondary)", lineHeight: 1.5 }}>
              {ep.overview}
            </p>
          )}

          {/* File info */}
          {file && (
            <div style={{ display: "flex", flexDirection: "column", gap: 4, marginBottom: 12 }}>
              <div style={{ color: "var(--color-text-muted)", fontSize: 12 }}>
                <span style={{ fontWeight: 500 }}>File:</span>{" "}
                <span style={{ fontFamily: "var(--font-family-mono)", fontSize: 11 }}>{file.path}</span>
              </div>
              <div style={{ color: "var(--color-text-muted)", fontSize: 12 }}>
                <span style={{ fontWeight: 500 }}>Quality:</span>{" "}
                {file.quality.source && `${file.quality.source} `}
                {file.quality.resolution && `${file.quality.resolution} `}
                {file.quality.codec && `· ${file.quality.codec} `}
                {file.quality.hdr && `· ${file.quality.hdr} `}
                · {formatBytes(file.size_bytes)}
              </div>
              <div style={{ color: "var(--color-text-muted)", fontSize: 12 }}>
                <span style={{ fontWeight: 500 }}>Imported:</span>{" "}
                {new Date(file.imported_at).toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" })}
              </div>
            </div>
          )}

          {/* Actions */}
          <div style={{ display: "flex", gap: 8 }}>
            <button onClick={onSearch} style={actionBtnStyle}>
              <Search size={13} /> Search
            </button>
            {file && onDeleteFile && (
              <button onClick={onDeleteFile} style={{ ...actionBtnStyle, color: "var(--color-danger)" }}>
                <Trash2 size={13} /> Delete File
              </button>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

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
