// EpisodeDetail.tsx — full-page view for a single episode. Mirrors
// Prism's MovieDetail layout: backdrop, hero with still + title +
// metadata, action bar, overview, file panel, grab-history table.
//
// URL: /series/:seriesId/episodes/:episodeId
//
// The page is reachable two ways:
//   - Click the episode title in EpisodeRow on the series detail page
//     (carries cour-display state via location.state for correct
//     "S03E01" rendering on cour-mode anime)
//   - Direct deep link (no state) — falls back to TMDB-relative
//     numbering ("S01E48"); display state is enrichment, not
//     load-bearing.

import { Link, useParams, useLocation } from "react-router-dom";
import { useState } from "react";
import { ChevronLeft, Search, Zap, CheckCircle2, Circle, Trash2, RefreshCw } from "lucide-react";
import { toast } from "sonner";
import { useEpisode, useUpdateEpisodeMonitored, useSeriesDetail } from "@/api/series";
import { useEpisodeFiles, useDeleteEpisodeFile } from "@/api/episode-files";
import { useSeriesGrabHistory, useReimportGrab, useSeriesHaulHistory } from "@/api/haul";
import { useGrabRelease, useAutoSearch } from "@/api/releases";
import { formatBytes } from "@/lib/utils";
import { useConfirm } from "@/shared/ConfirmDialog";
import ManualSearchModal from "@/components/ManualSearchModal";

interface DisplayState {
  // Carried from EpisodeRow when navigating from a cour view, so the
  // page heading reads "S03E01" instead of the TMDB-relative "S01E48".
  displaySeasonNumber?: number;
  displayEpisodeNumber?: number;
}

export default function EpisodeDetail() {
  const { id: seriesId, episodeId } = useParams<{ id: string; episodeId: string }>();
  const location = useLocation();
  const displayState: DisplayState = (location.state as DisplayState | null) ?? {};

  const { data: episode, isLoading, error } = useEpisode(episodeId ?? "");
  const { data: series } = useSeriesDetail(seriesId ?? "");
  const { data: files } = useEpisodeFiles(seriesId ?? "");
  const { data: grabRows } = useSeriesGrabHistory(seriesId ?? "");
  const { data: haulRecords } = useSeriesHaulHistory(seriesId ?? "");
  const updateMonitored = useUpdateEpisodeMonitored(seriesId ?? "");
  const deleteFile = useDeleteEpisodeFile();
  const reimportGrab = useReimportGrab(seriesId ?? "");
  const grab = useGrabRelease(seriesId ?? "");
  const autoSearch = useAutoSearch(seriesId ?? "");
  const confirm = useConfirm();
  const [showManualSearch, setShowManualSearch] = useState(false);

  if (isLoading) {
    return (
      <div style={{ padding: 24 }}>
        <div className="skeleton" style={{ height: 24, width: 200, borderRadius: 4, marginBottom: 20 }} />
        <div className="skeleton" style={{ height: 240, borderRadius: 8, marginBottom: 16 }} />
        <div className="skeleton" style={{ height: 80, borderRadius: 8 }} />
      </div>
    );
  }

  if (error || !episode) {
    return (
      <div style={{ padding: 24 }}>
        <Link to={`/series/${seriesId}`} style={{ fontSize: 13, color: "var(--color-accent)", textDecoration: "none" }}>
          ← Back to series
        </Link>
        <p style={{ marginTop: 24, fontSize: 13, color: "var(--color-text-muted)" }}>
          Episode not found or failed to load.
        </p>
      </div>
    );
  }

  const file = files?.find((f) => f.episode_id === episode.id);
  const episodeGrabs = (grabRows ?? []).filter(
    (g) => g.episode_id === episode.id || (
      // Fallback for orphan rows missing episode_id — match by season +
      // a release-title episode-number parse. Same heuristic as the
      // SeasonEpisodeList orphan-grab map.
      !g.episode_id && g.season_number === episode.season_number
        && releaseTitleMentionsEpisode(g.release_title, episode.episode_number)
    ),
  );
  const haulMatches = (haulRecords ?? []).filter((r) => r.episode_id === episode.id);

  // Display labels: prefer cour-mapped values from navigation state;
  // fall back to TMDB-relative numbers from the episode itself.
  const displaySeason = displayState.displaySeasonNumber ?? episode.season_number;
  const displayEpisode = displayState.displayEpisodeNumber ?? episode.episode_number;
  const sxxexx = `S${String(displaySeason).padStart(2, "0")}E${String(displayEpisode).padStart(2, "0")}`;

  function handleDeleteFile() {
    if (!file) return;
    void (async () => {
      const ok = await confirm({
        title: "Delete episode file",
        message: `Delete ${file.path.split("/").pop()} from disk? This cannot be undone.`,
        confirmLabel: "Delete",
        danger: true,
      });
      if (!ok) return;
      deleteFile.mutate({ fileId: file.id, deleteFromDisk: true });
    })();
  }

  return (
    <div style={{ padding: 24, maxWidth: 1100 }}>
      {/* Back link */}
      <Link
        to={`/series/${seriesId}`}
        style={{
          fontSize: 13, color: "var(--color-text-muted)", textDecoration: "none",
          display: "inline-flex", alignItems: "center", gap: 4, marginBottom: 20,
        }}
        onMouseEnter={(e) => { (e.currentTarget as HTMLAnchorElement).style.color = "var(--color-text-primary)"; }}
        onMouseLeave={(e) => { (e.currentTarget as HTMLAnchorElement).style.color = "var(--color-text-muted)"; }}
      >
        <ChevronLeft size={14} />
        {series?.title ?? "Series"}
      </Link>

      {/* Hero */}
      <div style={{ display: "flex", gap: 24, marginBottom: 24, alignItems: "flex-start" }}>
        {episode.still_path && (
          <img
            src={`https://image.tmdb.org/t/p/w500${episode.still_path}`}
            alt={episode.title}
            style={{
              width: 400, aspectRatio: "16/9", objectFit: "cover",
              borderRadius: 8, flexShrink: 0,
              background: "var(--color-bg-elevated)",
            }}
          />
        )}
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ fontSize: 12, fontWeight: 500, letterSpacing: "0.06em", color: "var(--color-accent)", marginBottom: 6 }}>
            {sxxexx}
          </div>
          <h1 style={{ margin: "0 0 12px", fontSize: 28, fontWeight: 600, color: "var(--color-text-primary)", lineHeight: 1.2 }}>
            {episode.title}
          </h1>
          <div style={{ display: "flex", gap: 16, fontSize: 13, color: "var(--color-text-muted)", flexWrap: "wrap", marginBottom: 16 }}>
            {episode.air_date && (
              <span>{new Date(episode.air_date).toLocaleDateString(undefined, { weekday: "short", year: "numeric", month: "long", day: "numeric" })}</span>
            )}
            {episode.runtime_minutes != null && episode.runtime_minutes > 0 && (
              <span>{episode.runtime_minutes} min</span>
            )}
            {episode.absolute_number != null && episode.absolute_number > 0 && (
              <span>Absolute #{episode.absolute_number}</span>
            )}
          </div>

          {/* Action bar */}
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
            <button
              onClick={() => updateMonitored.mutate({ episodeId: episode.id, monitored: !episode.monitored, seasonNumber: episode.season_number })}
              style={actionBtn(episode.monitored ? "primary" : "secondary")}
            >
              {episode.monitored
                ? <><CheckCircle2 size={14} /> Monitored</>
                : <><Circle size={14} /> Unmonitored</>}
            </button>
            <button
              onClick={() => setShowManualSearch(true)}
              style={actionBtn("secondary")}
            >
              <Search size={14} /> Interactive Search
            </button>
            <button
              onClick={() => {
                const toastId = toast.loading(`Searching releases for ${sxxexx}…`);
                autoSearch.mutate(
                  {
                    season: episode.season_number,
                    episode: episode.episode_number,
                    episode_id: episode.id,
                  },
                  {
                    onSuccess: (data) => {
                      if (data.result === "grabbed") {
                        toast.success(`Grabbed: ${data.release_title}`, { id: toastId });
                      } else {
                        toast.info(data.reason ?? "No matching release found", { id: toastId });
                      }
                    },
                    onError: (err) => toast.error((err as Error).message, { id: toastId }),
                  },
                );
              }}
              style={actionBtn("secondary")}
              disabled={autoSearch.isPending}
              title="Pick the highest-ranked release and grab it automatically"
            >
              <Zap size={14} /> Auto Search
            </button>
          </div>
        </div>
      </div>

      {/* Overview */}
      {episode.overview && (
        <section style={{ marginBottom: 24 }}>
          <SectionHeading>Overview</SectionHeading>
          <p style={{ margin: 0, fontSize: 14, lineHeight: 1.6, color: "var(--color-text-secondary)" }}>
            {episode.overview}
          </p>
        </section>
      )}

      {/* Downloaded file */}
      {file && (
        <section style={{ marginBottom: 24 }}>
          <SectionHeading>Downloaded file</SectionHeading>
          <div style={{
            padding: 14, borderRadius: 8,
            background: "var(--color-bg-elevated)",
            border: "1px solid var(--color-border-subtle)",
          }}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: 16 }}>
              <div style={{ minWidth: 0, flex: 1 }}>
                <div style={{ fontFamily: "var(--font-family-mono)", fontSize: 12, color: "var(--color-text-secondary)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", marginBottom: 6 }}>
                  {file.path}
                </div>
                <div style={{ fontSize: 12, color: "var(--color-text-muted)" }}>
                  {[file.quality?.source, file.quality?.resolution, file.quality?.codec].filter(Boolean).join(" · ") || "Unknown"}
                  {" · "}{formatBytes(file.size_bytes)}
                  {file.imported_at && (
                    <> · imported {new Date(file.imported_at).toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" })}</>
                  )}
                </div>
              </div>
              <button onClick={handleDeleteFile} style={{ ...actionBtn("danger"), flexShrink: 0 }}>
                <Trash2 size={13} /> Delete file
              </button>
            </div>
          </div>
        </section>
      )}

      {/* Grab history */}
      {episodeGrabs.length > 0 && (
        <section style={{ marginBottom: 24 }}>
          <SectionHeading>Grab history</SectionHeading>
          <div style={{ border: "1px solid var(--color-border-subtle)", borderRadius: 8, overflow: "hidden" }}>
            <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}>
              <thead>
                <tr style={{ background: "var(--color-bg-elevated)" }}>
                  <th style={th}>Release</th>
                  <th style={th}>Status</th>
                  <th style={th}>Grabbed</th>
                  <th style={th}></th>
                </tr>
              </thead>
              <tbody>
                {episodeGrabs.map((g) => {
                  const isOrphan = g.download_status === "completed" && !file;
                  return (
                    <tr key={g.id} style={{ borderTop: "1px solid var(--color-border-subtle)" }}>
                      <td style={{ ...td, fontFamily: "var(--font-family-mono)", fontSize: 12, maxWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }} title={g.release_title}>
                        {g.release_title}
                      </td>
                      <td style={td}>
                        <StatusPill status={g.download_status} />
                      </td>
                      <td style={{ ...td, color: "var(--color-text-muted)", fontSize: 12 }}>
                        {new Date(g.grabbed_at).toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" })}
                      </td>
                      <td style={{ ...td, textAlign: "right" }}>
                        {isOrphan && (
                          <button
                            onClick={() => reimportGrab.mutate(g.id)}
                            style={actionBtn("secondary")}
                            title="Run the importer against the file in Haul"
                          >
                            <RefreshCw size={12} /> Import
                          </button>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </section>
      )}

      {/* Haul records — if any matched */}
      {haulMatches.length > 0 && (
        <section style={{ marginBottom: 24 }}>
          <SectionHeading>Haul records</SectionHeading>
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            {haulMatches.map((r) => (
              <div key={r.info_hash} style={{
                padding: 12, borderRadius: 8,
                background: "var(--color-bg-elevated)",
                border: "1px solid var(--color-border-subtle)",
              }}>
                <div style={{ fontFamily: "var(--font-family-mono)", fontSize: 11, color: "var(--color-text-muted)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                  {r.name}
                </div>
                <div style={{ fontSize: 12, color: "var(--color-text-muted)", marginTop: 4 }}>
                  {r.completed_at ? `Completed ${new Date(r.completed_at).toLocaleDateString()}` : "In progress"}
                  {r.removed_at && " · removed"}
                </div>
              </div>
            ))}
          </div>
        </section>
      )}

      {showManualSearch && (
        <ManualSearchModal
          seriesId={seriesId ?? ""}
          seasonNumber={episode.season_number}
          episodeNumber={episode.episode_number}
          episodeId={episode.id}
          displaySeasonNumber={displayState.displaySeasonNumber}
          displayEpisodeNumber={displayState.displayEpisodeNumber}
          onClose={() => {
            setShowManualSearch(false);
            // grab.mutate's onSuccess refresh handles state refresh;
            // nothing else to do here.
            void grab;
          }}
        />
      )}
    </div>
  );
}

function SectionHeading({ children }: { children: React.ReactNode }) {
  return (
    <h2 style={{
      margin: "0 0 10px",
      fontSize: 11, fontWeight: 600, letterSpacing: "0.08em",
      textTransform: "uppercase", color: "var(--color-text-muted)",
    }}>
      {children}
    </h2>
  );
}

function StatusPill({ status }: { status: string }) {
  const colorMap: Record<string, string> = {
    completed: "var(--color-success)",
    queued: "var(--color-text-muted)",
    downloading: "var(--color-accent)",
    failed: "var(--color-danger)",
    stalled: "var(--color-warning)",
    removed: "var(--color-text-muted)",
  };
  const color = colorMap[status] ?? "var(--color-text-muted)";
  return (
    <span style={{
      display: "inline-flex", alignItems: "center",
      padding: "2px 8px", borderRadius: 4, fontSize: 11, fontWeight: 500,
      background: `color-mix(in srgb, ${color} 12%, transparent)`,
      color,
    }}>
      {status}
    </span>
  );
}

const th: React.CSSProperties = {
  textAlign: "left", padding: "8px 12px",
  fontSize: 11, fontWeight: 600,
  letterSpacing: "0.06em", textTransform: "uppercase",
  color: "var(--color-text-muted)",
  borderBottom: "1px solid var(--color-border-subtle)",
};

const td: React.CSSProperties = {
  padding: "10px 12px",
};

function actionBtn(variant: "primary" | "secondary" | "danger"): React.CSSProperties {
  const base: React.CSSProperties = {
    display: "inline-flex", alignItems: "center", gap: 6,
    padding: "6px 12px", borderRadius: 6,
    fontSize: 13, fontWeight: 500,
    cursor: "pointer", border: "1px solid var(--color-border-default)",
    background: "var(--color-bg-elevated)",
    color: "var(--color-text-secondary)",
  };
  if (variant === "primary") {
    return { ...base, background: "var(--color-accent-muted)", color: "var(--color-accent)", borderColor: "var(--color-accent)" };
  }
  if (variant === "danger") {
    return { ...base, color: "var(--color-danger)" };
  }
  return base;
}

// releaseTitleMentionsEpisode is a tiny helper for the per-episode
// orphan-grab fallback. Match the same patterns as
// parseEpisodeFromReleaseTitle in SeriesDetail.tsx — duplicated here
// to keep this file self-contained, but if a third caller appears the
// helper should move into src/lib.
function releaseTitleMentionsEpisode(title: string, episode: number): boolean {
  const sxx = title.match(/[Ss]\d{1,2}[Ee](\d{1,3})/);
  if (sxx && parseInt(sxx[1], 10) === episode) return true;
  const dash = title.match(/\s-\s+(\d{1,3})(?:v\d+)?\s+[(\[]/);
  if (dash && parseInt(dash[1], 10) === episode) return true;
  return false;
}
