import { CheckCircle2, Circle, Loader2, Search, Zap } from "lucide-react";
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
  onInteractiveSearch: () => void;
  onAutoSearchSeason: () => void;
  isAutoSearching?: boolean;
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
  filter, onFilterChange, onToggleMonitor, onInteractiveSearch, onAutoSearchSeason,
  isAutoSearching = false,
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

        {/* Interactive Search — opens a modal showing all releases for the season */}
        <button
          onClick={onInteractiveSearch}
          title="Browse and pick a release for this season"
          style={{
            display: "flex", alignItems: "center", gap: 5,
            padding: "4px 10px", borderRadius: 6,
            border: "1px solid var(--color-accent)",
            background: "var(--color-accent-muted)", cursor: "pointer",
            fontSize: 12, fontWeight: 500, color: "var(--color-accent)",
          }}
        >
          <Search size={13} /> Interactive Search
        </button>

        {/* Auto Search — grabs the best matching release without prompting */}
        <button
          onClick={onAutoSearchSeason}
          disabled={isAutoSearching}
          title="Automatically grab the best-scored season pack"
          style={{
            display: "flex", alignItems: "center", gap: 5,
            padding: "4px 10px", borderRadius: 6,
            border: "1px solid var(--color-border-default)",
            background: "none",
            cursor: isAutoSearching ? "wait" : "pointer",
            fontSize: 12, fontWeight: 500,
            color: "var(--color-text-secondary)",
            opacity: isAutoSearching ? 0.6 : 1,
          }}
        >
          {isAutoSearching
            ? <Loader2 size={13} style={{ animation: "spin 1s linear infinite" }} />
            : <Zap size={13} />}
          {isAutoSearching ? "Searching…" : "Auto Search"}
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
