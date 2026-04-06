import { useState, useMemo } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { ArrowLeft, Tv, CheckCircle2, RefreshCw } from "lucide-react";
import { useSeriesDetail, useSeasons, useEpisodes, useUpdateSeries, useUpdateEpisodeMonitored, useUpdateSeasonMonitored } from "@/api/series";
import { useEpisodeFiles, useLibraryScan } from "@/api/episode-files";
import { Poster } from "@/components/Poster";
import ManualSearchModal from "@/components/ManualSearchModal";
import { formatBytes } from "@/lib/utils";
import type { Episode, EpisodeFile, Season } from "@/types";

import SeasonPills from "./SeasonPills";
import SeasonHeader from "./SeasonHeader";
import type { EpisodeFilter } from "./SeasonHeader";
import EpisodeRow from "./EpisodeRow";
import AllSeasonsView from "./AllSeasonsView";
import type { SeasonSummary } from "./AllSeasonsView";
import BulkActionBar from "./BulkActionBar";

interface SearchTarget {
  seriesId: string;
  seasonNumber: number;
  episodeNumber?: number;
}

function SeriesStatusBadge({ status }: { status: string }) {
  const colors: Record<string, { color: string; bg: string }> = {
    continuing: { color: "var(--color-success)", bg: "color-mix(in srgb, var(--color-success) 12%, transparent)" },
    ended:      { color: "var(--color-text-muted)", bg: "var(--color-bg-subtle)" },
    upcoming:   { color: "var(--color-warning)", bg: "color-mix(in srgb, var(--color-warning) 12%, transparent)" },
  };
  const style = colors[status] ?? colors.ended;
  return (
    <span style={{ display: "inline-flex", alignItems: "center", padding: "3px 8px", borderRadius: 4, fontSize: 12, fontWeight: 500, color: style.color, background: style.bg, textTransform: "capitalize" }}>
      {status}
    </span>
  );
}

// ── Season episode list with filtering and bulk actions ─────────────────────

function SeasonEpisodeList({
  seriesId,
  season,
  fileMap,
  onSearch,
}: {
  seriesId: string;
  season: Season;
  fileMap: Map<string, EpisodeFile>;
  onSearch: (target: SearchTarget) => void;
}) {
  const { data: episodes, isLoading } = useEpisodes(seriesId, season.season_number);
  const updateEpMonitored = useUpdateEpisodeMonitored(seriesId);
  const updateSeasonMonitored = useUpdateSeasonMonitored(seriesId);
  const [filter, setFilter] = useState<EpisodeFilter>("all");
  const [selected, setSelected] = useState<Set<string>>(new Set());

  const now = new Date();

  const filtered = useMemo(() => {
    if (!episodes) return [];
    return episodes.filter((ep: Episode) => {
      const aired = ep.air_date ? new Date(ep.air_date) <= now : false;
      switch (filter) {
        case "downloaded": return ep.has_file;
        case "missing": return aired && ep.monitored && !ep.has_file;
        case "unaired": return !aired;
        case "unmonitored": return !ep.monitored;
        default: return true;
      }
    });
  }, [episodes, filter]);

  const downloadedCount = episodes?.filter((ep: Episode) => ep.has_file).length ?? 0;
  const totalSize = useMemo(() => {
    let sum = 0;
    for (const ep of episodes ?? []) {
      const f = fileMap.get(ep.id);
      if (f) sum += f.size_bytes;
    }
    return sum;
  }, [episodes, fileMap]);

  const toggleSelect = (id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  };

  const handleBulkMonitor = (monitored: boolean) => {
    for (const id of selected) {
      const ep = episodes?.find((e: Episode) => e.id === id);
      if (ep) updateEpMonitored.mutate({ episodeId: id, monitored, seasonNumber: season.season_number });
    }
    setSelected(new Set());
  };

  if (isLoading) {
    return (
      <div style={{ padding: "16px 0" }}>
        {Array.from({ length: 4 }).map((_, i) => (
          <div key={i} className="skeleton" style={{ height: 40, borderRadius: 4, marginBottom: 6 }} />
        ))}
      </div>
    );
  }

  return (
    <>
      <SeasonHeader
        season={season}
        episodeCount={episodes?.length ?? 0}
        downloadedCount={downloadedCount}
        totalSize={totalSize > 0 ? formatBytes(totalSize) : ""}
        filter={filter}
        onFilterChange={setFilter}
        onToggleMonitor={() => updateSeasonMonitored.mutate({ seasonId: season.id, monitored: !season.monitored })}
        onSearchMissing={() => onSearch({ seriesId, seasonNumber: season.season_number })}
      />

      {filtered.length === 0 ? (
        <div style={{ padding: 20, fontSize: 13, color: "var(--color-text-muted)", textAlign: "center" }}>
          {filter === "all" ? "No episodes." : `No ${filter} episodes.`}
        </div>
      ) : (
        <div style={{ border: "1px solid var(--color-border-subtle)", borderRadius: 8, overflow: "hidden" }}>
          {filtered.map((ep: Episode) => (
            <EpisodeRow
              key={ep.id}
              episode={ep}
              file={fileMap.get(ep.id)}
              seasonNumber={season.season_number}
              selected={selected.has(ep.id)}
              onToggleSelect={() => toggleSelect(ep.id)}
              onToggleMonitor={() => updateEpMonitored.mutate({ episodeId: ep.id, monitored: !ep.monitored, seasonNumber: season.season_number })}
              onSearch={() => onSearch({ seriesId, seasonNumber: season.season_number, episodeNumber: ep.episode_number })}
            />
          ))}
        </div>
      )}

      <BulkActionBar
        count={selected.size}
        onMonitor={() => handleBulkMonitor(true)}
        onUnmonitor={() => handleBulkMonitor(false)}
        onSearch={() => {
          for (const id of selected) {
            const ep = episodes?.find((e: Episode) => e.id === id);
            if (ep) onSearch({ seriesId, seasonNumber: season.season_number, episodeNumber: ep.episode_number });
          }
          setSelected(new Set());
        }}
        onClear={() => setSelected(new Set())}
      />
    </>
  );
}

// ── Main page ───────────────────────────────────────────────────────────────

export default function SeriesDetail() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { data: series, isLoading, isError } = useSeriesDetail(id ?? "");
  const { data: seasons } = useSeasons(id ?? "");
  const updateSeries = useUpdateSeries(id ?? "");
  const updateSeasonMonitored = useUpdateSeasonMonitored(id ?? "");
  const libraryScan = useLibraryScan();
  const [searchTarget, setSearchTarget] = useState<SearchTarget | null>(null);
  const [activeSeason, setActiveSeason] = useState(-1);

  const { data: episodeFiles } = useEpisodeFiles(id ?? "");
  const fileMap = useMemo(
    () => new Map<string, EpisodeFile>((episodeFiles ?? []).map((f) => [f.episode_id, f])),
    [episodeFiles]
  );

  const allSeasons = seasons ?? [];
  const regularSeasons = allSeasons.filter((s) => s.season_number > 0);
  const specials = allSeasons.filter((s) => s.season_number === 0);
  const orderedSeasons = [...regularSeasons, ...specials];

  const episodeCounts = useMemo(() => new Map<number, { total: number; downloaded: number }>(), []);

  const seasonSummaries: SeasonSummary[] = useMemo(() => {
    return orderedSeasons.map((s) => ({
      season: s,
      totalEpisodes: 0,
      downloadedEpisodes: 0,
      totalSize: 0,
    }));
  }, [orderedSeasons]);

  const selectedSeason = orderedSeasons.find((s) => s.season_number === activeSeason);

  if (isLoading) {
    return (
      <div style={{ padding: "24px 28px" }}>
        <div className="skeleton" style={{ height: 220, borderRadius: 8, marginBottom: 24 }} />
        <div className="skeleton" style={{ height: 400, borderRadius: 8 }} />
      </div>
    );
  }

  if (isError || !series) {
    return (
      <div style={{ padding: "24px 28px" }}>
        <button onClick={() => navigate(-1)} style={{ display: "flex", alignItems: "center", gap: 6, background: "none", border: "none", cursor: "pointer", color: "var(--color-text-secondary)", fontSize: 14, padding: 0, marginBottom: 20 }}>
          <ArrowLeft size={16} /> Back
        </button>
        <div style={{ padding: 20, background: "color-mix(in srgb, var(--color-danger) 10%, transparent)", border: "1px solid color-mix(in srgb, var(--color-danger) 30%, transparent)", borderRadius: 8, color: "var(--color-danger)", fontSize: 14 }}>
          Series not found.
        </div>
      </div>
    );
  }

  return (
    <div>
      {/* Hero banner */}
      <div style={{
        position: "relative", minHeight: 240,
        background: series.fanart_url
          ? `linear-gradient(to bottom, rgba(0,0,0,0.5) 0%, var(--color-bg-base) 100%), url(${series.fanart_url}) center/cover no-repeat`
          : "var(--color-bg-surface)",
        borderBottom: "1px solid var(--color-border-subtle)",
      }}>
        <div style={{ padding: "20px 28px 28px" }}>
          <button onClick={() => navigate(-1)} style={{ display: "flex", alignItems: "center", gap: 6, background: "none", border: "none", cursor: "pointer", color: "var(--color-text-secondary)", fontSize: 13, padding: 0, marginBottom: 20 }}>
            <ArrowLeft size={15} /> All Series
          </button>

          <div style={{ display: "flex", gap: 24, alignItems: "flex-start" }}>
            <div style={{ width: 120, flexShrink: 0 }}>
              <Poster src={series.poster_url} title={series.title} year={series.year} loading="eager" style={{ borderRadius: 8, boxShadow: "0 4px 16px rgba(0,0,0,0.5)" }} />
            </div>

            <div style={{ flex: 1, minWidth: 0 }}>
              <div style={{ display: "flex", alignItems: "center", gap: 10, flexWrap: "wrap", marginBottom: 6 }}>
                <h1 style={{ margin: 0, fontSize: 24, fontWeight: 700, color: "var(--color-text-primary)", letterSpacing: "-0.02em" }}>{series.title}</h1>
                <SeriesStatusBadge status={series.status} />
              </div>

              <div style={{ display: "flex", alignItems: "center", gap: 12, fontSize: 13, color: "var(--color-text-secondary)", marginBottom: 8, flexWrap: "wrap" }}>
                <span>{series.year}</span>
                {series.network && <><span style={{ color: "var(--color-border-default)" }}>·</span><span>{series.network}</span></>}
                {series.runtime_minutes && <><span style={{ color: "var(--color-border-default)" }}>·</span><span>{series.runtime_minutes}m</span></>}
                {series.genres.length > 0 && <><span style={{ color: "var(--color-border-default)" }}>·</span><span>{series.genres.slice(0, 3).join(", ")}</span></>}
              </div>

              <div style={{ fontSize: 13, color: "var(--color-text-muted)", marginBottom: 12 }}>
                {series.episode_file_count}/{series.episode_count} episodes
                {episodeFiles && episodeFiles.length > 0 && ` · ${formatBytes(episodeFiles.reduce((sum, f) => sum + f.size_bytes, 0))}`}
              </div>

              <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
                <button
                  onClick={() => updateSeries.mutate({ monitored: !series.monitored })}
                  style={{
                    display: "inline-flex", alignItems: "center", gap: 6, padding: "6px 12px",
                    border: "1px solid var(--color-border-default)", borderRadius: 6,
                    background: series.monitored ? "var(--color-accent-muted)" : "none",
                    cursor: "pointer", fontSize: 13, fontWeight: 500,
                    color: series.monitored ? "var(--color-accent)" : "var(--color-text-secondary)",
                  }}
                >
                  {series.monitored ? <><CheckCircle2 size={15} /> Monitored</> : <><Tv size={15} /> Monitor</>}
                </button>
                <button
                  onClick={() => libraryScan.mutate()}
                  disabled={libraryScan.isPending}
                  style={{
                    display: "inline-flex", alignItems: "center", gap: 6, padding: "6px 12px",
                    border: "1px solid var(--color-border-default)", borderRadius: 6,
                    background: "none", cursor: "pointer", fontSize: 13, fontWeight: 500,
                    color: "var(--color-text-secondary)", opacity: libraryScan.isPending ? 0.6 : 1,
                  }}
                >
                  <RefreshCw size={15} style={{ animation: libraryScan.isPending ? "spin 1s linear infinite" : "none" }} />
                  Scan Library
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>

      {searchTarget && (
        <ManualSearchModal
          seriesId={searchTarget.seriesId}
          seasonNumber={searchTarget.seasonNumber}
          episodeNumber={searchTarget.episodeNumber}
          onClose={() => setSearchTarget(null)}
        />
      )}

      <div style={{ padding: "24px 28px" }}>
        {series.overview && (
          <div style={{ background: "var(--color-bg-surface)", border: "1px solid var(--color-border-subtle)", borderRadius: 8, padding: 20, marginBottom: 24, boxShadow: "var(--shadow-card)" }}>
            <p style={{ margin: 0, fontSize: 14, lineHeight: 1.65, color: "var(--color-text-secondary)" }}>{series.overview}</p>
          </div>
        )}

        <div style={{ background: "var(--color-bg-surface)", border: "1px solid var(--color-border-subtle)", borderRadius: 8, padding: 20, boxShadow: "var(--shadow-card)" }}>
          <SeasonPills
            seasons={orderedSeasons}
            activeSeason={activeSeason}
            onSelect={setActiveSeason}
            episodeCounts={episodeCounts}
          />

          {activeSeason === -1 ? (
            <AllSeasonsView
              summaries={seasonSummaries}
              onSelectSeason={setActiveSeason}
              onToggleMonitor={(seasonId, monitored) => updateSeasonMonitored.mutate({ seasonId, monitored })}
            />
          ) : selectedSeason ? (
            <SeasonEpisodeList
              seriesId={series.id}
              season={selectedSeason}
              fileMap={fileMap}
              onSearch={setSearchTarget}
            />
          ) : (
            <div style={{ fontSize: 13, color: "var(--color-text-muted)", padding: 20 }}>Season not found.</div>
          )}
        </div>
      </div>
    </div>
  );
}
