import { CheckCircle2, Circle, Search } from "lucide-react";
import type { Season } from "@/types";

type EpisodeFilter = "all" | "downloaded" | "missing" | "unaired" | "unmonitored";

interface Props {
  season: Season;
  episodeCount: number;
  downloadedCount: number;
  totalSize: string;
  filter: EpisodeFilter;
  onFilterChange: (filter: EpisodeFilter) => void;
  onToggleMonitor: () => void;
  onSearchMissing: () => void;
}

const FILTERS: { value: EpisodeFilter; label: string }[] = [
  { value: "all", label: "All" },
  { value: "downloaded", label: "Downloaded" },
  { value: "missing", label: "Missing" },
  { value: "unaired", label: "Unaired" },
  { value: "unmonitored", label: "Unmonitored" },
];

export type { EpisodeFilter };

export default function SeasonHeader({
  season, episodeCount, downloadedCount, totalSize,
  filter, onFilterChange, onToggleMonitor, onSearchMissing,
}: Props) {
  const label = season.season_number === 0 ? "Specials" : `Season ${season.season_number}`;
  const progress = episodeCount > 0 ? (downloadedCount / episodeCount) * 100 : 0;

  return (
    <div style={{ marginBottom: 16 }}>
      {/* Stats row */}
      <div style={{ display: "flex", alignItems: "center", gap: 12, marginBottom: 10 }}>
        <span style={{ fontSize: 16, fontWeight: 600, color: "var(--color-text-primary)" }}>{label}</span>
        <span style={{ fontSize: 13, color: "var(--color-text-muted)" }}>
          {episodeCount} episode{episodeCount !== 1 ? "s" : ""}
          {downloadedCount > 0 && ` · ${downloadedCount} downloaded`}
          {totalSize && ` · ${totalSize}`}
        </span>
        <div style={{ flex: 1 }} />

        {/* Search Missing */}
        <button
          onClick={onSearchMissing}
          style={{
            display: "flex", alignItems: "center", gap: 5,
            padding: "4px 10px", borderRadius: 6,
            border: "1px solid var(--color-border-default)",
            background: "none", cursor: "pointer",
            fontSize: 12, fontWeight: 500, color: "var(--color-text-secondary)",
          }}
        >
          <Search size={13} /> Search Missing
        </button>

        {/* Monitor toggle */}
        <button
          onClick={onToggleMonitor}
          style={{
            display: "flex", alignItems: "center", gap: 5,
            padding: "4px 10px", borderRadius: 6,
            border: "1px solid var(--color-border-default)",
            background: "none", cursor: "pointer",
            fontSize: 12, fontWeight: 500,
            color: season.monitored ? "var(--color-accent)" : "var(--color-text-muted)",
          }}
        >
          {season.monitored
            ? <><CheckCircle2 size={14} /> Monitored</>
            : <><Circle size={14} /> Unmonitored</>}
        </button>
      </div>

      {/* Progress bar */}
      {episodeCount > 0 && (
        <div style={{ height: 3, borderRadius: 2, background: "var(--color-bg-subtle)", marginBottom: 12 }}>
          <div style={{
            height: "100%", borderRadius: 2,
            width: `${progress}%`,
            background: progress === 100 ? "var(--color-success)" : "var(--color-accent)",
            transition: "width 300ms ease",
          }} />
        </div>
      )}

      {/* Filter chips */}
      <div style={{ display: "flex", gap: 4 }}>
        {FILTERS.map((f) => (
          <button
            key={f.value}
            onClick={() => onFilterChange(f.value)}
            style={{
              padding: "3px 10px", borderRadius: 4, fontSize: 12, fontWeight: 500,
              cursor: "pointer",
              border: filter === f.value ? "1px solid var(--color-accent)" : "1px solid var(--color-border-default)",
              background: filter === f.value ? "var(--color-accent-muted)" : "transparent",
              color: filter === f.value ? "var(--color-accent)" : "var(--color-text-secondary)",
              transition: "all 100ms ease",
            }}
          >
            {f.label}
          </button>
        ))}
      </div>
    </div>
  );
}
