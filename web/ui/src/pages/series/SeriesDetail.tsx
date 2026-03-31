import { useState } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { ArrowLeft, Tv, CheckCircle2, Circle, FileCheck, FileX, Search, RefreshCw } from "lucide-react";
import { useSeriesDetail, useSeasons, useEpisodes, useUpdateSeries, useUpdateEpisodeMonitored, useUpdateSeasonMonitored } from "@/api/series";
import { useEpisodeFiles, useLibraryScan } from "@/api/episode-files";
import { Poster } from "@/components/Poster";
import ManualSearchModal from "@/components/ManualSearchModal";
import { formatBytes } from "@/lib/utils";
import type { Episode, EpisodeFile } from "@/types";

// ── Search state shared by SeasonTabs/SeasonEpisodes ─────────────────────────

interface SearchTarget {
  seriesId: string;
  seasonNumber: number;
  episodeNumber?: number;
}

// ── Status badge ─────────────────────────────────────────────────────────────

function StatusBadge({ status }: { status: string }) {
  const colors: Record<string, { color: string; bg: string }> = {
    continuing: { color: "var(--color-success)", bg: "color-mix(in srgb, var(--color-success) 12%, transparent)" },
    ended:      { color: "var(--color-text-muted)", bg: "var(--color-bg-subtle)" },
    upcoming:   { color: "var(--color-warning)", bg: "color-mix(in srgb, var(--color-warning) 12%, transparent)" },
  };
  const style = colors[status] ?? colors.ended;

  return (
    <span
      style={{
        display: "inline-flex",
        alignItems: "center",
        padding: "3px 8px",
        borderRadius: 4,
        fontSize: 12,
        fontWeight: 500,
        color: style.color,
        background: style.bg,
        textTransform: "capitalize",
      }}
    >
      {status}
    </span>
  );
}

// ── Episode table for one season ─────────────────────────────────────────────

function SeasonEpisodes({
  seriesId,
  seasonNumber,
  fileMap,
  onSearch,
}: {
  seriesId: string;
  seasonNumber: number;
  fileMap: Map<string, EpisodeFile>;
  onSearch: (target: SearchTarget) => void;
}) {
  const { data: episodes, isLoading } = useEpisodes(seriesId, seasonNumber);
  const updateMonitored = useUpdateEpisodeMonitored(seriesId);

  if (isLoading) {
    return (
      <div style={{ padding: "16px 0" }}>
        {Array.from({ length: 4 }).map((_, i) => (
          <div
            key={i}
            className="skeleton"
            style={{ height: 40, borderRadius: 4, marginBottom: 6 }}
          />
        ))}
      </div>
    );
  }

  if (!episodes?.length) {
    return (
      <div style={{ padding: "16px 0", fontSize: 13, color: "var(--color-text-muted)" }}>
        No episodes found.
      </div>
    );
  }

  return (
    <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}>
      <thead>
        <tr>
          {["#", "Title", "Air Date", "Monitored", "File", ""].map((h) => (
            <th
              key={h}
              style={{
                textAlign: "left",
                padding: "6px 8px",
                fontSize: 11,
                fontWeight: 600,
                letterSpacing: "0.06em",
                textTransform: "uppercase",
                color: "var(--color-text-muted)",
                borderBottom: "1px solid var(--color-border-subtle)",
              }}
            >
              {h}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {episodes.map((ep: Episode) => (
          <tr
            key={ep.id}
            style={{
              borderBottom: "1px solid var(--color-border-subtle)",
            }}
          >
            <td
              style={{
                padding: "10px 8px",
                color: "var(--color-text-muted)",
                fontVariantNumeric: "tabular-nums",
                width: 48,
              }}
            >
              {ep.episode_number}
            </td>
            <td style={{ padding: "10px 8px", color: "var(--color-text-primary)" }}>
              {ep.title}
            </td>
            <td
              style={{
                padding: "10px 8px",
                color: "var(--color-text-secondary)",
                whiteSpace: "nowrap",
                width: 110,
              }}
            >
              {ep.air_date
                ? new Date(ep.air_date).toLocaleDateString(undefined, {
                    year: "numeric",
                    month: "short",
                    day: "numeric",
                  })
                : "—"}
            </td>
            <td style={{ padding: "10px 8px", width: 96 }}>
              <button
                onClick={() =>
                  updateMonitored.mutate({
                    episodeId: ep.id,
                    monitored: !ep.monitored,
                    seasonNumber,
                  })
                }
                style={{
                  background: "none",
                  border: "none",
                  cursor: "pointer",
                  padding: 0,
                  display: "flex",
                  alignItems: "center",
                  color: ep.monitored ? "var(--color-accent)" : "var(--color-text-muted)",
                }}
                title={ep.monitored ? "Monitored — click to unmonitor" : "Unmonitored — click to monitor"}
              >
                {ep.monitored
                  ? <CheckCircle2 size={16} strokeWidth={1.5} />
                  : <Circle size={16} strokeWidth={1.5} />}
              </button>
            </td>
            <td style={{ padding: "10px 8px", width: 120 }}>
              {ep.has_file ? (
                <span
                  style={{
                    display: "inline-flex",
                    alignItems: "center",
                    gap: 5,
                    color: "var(--color-success)",
                  }}
                >
                  <FileCheck size={14} strokeWidth={1.5} style={{ flexShrink: 0 }} />
                  {fileMap.has(ep.id) && (
                    <span style={{ fontSize: 12, color: "var(--color-text-secondary)", fontVariantNumeric: "tabular-nums" }}>
                      {formatBytes(fileMap.get(ep.id)!.size_bytes)}
                    </span>
                  )}
                </span>
              ) : (
                <FileX size={14} strokeWidth={1.5} style={{ color: "var(--color-text-muted)" }} />
              )}
            </td>
            <td style={{ padding: "10px 8px", width: 40 }}>
              <button
                onClick={() =>
                  onSearch({ seriesId, seasonNumber, episodeNumber: ep.episode_number })
                }
                style={{
                  background: "none",
                  border: "none",
                  cursor: "pointer",
                  padding: 2,
                  display: "flex",
                  alignItems: "center",
                  color: "var(--color-text-muted)",
                  borderRadius: 4,
                }}
                title={`Search for S${String(seasonNumber).padStart(2, "0")}E${String(ep.episode_number).padStart(2, "0")}`}
              >
                <Search size={14} strokeWidth={1.5} />
              </button>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

// ── Season tabs ───────────────────────────────────────────────────────────────

function SeasonTabs({
  seriesId,
  fileMap,
  onSearch,
}: {
  seriesId: string;
  fileMap: Map<string, EpisodeFile>;
  onSearch: (target: SearchTarget) => void;
}) {
  const { data: seasons, isLoading } = useSeasons(seriesId);
  const updateSeasonMonitored = useUpdateSeasonMonitored(seriesId);

  const allSeasons = seasons ?? [];
  const regularSeasons = allSeasons.filter((s) => s.season_number > 0);
  const specials = allSeasons.filter((s) => s.season_number === 0);
  const orderedSeasons = [...regularSeasons, ...specials];

  const [activeIdx, setActiveIdx] = useState(0);

  if (isLoading) {
    return (
      <div style={{ display: "flex", gap: 8, marginBottom: 16 }}>
        {Array.from({ length: 3 }).map((_, i) => (
          <div key={i} className="skeleton" style={{ height: 32, width: 80, borderRadius: 6 }} />
        ))}
      </div>
    );
  }

  if (!orderedSeasons.length) {
    return (
      <div style={{ fontSize: 13, color: "var(--color-text-muted)" }}>
        No seasons available.
      </div>
    );
  }

  const activeSeason = orderedSeasons[activeIdx];

  return (
    <div>
      {/* Tab strip */}
      <div
        style={{
          display: "flex",
          gap: 4,
          marginBottom: 20,
          borderBottom: "1px solid var(--color-border-subtle)",
          paddingBottom: 0,
        }}
      >
        {orderedSeasons.map((season, idx) => {
          const isActive = idx === activeIdx;
          return (
            <button
              key={season.id}
              onClick={() => setActiveIdx(idx)}
              style={{
                padding: "8px 14px",
                background: "none",
                border: "none",
                cursor: "pointer",
                fontSize: 13,
                fontWeight: isActive ? 600 : 500,
                color: isActive ? "var(--color-accent)" : "var(--color-text-secondary)",
                borderBottom: isActive ? "2px solid var(--color-accent)" : "2px solid transparent",
                marginBottom: -1,
                transition: "color 150ms ease",
                whiteSpace: "nowrap",
              }}
            >
              {season.season_number === 0 ? "Specials" : `Season ${season.season_number}`}
            </button>
          );
        })}
      </div>

      {/* Season header with monitored toggle + season search */}
      {activeSeason && (
        <div
          style={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            marginBottom: 12,
          }}
        >
          <span style={{ fontSize: 13, color: "var(--color-text-muted)" }}>
            {orderedSeasons.length} season{orderedSeasons.length !== 1 ? "s" : ""}
          </span>
          <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
            <button
              onClick={() =>
                onSearch({ seriesId, seasonNumber: activeSeason.season_number })
              }
              style={{
                display: "flex",
                alignItems: "center",
                gap: 5,
                padding: "4px 10px",
                background: "none",
                border: "1px solid var(--color-border-default)",
                borderRadius: 6,
                cursor: "pointer",
                fontSize: 12,
                fontWeight: 500,
                color: "var(--color-text-secondary)",
              }}
              title="Search for entire season"
            >
              <Search size={13} strokeWidth={1.5} />
              Search Season
            </button>
            <button
              onClick={() =>
                updateSeasonMonitored.mutate({
                  seasonId: activeSeason.id,
                  monitored: !activeSeason.monitored,
                })
              }
              style={{
                display: "flex",
                alignItems: "center",
                gap: 6,
                padding: "4px 10px",
                background: "none",
                border: "1px solid var(--color-border-default)",
                borderRadius: 6,
                cursor: "pointer",
                fontSize: 12,
                fontWeight: 500,
                color: activeSeason.monitored ? "var(--color-accent)" : "var(--color-text-muted)",
                transition: "color 150ms ease, border-color 150ms ease",
              }}
            >
              {activeSeason.monitored
                ? <><CheckCircle2 size={14} strokeWidth={1.5} /> Monitored</>
                : <><Circle size={14} strokeWidth={1.5} /> Unmonitored</>}
            </button>
          </div>
        </div>
      )}

      {activeSeason && (
        <SeasonEpisodes
          seriesId={seriesId}
          seasonNumber={activeSeason.season_number}
          fileMap={fileMap}
          onSearch={onSearch}
        />
      )}
    </div>
  );
}

// ── Main page ─────────────────────────────────────────────────────────────────

export default function SeriesDetail() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { data: series, isLoading, isError } = useSeriesDetail(id ?? "");
  const updateSeries = useUpdateSeries(id ?? "");
  const libraryScan = useLibraryScan();
  const [searchTarget, setSearchTarget] = useState<SearchTarget | null>(null);

  const { data: episodeFiles } = useEpisodeFiles(id ?? "");
  const fileMap = new Map<string, EpisodeFile>(
    (episodeFiles ?? []).map((f) => [f.episode_id, f])
  );

  if (isLoading) {
    return (
      <div style={{ padding: "24px 28px" }}>
        <div
          className="skeleton"
          style={{ height: 220, borderRadius: 8, marginBottom: 24 }}
        />
        <div className="skeleton" style={{ height: 400, borderRadius: 8 }} />
      </div>
    );
  }

  if (isError || !series) {
    return (
      <div style={{ padding: "24px 28px" }}>
        <button
          onClick={() => navigate(-1)}
          style={{
            display: "flex",
            alignItems: "center",
            gap: 6,
            background: "none",
            border: "none",
            cursor: "pointer",
            color: "var(--color-text-secondary)",
            fontSize: 14,
            padding: 0,
            marginBottom: 20,
          }}
        >
          <ArrowLeft size={16} />
          Back
        </button>
        <div
          style={{
            padding: 20,
            background: "color-mix(in srgb, var(--color-danger) 10%, transparent)",
            border: "1px solid color-mix(in srgb, var(--color-danger) 30%, transparent)",
            borderRadius: 8,
            color: "var(--color-danger)",
            fontSize: 14,
          }}
        >
          Series not found.
        </div>
      </div>
    );
  }

  return (
    <div>
      {/* Hero banner */}
      <div
        style={{
          position: "relative",
          minHeight: 240,
          background: series.fanart_url
            ? `linear-gradient(to bottom, rgba(0,0,0,0.5) 0%, var(--color-bg-base) 100%), url(${series.fanart_url}) center/cover no-repeat`
            : "var(--color-bg-surface)",
          borderBottom: "1px solid var(--color-border-subtle)",
        }}
      >
        <div style={{ padding: "20px 28px 28px" }}>
          <button
            onClick={() => navigate(-1)}
            style={{
              display: "flex",
              alignItems: "center",
              gap: 6,
              background: "none",
              border: "none",
              cursor: "pointer",
              color: "var(--color-text-secondary)",
              fontSize: 13,
              padding: 0,
              marginBottom: 20,
            }}
          >
            <ArrowLeft size={15} />
            All Series
          </button>

          <div style={{ display: "flex", gap: 24, alignItems: "flex-start" }}>
            {/* Poster thumbnail */}
            <div style={{ width: 120, flexShrink: 0 }}>
              <Poster
                src={series.poster_url}
                title={series.title}
                year={series.year}
                loading="eager"
                style={{ borderRadius: 8, boxShadow: "0 4px 16px rgba(0,0,0,0.5)" }}
              />
            </div>

            {/* Meta */}
            <div style={{ flex: 1, minWidth: 0 }}>
              <div
                style={{
                  display: "flex",
                  alignItems: "center",
                  gap: 10,
                  flexWrap: "wrap",
                  marginBottom: 6,
                }}
              >
                <h1
                  style={{
                    margin: 0,
                    fontSize: 24,
                    fontWeight: 700,
                    color: "var(--color-text-primary)",
                    letterSpacing: "-0.02em",
                  }}
                >
                  {series.title}
                </h1>
                <StatusBadge status={series.status} />
              </div>

              <div
                style={{
                  display: "flex",
                  alignItems: "center",
                  gap: 12,
                  fontSize: 13,
                  color: "var(--color-text-secondary)",
                  marginBottom: 12,
                  flexWrap: "wrap",
                }}
              >
                <span>{series.year}</span>
                {series.network && (
                  <>
                    <span style={{ color: "var(--color-border-default)" }}>·</span>
                    <span>{series.network}</span>
                  </>
                )}
                {series.runtime_minutes && (
                  <>
                    <span style={{ color: "var(--color-border-default)" }}>·</span>
                    <span>{series.runtime_minutes}m per episode</span>
                  </>
                )}
                {series.genres.length > 0 && (
                  <>
                    <span style={{ color: "var(--color-border-default)" }}>·</span>
                    <span>{series.genres.slice(0, 3).join(", ")}</span>
                  </>
                )}
              </div>

              {/* Action row */}
              <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
                {/* Monitored toggle */}
                <button
                  onClick={() =>
                    updateSeries.mutate({ monitored: !series.monitored })
                  }
                  style={{
                    display: "inline-flex",
                    alignItems: "center",
                    gap: 6,
                    padding: "6px 12px",
                    border: "1px solid var(--color-border-default)",
                    borderRadius: 6,
                    background: series.monitored ? "var(--color-accent-muted)" : "none",
                    cursor: "pointer",
                    fontSize: 13,
                    fontWeight: 500,
                    color: series.monitored ? "var(--color-accent)" : "var(--color-text-secondary)",
                    transition: "background 150ms ease, color 150ms ease",
                  }}
                >
                  {series.monitored
                    ? <><CheckCircle2 size={15} strokeWidth={1.5} /> Monitored</>
                    : <><Tv size={15} strokeWidth={1.5} /> Monitor</>}
                </button>

                {/* Scan Library */}
                <button
                  onClick={() => libraryScan.mutate()}
                  disabled={libraryScan.isPending}
                  style={{
                    display: "inline-flex",
                    alignItems: "center",
                    gap: 6,
                    padding: "6px 12px",
                    border: "1px solid var(--color-border-default)",
                    borderRadius: 6,
                    background: "none",
                    cursor: libraryScan.isPending ? "not-allowed" : "pointer",
                    fontSize: 13,
                    fontWeight: 500,
                    color: "var(--color-text-secondary)",
                    opacity: libraryScan.isPending ? 0.6 : 1,
                    transition: "opacity 150ms ease",
                  }}
                  title="Trigger a library scan"
                >
                  <RefreshCw
                    size={15}
                    strokeWidth={1.5}
                    style={{
                      animation: libraryScan.isPending ? "spin 1s linear infinite" : "none",
                    }}
                  />
                  Scan Library
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Manual search modal */}
      {searchTarget && (
        <ManualSearchModal
          seriesId={searchTarget.seriesId}
          seasonNumber={searchTarget.seasonNumber}
          episodeNumber={searchTarget.episodeNumber}
          onClose={() => setSearchTarget(null)}
        />
      )}

      {/* Body */}
      <div style={{ padding: "24px 28px" }}>
        {/* Overview */}
        {series.overview && (
          <div
            style={{
              background: "var(--color-bg-surface)",
              border: "1px solid var(--color-border-subtle)",
              borderRadius: 8,
              padding: 20,
              marginBottom: 24,
              boxShadow: "var(--shadow-card)",
            }}
          >
            <h2
              style={{
                margin: "0 0 10px",
                fontSize: 11,
                fontWeight: 600,
                letterSpacing: "0.08em",
                textTransform: "uppercase",
                color: "var(--color-text-muted)",
              }}
            >
              Overview
            </h2>
            <p
              style={{
                margin: 0,
                fontSize: 14,
                lineHeight: 1.65,
                color: "var(--color-text-secondary)",
              }}
            >
              {series.overview}
            </p>
          </div>
        )}

        {/* Episodes section */}
        <div
          style={{
            background: "var(--color-bg-surface)",
            border: "1px solid var(--color-border-subtle)",
            borderRadius: 8,
            padding: 20,
            boxShadow: "var(--shadow-card)",
          }}
        >
          <h2
            style={{
              margin: "0 0 16px",
              fontSize: 11,
              fontWeight: 600,
              letterSpacing: "0.08em",
              textTransform: "uppercase",
              color: "var(--color-text-muted)",
            }}
          >
            Episodes
          </h2>
          <SeasonTabs seriesId={series.id} fileMap={fileMap} onSearch={setSearchTarget} />
        </div>
      </div>
    </div>
  );
}
