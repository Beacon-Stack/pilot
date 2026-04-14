import { CheckCircle2, Circle } from "lucide-react";
import { formatBytes } from "@/lib/utils";
import type { Season } from "@/types";

interface SeasonSummary {
  season: Season;
  totalEpisodes: number;
  downloadedEpisodes: number;
  totalSize: number;
}

interface Props {
  summaries: SeasonSummary[];
  onSelectSeason: (seasonNumber: number) => void;
  onToggleMonitor: (seasonId: string, monitored: boolean) => void;
}

export type { SeasonSummary };

export function buildSeasonSummaries(seasons: Season[]): SeasonSummary[] {
  return seasons.map((s) => ({
    season: s,
    totalEpisodes: s.episode_count,
    downloadedEpisodes: s.episode_file_count,
    totalSize: s.total_size_bytes,
  }));
}

export default function AllSeasonsView({ summaries, onSelectSeason, onToggleMonitor }: Props) {
  if (!summaries.length) {
    return <div style={{ fontSize: 13, color: "var(--color-text-muted)", padding: 20 }}>No seasons.</div>;
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
      {summaries.map((s) => {
        const progress = s.totalEpisodes > 0 ? (s.downloadedEpisodes / s.totalEpisodes) * 100 : 0;
        const label = s.season.season_number === 0 ? "Specials" : `Season ${s.season.season_number}`;

        return (
          <div
            key={s.season.id}
            onClick={() => onSelectSeason(s.season.season_number)}
            style={{
              display: "flex",
              alignItems: "center",
              gap: 12,
              padding: "10px 14px",
              borderRadius: 6,
              cursor: "pointer",
              transition: "background 100ms ease",
            }}
            onMouseEnter={(e) => { e.currentTarget.style.background = "var(--color-bg-elevated)"; }}
            onMouseLeave={(e) => { e.currentTarget.style.background = "transparent"; }}
          >
            {/* Season name */}
            <span style={{ fontSize: 14, fontWeight: 600, color: "var(--color-text-primary)", width: 110, flexShrink: 0 }}>
              {label}
            </span>

            {/* Progress bar */}
            <div style={{ flex: 1, maxWidth: 200 }}>
              <div style={{ height: 4, borderRadius: 2, background: "var(--color-bg-subtle)" }}>
                <div style={{
                  height: "100%", borderRadius: 2,
                  width: `${progress}%`,
                  background: progress === 100 ? "var(--color-success)" : "var(--color-accent)",
                  transition: "width 300ms ease",
                }} />
              </div>
            </div>

            {/* Stats */}
            <span style={{ fontSize: 13, color: "var(--color-text-secondary)", width: 120, textAlign: "right" }}>
              {s.downloadedEpisodes}/{s.totalEpisodes} episodes
            </span>

            {s.totalSize > 0 && (
              <span style={{ fontSize: 12, color: "var(--color-text-muted)", fontFamily: "var(--font-family-mono)", width: 80, textAlign: "right" }}>
                {formatBytes(s.totalSize)}
              </span>
            )}

            {/* Monitor */}
            <button
              onClick={(e) => { e.stopPropagation(); onToggleMonitor(s.season.id, !s.season.monitored); }}
              style={{ background: "none", border: "none", cursor: "pointer", padding: 2, display: "flex", flexShrink: 0 }}
            >
              {s.season.monitored
                ? <CheckCircle2 size={16} style={{ color: "var(--color-accent)" }} />
                : <Circle size={16} style={{ color: "var(--color-text-muted)" }} />
              }
            </button>
          </div>
        );
      })}
    </div>
  );
}
