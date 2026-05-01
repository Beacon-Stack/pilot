import { useState, useMemo } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { ArrowLeft, Tv, CheckCircle2, RefreshCw } from "lucide-react";
import { useSeriesDetail, useSeasons, useCours, useUpdateCourMonitored, useEpisodes, useUpdateSeries, useUpdateEpisodeMonitored, useUpdateSeasonMonitored, useRefreshSeriesMetadata } from "@/api/series";
import { useSeriesHaulHistory, useReimportFromHaul, useSeriesGrabHistory, useReimportGrab } from "@/api/haul";
import type { HaulRecord, SeriesGrabHistoryItem } from "@/api/haul";
import { useEpisodeFiles, useLibraryScan } from "@/api/episode-files";
import { Poster } from "@/components/Poster";
import ManualSearchModal from "@/components/ManualSearchModal";
import { useAutoSearch } from "@/api/releases";
import { formatBytes } from "@/lib/utils";
import { toast } from "sonner";
import type { Episode, EpisodeFile, Season } from "@/types";

import SeasonPills from "./SeasonPills";
import SeasonHeader from "./SeasonHeader";
import type { EpisodeFilter } from "./SeasonHeader";
import EpisodeRow from "./EpisodeRow";
import AllSeasonsView, { buildSeasonSummaries } from "./AllSeasonsView";
import type { SeasonSummary } from "./AllSeasonsView";
import BulkActionBar from "./BulkActionBar";

// parseEpisodeFromReleaseTitle is a small recovery helper for
// orphaned-grab matching. When a grab_history row is missing
// episode_id (older grabs predating the ManualSearchModal fix that
// plumbed the UUID through), the release title is the only thing
// telling us which episode it was for.
//
// Two patterns covered:
//   - SxxExx / SxxExxExx — "Show.Name.S01E48.1080p..."  → {1, 48}
//   - Anime fansub absolute — "[SubsPlease] Show - 48 (1080p) [hash]"
//     → {undefined, 48}; the caller fills the season from the grab's
//     season_number column or the series' single-cour TMDB shape.
//
// Returns null when no pattern matches. Conservative on purpose —
// false positives badge the wrong episode, which is worse than no
// badge at all.
export function parseEpisodeFromReleaseTitle(title: string): { season?: number; episode: number } | null {
  const sxx = title.match(/[Ss](\d{1,2})[Ee](\d{1,3})/);
  if (sxx) return { season: parseInt(sxx[1], 10), episode: parseInt(sxx[2], 10) };
  // Anime fansub form: "<group> Show Name - 48 (..." or " - 48v2 ["
  const dash = title.match(/\s-\s+(\d{1,3})(?:v\d+)?\s+[(\[]/);
  if (dash) return { episode: parseInt(dash[1], 10) };
  return null;
}

interface SearchTarget {
  seriesId: string;
  // seasonNumber/episodeNumber are TMDB-relative — the backend's
  // search filter resolves releases against these (Phase 2A's
  // TVDB→absolute conversion expects TMDB coords).
  seasonNumber: number;
  episodeNumber?: number;
  // episodeId is set when the search is scoped to a specific episode
  // (vs a season pack search). Forwarded into grab_history so the
  // row carries episode_id — without it, downstream features (orphan
  // detection, per-episode history) miss the row.
  episodeId?: string;
  // displaySeasonNumber/displayEpisodeNumber are what the user sees in
  // the UI (cour-relative in cour mode: Season 3 ep 1 instead of
  // TMDB-relative Season 1 ep 48). Used only for labels (toast +
  // modal title); the API call still uses seasonNumber/episodeNumber.
  // When omitted the modal/toast falls back to the API values, which
  // is the correct behaviour for non-anime series.
  displaySeasonNumber?: number;
  displayEpisodeNumber?: number;
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
  haulMap,
  grabRows,
  onSearch,
  onAutoSearch,
  isAutoSearching,
  fetchSeasonNumber,
  filterEpisodeIDs,
  displayEpisodeOffset,
  onMonitorOverride,
  onReimport,
  onReimportGrab,
}: {
  seriesId: string;
  season: Season;
  fileMap: Map<string, EpisodeFile>;
  haulMap: Map<string, HaulRecord>;
  // grabRows is the unfiltered grab_history list — orphaned-grab
  // matching happens locally below using the season's episode list,
  // because some old grabs are missing episode_id and need a release-
  // title-based fallback to find the right episode.
  grabRows: SeriesGrabHistoryItem[] | undefined;
  onSearch: (target: SearchTarget) => void;
  onAutoSearch: (target: SearchTarget) => void;
  isAutoSearching: boolean;
  // fetchSeasonNumber overrides which TMDB season to fetch episodes
  // from AND the season number passed to the backend for search/grab.
  // Cours have season.season_number = the cour identifier (e.g. 3 for
  // "Season 3"), but episodes live in a different TMDB season (0 for
  // specials, 1 for typical cours). The backend's search filter resolves
  // releases against TMDB-relative coords (Phase 2A handles the
  // TVDB→absolute conversion), so we must pass the TMDB season here.
  // Defaults to season.season_number when omitted (non-anime path).
  fetchSeasonNumber?: number;
  // filterEpisodeIDs, when provided, narrows the rendered episode list
  // to just those IDs. Used for cour mode to slice the TMDB season's
  // full episode list down to the cour's window.
  filterEpisodeIDs?: Set<string>;
  // displayEpisodeOffset shifts the displayed episode number for
  // cour-relative numbering — e.g. cour 3 starts at TMDB ep 48 with
  // offset=47, so the user sees "3x01" instead of "3x48". Search and
  // grab calls still use the underlying TMDB-relative episode number.
  displayEpisodeOffset?: number;
  // onMonitorOverride replaces the default season-monitor mutation.
  // For cours we hit the cour endpoint instead of the season endpoint.
  onMonitorOverride?: (monitored: boolean) => void;
  onReimport?: (infoHash: string) => void;
  onReimportGrab?: (grabId: string) => void;
}) {
  // apiSeasonNumber is the season used for backend API calls (search,
  // grab, episode-list fetch). For cour mode this is the underlying
  // TMDB season, not the cour identifier — the backend's filter
  // resolves against TMDB-relative coords.
  const apiSeasonNumber = fetchSeasonNumber ?? season.season_number;
  const { data: episodesAll, isLoading } = useEpisodes(seriesId, apiSeasonNumber);
  const episodes = useMemo(() => {
    if (!episodesAll) return undefined;
    if (!filterEpisodeIDs) return episodesAll;
    return episodesAll.filter((ep: Episode) => filterEpisodeIDs.has(ep.id));
  }, [episodesAll, filterEpisodeIDs]);
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

  // Build episode_id → orphaned grab map for THIS season's episodes.
  // Two match paths per row:
  //   1. grab.episode_id is set → direct hit
  //   2. grab.episode_id is empty → parse the release title for an
  //      episode number (SxxExx or anime "- 48" form), then lookup
  //      the matching episode within this season. This recovers
  //      pre-fix grabs that landed in grab_history without episode_id.
  // Most-recent-wins per episode.
  const orphanedGrabMap = useMemo(() => {
    const m = new Map<string, SeriesGrabHistoryItem>();
    if (!grabRows || !episodes) return m;
    for (const g of grabRows) {
      if (g.download_status !== "completed") continue;
      let epId = g.episode_id;
      if (!epId) {
        const parsed = parseEpisodeFromReleaseTitle(g.release_title);
        if (parsed === null) continue;
        const targetSeason = parsed.season ?? g.season_number;
        const ep = episodes.find((e: Episode) =>
          (targetSeason === undefined || e.season_number === targetSeason) &&
          e.episode_number === parsed.episode,
        );
        if (!ep) continue;
        epId = ep.id;
      }
      const existing = m.get(epId);
      if (!existing || g.grabbed_at > existing.grabbed_at) {
        m.set(epId, g);
      }
    }
    return m;
  }, [grabRows, episodes]);

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
      if (ep) updateEpMonitored.mutate({ episodeId: id, monitored, seasonNumber: apiSeasonNumber });
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
        onToggleMonitor={() => {
          if (onMonitorOverride) {
            onMonitorOverride(!season.monitored);
          } else {
            updateSeasonMonitored.mutate({ seasonId: season.id, monitored: !season.monitored });
          }
        }}
        onInteractiveSearch={() => onSearch({ seriesId, seasonNumber: apiSeasonNumber, displaySeasonNumber: season.season_number })}
        onAutoSearchSeason={() => onAutoSearch({ seriesId, seasonNumber: apiSeasonNumber, displaySeasonNumber: season.season_number })}
        isAutoSearching={isAutoSearching}
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
              displayEpisodeOffset={displayEpisodeOffset}
              selected={selected.has(ep.id)}
              onToggleSelect={() => toggleSelect(ep.id)}
              onToggleMonitor={() => updateEpMonitored.mutate({ episodeId: ep.id, monitored: !ep.monitored, seasonNumber: apiSeasonNumber })}
              onSearch={() => onSearch({
                seriesId,
                seasonNumber: apiSeasonNumber,
                episodeNumber: ep.episode_number,
                episodeId: ep.id,
                displaySeasonNumber: season.season_number,
                displayEpisodeNumber: ep.episode_number - (displayEpisodeOffset ?? 0),
              })}
              onAutoSearch={() => onAutoSearch({
                seriesId,
                seasonNumber: apiSeasonNumber,
                episodeNumber: ep.episode_number,
                episodeId: ep.id,
                displaySeasonNumber: season.season_number,
                displayEpisodeNumber: ep.episode_number - (displayEpisodeOffset ?? 0),
              })}
              haulRecord={haulMap.get(ep.id)}
              onReimport={onReimport}
              orphanedGrab={orphanedGrabMap.get(ep.id)}
              onReimportGrab={onReimportGrab}
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
            if (ep) onSearch({
              seriesId,
              seasonNumber: apiSeasonNumber,
              episodeNumber: ep.episode_number,
              episodeId: ep.id,
              displaySeasonNumber: season.season_number,
              displayEpisodeNumber: ep.episode_number - (displayEpisodeOffset ?? 0),
            });
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
  const isAnime = series?.series_type === "anime";
  const { data: cours } = useCours(id ?? "", isAnime);
  const updateSeries = useUpdateSeries(id ?? "");
  const refreshMetadata = useRefreshSeriesMetadata(id ?? "");
  const updateSeasonMonitored = useUpdateSeasonMonitored(id ?? "");
  const updateCourMonitored = useUpdateCourMonitored(id ?? "");
  const libraryScan = useLibraryScan();
  const [searchTarget, setSearchTarget] = useState<SearchTarget | null>(null);
  const autoSearch = useAutoSearch(id ?? "");
  const [activeSeason, setActiveSeason] = useState(-1);

  function handleAutoSearch(target: SearchTarget) {
    const labelSeason = target.displaySeasonNumber ?? target.seasonNumber;
    const labelEpisode = target.displayEpisodeNumber ?? target.episodeNumber;
    const label = labelEpisode !== undefined
      ? `S${String(labelSeason).padStart(2, "0")}E${String(labelEpisode).padStart(2, "0")}`
      : `Season ${labelSeason}`;
    const toastId = toast.loading(`Searching releases for ${label}…`);
    autoSearch.mutate(
      { season: target.seasonNumber, episode: target.episodeNumber },
      {
        onSuccess: (data) => {
          if (data.result === "grabbed") {
            toast.success(`Grabbed: ${data.release_title}`, { id: toastId });
          } else {
            toast.info(data.reason ?? "No matching release found", { id: toastId });
          }
        },
        onError: (err) => toast.error((err as Error).message, { id: toastId }),
      }
    );
  }

  const { data: episodeFiles } = useEpisodeFiles(id ?? "");
  const fileMap = useMemo(
    () => new Map<string, EpisodeFile>((episodeFiles ?? []).map((f) => [f.episode_id, f])),
    [episodeFiles]
  );

  const { data: haulRecords } = useSeriesHaulHistory(id ?? "");
  // Build episode_id → record map; only include completed, still-present records
  const haulMap = useMemo(() => {
    const m = new Map<string, HaulRecord>();
    for (const r of haulRecords ?? []) {
      if (r.completed_at && !r.removed_at && r.episode_id) {
        m.set(r.episode_id, r);
      }
    }
    return m;
  }, [haulRecords]);
  const reimport = useReimportFromHaul(id ?? "");

  // Orphaned grabs: completed grabs whose file isn't (yet) linked into
  // the library. The map's value is the most-recent completed grab per
  // episode_id. Most-recent-wins to avoid badging episodes off a stale
  // grab when there's been a retry.
  //
  // Building the key has two paths:
  //   1. Direct: grab.episode_id is set — happy path for grabs made
  //      after we plumbed episode_id through ManualSearchModal.
  //   2. Fallback: grab.episode_id is empty (older grabs predating that
  //      fix) — parse the release title for the episode number, then
  //      match against the per-season episode lists from haulMap's
  //      backing data... actually we don't have that here. Pass
  //      grabRows down to SeasonEpisodeList instead, which has the
  //      episode list and can do the per-season match locally.
  const { data: grabRows } = useSeriesGrabHistory(id ?? "");
  const reimportGrab = useReimportGrab(id ?? "");

  // For anime series with an Anime-Lists mapping, the cours[] response
  // is non-empty and we project each cour into a Season-shaped object
  // for the existing UI components to consume. The synthetic id starts
  // with "cour-" so SeasonEpisodeList can detect cour mode and use the
  // alternate fetch+filter+monitor paths. Each cour maps to TMDB
  // Season 1's episodes via a precomputed episode-id set.
  const useCourMode = isAnime && (cours?.length ?? 0) > 0;
  const allSeasons: Season[] = useMemo(() => {
    if (useCourMode && cours) {
      return cours.map((c) => ({
        id: `cour-${c.tvdb_season}`,
        series_id: id ?? "",
        season_number: c.tvdb_season,
        monitored: c.monitored,
        episode_count: c.episode_count,
        episode_file_count: c.episode_file_count,
        total_size_bytes: c.total_size_bytes,
      }));
    }
    return seasons ?? [];
  }, [seasons, cours, useCourMode, id]);

  const courEpisodeIDsByCour = useMemo(() => {
    const m = new Map<number, Set<string>>();
    for (const c of cours ?? []) m.set(c.tvdb_season, new Set(c.episode_ids));
    return m;
  }, [cours]);

  // Per-cour TMDB-shape metadata — fetch the right TMDB season for each
  // cour (specials live in TMDB Season 0; the rest typically Season 1)
  // and subtract the per-cour episode offset for cour-relative display
  // (cour 3 ep "3x48" → "3x01").
  const courMetaByCour = useMemo(() => {
    const m = new Map<number, { tmdbSeason: number; episodeOffset: number }>();
    for (const c of cours ?? []) {
      m.set(c.tvdb_season, { tmdbSeason: c.tmdb_season, episodeOffset: c.episode_offset });
    }
    return m;
  }, [cours]);

  const regularSeasons = allSeasons.filter((s) => s.season_number > 0);
  const specials = allSeasons.filter((s) => s.season_number === 0);
  const orderedSeasons = [...regularSeasons, ...specials];

  const episodeCounts = useMemo(() => new Map<number, { total: number; downloaded: number }>(), []);

  const seasonSummaries: SeasonSummary[] = useMemo(
    () => buildSeasonSummaries(orderedSeasons),
    [orderedSeasons]
  );

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
                <button
                  onClick={() => {
                    refreshMetadata.mutate(undefined, {
                      onSuccess: (s) => toast.success(
                        `Metadata refreshed${(s.alternate_titles?.length ?? 0) > 0 ? ` · ${s.alternate_titles!.length} alternate title${s.alternate_titles!.length === 1 ? "" : "s"}` : ""}`
                      ),
                      onError: (e) => toast.error((e as Error).message),
                    });
                  }}
                  disabled={refreshMetadata.isPending}
                  style={{
                    display: "inline-flex", alignItems: "center", gap: 6, padding: "6px 12px",
                    border: "1px solid var(--color-border-default)", borderRadius: 6,
                    background: "none", cursor: "pointer", fontSize: 13, fontWeight: 500,
                    color: "var(--color-text-secondary)", opacity: refreshMetadata.isPending ? 0.6 : 1,
                  }}
                  title="Re-fetch series metadata from TMDB (incl. alternate titles)"
                >
                  <RefreshCw size={15} style={{ animation: refreshMetadata.isPending ? "spin 1s linear infinite" : "none" }} />
                  Refresh Metadata
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
          episodeId={searchTarget.episodeId}
          displaySeasonNumber={searchTarget.displaySeasonNumber}
          displayEpisodeNumber={searchTarget.displayEpisodeNumber}
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
              onToggleMonitor={(seasonId, monitored) => {
                // Synthetic cour ids start with "cour-" — route the
                // monitor toggle to the cour endpoint instead of the
                // per-season-row update.
                if (useCourMode && seasonId.startsWith("cour-")) {
                  const tvdbSeason = parseInt(seasonId.slice(5), 10);
                  updateCourMonitored.mutate({ tvdbSeason, monitored });
                } else {
                  updateSeasonMonitored.mutate({ seasonId, monitored });
                }
              }}
            />
          ) : selectedSeason ? (
            <SeasonEpisodeList
              seriesId={series.id}
              season={selectedSeason}
              fileMap={fileMap}
              haulMap={haulMap}
              grabRows={grabRows}
              onSearch={setSearchTarget}
              onAutoSearch={handleAutoSearch}
              isAutoSearching={autoSearch.isPending}
              // Cour mode: fetch the underlying TMDB season for THIS
              // cour (specials use 0, normal cours typically 1) and
              // filter to just the cour's episode window. The cour's
              // tvdb_season is what the user sees in the pill label.
              fetchSeasonNumber={useCourMode ? courMetaByCour.get(selectedSeason.season_number)?.tmdbSeason : undefined}
              filterEpisodeIDs={useCourMode ? courEpisodeIDsByCour.get(selectedSeason.season_number) : undefined}
              displayEpisodeOffset={useCourMode ? courMetaByCour.get(selectedSeason.season_number)?.episodeOffset : undefined}
              onMonitorOverride={useCourMode ? (monitored) => updateCourMonitored.mutate({ tvdbSeason: selectedSeason.season_number, monitored }) : undefined}
              onReimport={(infoHash) => reimport.mutate(infoHash)}
              onReimportGrab={(grabId) => reimportGrab.mutate(grabId)}
            />
          ) : (
            <div style={{ fontSize: 13, color: "var(--color-text-muted)", padding: 20 }}>Season not found.</div>
          )}
        </div>
      </div>
    </div>
  );
}
