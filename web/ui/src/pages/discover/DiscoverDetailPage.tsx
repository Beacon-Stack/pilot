import { useState } from "react";
import { useParams, useNavigate, Link } from "react-router-dom";
import { Star, Calendar, Tv, Loader2 } from "lucide-react";
import { toast } from "sonner";
import { useDiscoverSeries } from "@/api/discover";
import { useAddSeries, tmdbPosterURL } from "@/api/series";
import { useLibraries } from "@/api/libraries";
import { useQualityProfiles } from "@/api/quality";
import { apiFetch } from "@/api/client";
import Modal from "@beacon-shared/Modal";
import ManualSearchModal from "@/components/ManualSearchModal";
import type { Series } from "@/types";

type AddAction = "none" | "interactive" | "auto";

// Season-1 search default for newly-added series — mirrors Sonarr's
// "search on add" semantic. Further per-season searches happen from the
// series detail page once the user lands there.
const SEARCH_ON_ADD_SEASON = 1;

interface AutoSearchResponse {
  result: "grabbed" | "no_match";
  release_title?: string;
  reason?: string;
}

const MONITOR_TYPES = [
  { value: "all", label: "All Episodes" },
  { value: "future", label: "Future Only" },
  { value: "missing", label: "Missing Only" },
  { value: "none", label: "None" },
  { value: "pilot", label: "Pilot Only" },
  { value: "first_season", label: "First Season" },
  { value: "last_season", label: "Latest Season" },
];

const SERIES_TYPES = [
  { value: "standard", label: "Standard" },
  { value: "daily", label: "Daily" },
  { value: "anime", label: "Anime" },
];

// Add Series Modal
// Preserves the full add form from the old AddSeriesDrawer: library, quality
// profile, monitor type, series type, and monitored. The chosen action
// (none/interactive/auto) is carried through and handled by the page.

function QuickAddSeriesModal({
  series,
  action,
  onClose,
  onAdded,
}: {
  series: { tmdb_id: number; title: string; year: number };
  action: AddAction;
  onClose: () => void;
  onAdded: (seriesId: string, action: AddAction) => void;
}) {
  const { data: libraries } = useLibraries();
  const { data: profiles } = useQualityProfiles();
  const addSeries = useAddSeries();

  const [libraryId, setLibraryId] = useState("");
  const [profileId, setProfileId] = useState("");
  const [monitorType, setMonitorType] = useState("all");
  const [seriesType, setSeriesType] = useState("standard");
  const [monitored, setMonitored] = useState(true);

  // Set defaults when data loads.
  if (libraries?.length && !libraryId) setLibraryId(libraries[0].id);

  const submitLabel =
    action === "interactive" ? "Add & Interactive Search"
    : action === "auto" ? "Add & Auto Search"
    : "Add Series";

  function handleAdd() {
    if (!libraryId) { toast.error("Select a library first"); return; }
    addSeries.mutate(
      {
        tmdb_id: series.tmdb_id,
        library_id: libraryId,
        quality_profile_id: profileId,
        monitored,
        monitor_type: monitorType,
        series_type: seriesType,
      },
      {
        onSuccess: (added: Series) => {
          toast.success(`${series.title} added to library`);
          onAdded(added.id, action);
        },
        onError: (err) => {
          const msg = (err as Error).message;
          if (msg.includes("409") || msg.toLowerCase().includes("already")) {
            toast.error("This series is already in your library");
          } else {
            toast.error(msg);
          }
        },
      }
    );
  }

  const inputStyle: React.CSSProperties = {
    width: "100%",
    padding: "8px 10px",
    borderRadius: 6,
    border: "1px solid var(--color-border-default)",
    background: "var(--color-bg-elevated)",
    color: "var(--color-text-primary)",
    fontSize: 13,
  };
  const labelStyle: React.CSSProperties = {
    display: "block",
    fontSize: 12,
    fontWeight: 500,
    color: "var(--color-text-secondary)",
    marginBottom: 4,
  };

  return (
    <Modal onClose={onClose} width={420}>
      <div style={{ padding: 24 }}>
        <h3 style={{ margin: "0 0 4px", fontSize: 16, fontWeight: 600, color: "var(--color-text-primary)" }}>
          Add {series.title}
        </h3>
        <p style={{ margin: "0 0 20px", fontSize: 12, color: "var(--color-text-muted)" }}>
          {series.year > 0 && series.year}
        </p>

        <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
          <div>
            <label style={labelStyle}>Library</label>
            {!libraries?.length ? (
              <div style={{ fontSize: 12, color: "var(--color-warning)" }}>
                No libraries configured.{" "}
                <Link to="/settings/libraries" style={{ color: "var(--color-accent)" }}>Create one first</Link>
              </div>
            ) : (
              <select value={libraryId} onChange={(e) => setLibraryId(e.target.value)} style={inputStyle}>
                {libraries.map((l) => (
                  <option key={l.id} value={l.id}>{l.name}</option>
                ))}
              </select>
            )}
          </div>
          <div>
            <label style={labelStyle}>Quality Profile</label>
            <select value={profileId} onChange={(e) => setProfileId(e.target.value)} style={inputStyle}>
              <option value="">Any Quality</option>
              {profiles?.map((p) => (
                <option key={p.id} value={p.id}>{p.name}</option>
              ))}
            </select>
          </div>
          <div>
            <label style={labelStyle}>Monitor</label>
            <select value={monitorType} onChange={(e) => setMonitorType(e.target.value)} style={inputStyle}>
              {MONITOR_TYPES.map((m) => (
                <option key={m.value} value={m.value}>{m.label}</option>
              ))}
            </select>
          </div>
          <div>
            <label style={labelStyle}>Series Type</label>
            <select value={seriesType} onChange={(e) => setSeriesType(e.target.value)} style={inputStyle}>
              {SERIES_TYPES.map((t) => (
                <option key={t.value} value={t.value}>{t.label}</option>
              ))}
            </select>
          </div>
          <label style={{ display: "flex", alignItems: "center", gap: 8, cursor: "pointer", fontSize: 13, color: "var(--color-text-secondary)" }}>
            <input type="checkbox" checked={monitored} onChange={(e) => setMonitored(e.target.checked)} style={{ accentColor: "var(--color-accent)" }} />
            Start monitoring immediately
          </label>
        </div>

        <div style={{ display: "flex", justifyContent: "flex-end", gap: 8, marginTop: 20 }}>
          <button
            onClick={onClose}
            style={{
              padding: "7px 14px",
              borderRadius: 6,
              border: "1px solid var(--color-border-default)",
              background: "transparent",
              color: "var(--color-text-secondary)",
              fontSize: 13,
              cursor: "pointer",
            }}
          >
            Cancel
          </button>
          <button
            onClick={handleAdd}
            disabled={!libraryId || addSeries.isPending}
            style={{
              padding: "7px 16px",
              borderRadius: 6,
              border: "none",
              background: !libraryId || addSeries.isPending ? "var(--color-bg-subtle)" : "var(--color-accent)",
              color: !libraryId || addSeries.isPending ? "var(--color-text-muted)" : "var(--color-accent-fg)",
              fontSize: 13,
              fontWeight: 500,
              cursor: !libraryId || addSeries.isPending ? "not-allowed" : "pointer",
              display: "flex",
              alignItems: "center",
              gap: 6,
            }}
          >
            {addSeries.isPending
              ? <><Loader2 size={14} style={{ animation: "spin 1s linear infinite" }} /> Adding…</>
              : submitLabel}
          </button>
        </div>
      </div>
    </Modal>
  );
}

// Page

export default function DiscoverDetailPage() {
  const { tmdbId } = useParams<{ tmdbId: string }>();
  const navigate = useNavigate();
  const { data: series, isLoading, error } = useDiscoverSeries(Number(tmdbId) || 0);
  const [pendingAction, setPendingAction] = useState<AddAction | null>(null);
  const [manualSearchFor, setManualSearchFor] = useState<{ seriesId: string } | null>(null);
  const [autoBusy, setAutoBusy] = useState(false);

  async function runAutoSearch(seriesId: string) {
    setAutoBusy(true);
    try {
      const res = await apiFetch<AutoSearchResponse>(
        `/series/${seriesId}/auto-search`,
        { method: "POST", body: JSON.stringify({ season: SEARCH_ON_ADD_SEASON }) }
      );
      if (res.result === "grabbed") {
        toast.success(`Grabbed: ${res.release_title ?? "release"}`);
      } else {
        toast.info(res.reason ?? "No suitable release found for season 1");
      }
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setAutoBusy(false);
    }
  }

  function handleAdded(seriesId: string, action: AddAction) {
    setPendingAction(null);
    if (action === "interactive") {
      setManualSearchFor({ seriesId });
      return;
    }
    if (action === "auto") {
      void runAutoSearch(seriesId);
    }
  }

  if (isLoading) {
    return (
      <div style={{ padding: 24, display: "flex", flexDirection: "column", gap: 20 }}>
        <div className="skeleton" style={{ height: 16, width: 80, borderRadius: 4 }} />
        <div className="skeleton" style={{ height: 24, width: 300, borderRadius: 4 }} />
        <div style={{ display: "flex", gap: 24 }}>
          <div className="skeleton" style={{ width: 200, height: 300, borderRadius: 8, flexShrink: 0 }} />
          <div style={{ flex: 1, display: "flex", flexDirection: "column", gap: 12 }}>
            {[1, 2, 3, 4].map((i) => (
              <div key={i} className="skeleton" style={{ height: 20, borderRadius: 4 }} />
            ))}
          </div>
        </div>
      </div>
    );
  }

  if (error || !series) {
    return (
      <div style={{ padding: 24 }}>
        <Link to="/discover" style={{ fontSize: 13, color: "var(--color-accent)", textDecoration: "none" }}>
          ← Discover
        </Link>
        <p style={{ marginTop: 24, fontSize: 13, color: "var(--color-text-muted)" }}>
          Series not found or failed to load.
        </p>
      </div>
    );
  }

  const posterSrc = tmdbPosterURL(series.poster_path, "w342");

  return (
    <div style={{ padding: 24, maxWidth: 1000, position: "relative" }}>
      {/* Backdrop */}
      {series.backdrop_path && (
        <div
          style={{
            position: "absolute",
            top: 0,
            left: 0,
            right: 0,
            height: 280,
            backgroundImage: `url(https://image.tmdb.org/t/p/w1280${series.backdrop_path})`,
            backgroundSize: "cover",
            backgroundPosition: "center top",
            opacity: 0.12,
            maskImage: "linear-gradient(to bottom, black 40%, transparent 100%)",
            WebkitMaskImage: "linear-gradient(to bottom, black 40%, transparent 100%)",
            pointerEvents: "none",
            borderRadius: 12,
          }}
        />
      )}

      {/* Back link */}
      <Link
        to="/discover"
        style={{ fontSize: 13, color: "var(--color-text-muted)", textDecoration: "none", display: "inline-block", marginBottom: 20 }}
        onMouseEnter={(e) => { (e.currentTarget as HTMLAnchorElement).style.color = "var(--color-text-primary)"; }}
        onMouseLeave={(e) => { (e.currentTarget as HTMLAnchorElement).style.color = "var(--color-text-muted)"; }}
      >
        ← Discover
      </Link>

      {/* Header row */}
      <div style={{ display: "flex", alignItems: "flex-start", justifyContent: "space-between", marginBottom: 24, gap: 16 }}>
        <div>
          <h1 style={{ margin: 0, fontSize: 22, fontWeight: 700, color: "var(--color-text-primary)", letterSpacing: "-0.02em" }}>
            {series.title}
          </h1>
          {series.year > 0 && (
            <p style={{ margin: "2px 0 0", fontSize: 14, color: "var(--color-text-muted)" }}>{series.year}</p>
          )}
        </div>
        <div style={{ flexShrink: 0 }}>
          {series.in_library ? (
            <button
              onClick={() => series.library_series_id && navigate(`/series/${series.library_series_id}`)}
              style={{
                padding: "7px 16px",
                borderRadius: 6,
                border: "1px solid var(--color-success)",
                background: "color-mix(in srgb, var(--color-success) 12%, transparent)",
                color: "var(--color-success)",
                fontSize: 13,
                fontWeight: 600,
                cursor: "pointer",
              }}
            >
              In Library →
            </button>
          ) : (
            <div style={{ display: "flex", gap: 8 }}>
              <button
                onClick={() => setPendingAction("none")}
                style={{
                  padding: "7px 16px",
                  borderRadius: 6,
                  border: "none",
                  background: "var(--color-accent)",
                  color: "var(--color-accent-fg)",
                  fontSize: 13,
                  fontWeight: 600,
                  cursor: "pointer",
                }}
              >
                + Add to Library
              </button>
              <button
                onClick={() => setPendingAction("interactive")}
                style={{
                  padding: "7px 14px",
                  borderRadius: 6,
                  border: "1px solid var(--color-border-default)",
                  background: "var(--color-bg-elevated)",
                  color: "var(--color-text-secondary)",
                  fontSize: 13,
                  fontWeight: 500,
                  cursor: "pointer",
                }}
              >
                Interactive Search
              </button>
              <button
                onClick={() => setPendingAction("auto")}
                disabled={autoBusy}
                style={{
                  padding: "7px 14px",
                  borderRadius: 6,
                  border: "1px solid var(--color-border-default)",
                  background: "var(--color-bg-elevated)",
                  color: "var(--color-text-secondary)",
                  fontSize: 13,
                  fontWeight: 500,
                  cursor: autoBusy ? "not-allowed" : "pointer",
                  opacity: autoBusy ? 0.6 : 1,
                }}
              >
                {autoBusy ? "Searching…" : "Auto Search"}
              </button>
            </div>
          )}
        </div>
      </div>

      {/* Main layout — poster + content */}
      <div style={{ display: "flex", gap: 24, alignItems: "flex-start" }}>
        {/* Poster */}
        <div style={{ flexShrink: 0, width: 200 }}>
          {posterSrc ? (
            <img
              src={posterSrc}
              alt={series.title}
              loading="eager"
              style={{ width: "100%", borderRadius: 8, boxShadow: "var(--shadow-modal)" }}
            />
          ) : (
            <div style={{ width: "100%", aspectRatio: "2/3", borderRadius: 8, background: "var(--color-bg-elevated)", display: "flex", alignItems: "center", justifyContent: "center", color: "var(--color-text-muted)", fontSize: 12 }}>
              No Poster
            </div>
          )}
        </div>

        {/* Content */}
        <div style={{ flex: 1, minWidth: 0 }}>
          {/* Quick facts */}
          <div style={{ display: "flex", gap: 16, flexWrap: "wrap", marginBottom: 20 }}>
            {series.rating > 0 && (
              <div style={{ display: "flex", alignItems: "center", gap: 5, fontSize: 13, color: "#fbbf24" }}>
                <Star size={14} fill="#fbbf24" stroke="none" />
                <span style={{ fontWeight: 600 }}>{series.rating.toFixed(1)}</span>
              </div>
            )}
            {series.network && (
              <div style={{ display: "flex", alignItems: "center", gap: 5, fontSize: 13, color: "var(--color-text-muted)" }}>
                <Tv size={13} />
                {series.network}
              </div>
            )}
            {series.first_air_date && (
              <div style={{ display: "flex", alignItems: "center", gap: 5, fontSize: 13, color: "var(--color-text-muted)" }}>
                <Calendar size={13} />
                {series.first_air_date}
              </div>
            )}
            {series.status && (
              <span style={{
                fontSize: 11,
                padding: "2px 8px",
                borderRadius: 4,
                background: series.status === "continuing"
                  ? "color-mix(in srgb, var(--color-success) 15%, transparent)"
                  : "var(--color-bg-subtle)",
                color: series.status === "continuing" ? "var(--color-success)" : "var(--color-text-muted)",
                fontWeight: 600,
                textTransform: "capitalize",
              }}>
                {series.status}
              </span>
            )}
          </div>

          {/* Genres */}
          {series.genres?.length > 0 && (
            <div style={{ display: "flex", gap: 6, flexWrap: "wrap", marginBottom: 16 }}>
              {series.genres.map((g) => (
                <span
                  key={g}
                  style={{
                    fontSize: 11,
                    padding: "3px 10px",
                    borderRadius: 4,
                    background: "var(--color-bg-elevated)",
                    border: "1px solid var(--color-border-subtle)",
                    color: "var(--color-text-secondary)",
                    fontWeight: 500,
                  }}
                >
                  {g}
                </span>
              ))}
            </div>
          )}

          {/* Overview */}
          {series.overview && (
            <p style={{ margin: "0 0 24px", fontSize: 14, lineHeight: 1.7, color: "var(--color-text-secondary)" }}>
              {series.overview}
            </p>
          )}

          {/* Seasons */}
          {series.seasons?.length > 0 && (
            <div style={{ marginBottom: 24 }}>
              <h3 style={{ margin: "0 0 10px", fontSize: 13, fontWeight: 600, color: "var(--color-text-primary)", textTransform: "uppercase", letterSpacing: "0.04em" }}>
                Seasons
              </h3>
              <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
                {series.seasons.map((s) => (
                  <div
                    key={s.season_number}
                    style={{
                      padding: "8px 12px",
                      borderRadius: 6,
                      background: "var(--color-bg-elevated)",
                      border: "1px solid var(--color-border-subtle)",
                      minWidth: 96,
                    }}
                  >
                    <div style={{ fontSize: 13, fontWeight: 600, color: "var(--color-text-primary)" }}>
                      {s.season_number === 0 ? "Specials" : `Season ${s.season_number}`}
                    </div>
                    <div style={{ fontSize: 11, color: "var(--color-text-muted)", marginTop: 2 }}>
                      {s.episode_count} {s.episode_count === 1 ? "episode" : "episodes"}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Crew */}
          {series.crew?.length > 0 && (
            <div style={{ marginBottom: 24 }}>
              <h3 style={{ margin: "0 0 10px", fontSize: 13, fontWeight: 600, color: "var(--color-text-primary)", textTransform: "uppercase", letterSpacing: "0.04em" }}>
                Crew
              </h3>
              <div style={{ display: "flex", gap: 16, flexWrap: "wrap" }}>
                {series.crew.map((c) => (
                  <div key={`${c.id}-${c.job}`} style={{ minWidth: 100 }}>
                    <div style={{ fontSize: 13, fontWeight: 500, color: "var(--color-text-primary)" }}>{c.name}</div>
                    <div style={{ fontSize: 11, color: "var(--color-text-muted)" }}>{c.job}</div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Cast */}
          {series.cast?.length > 0 && (
            <div style={{ marginBottom: 24 }}>
              <h3 style={{ margin: "0 0 10px", fontSize: 13, fontWeight: 600, color: "var(--color-text-primary)", textTransform: "uppercase", letterSpacing: "0.04em" }}>
                Cast
              </h3>
              <div style={{ display: "flex", gap: 12, flexWrap: "wrap" }}>
                {series.cast.map((c) => (
                  <Link
                    key={c.id}
                    to={`/people/${c.id}`}
                    style={{
                      textDecoration: "none",
                      color: "inherit",
                      display: "flex",
                      alignItems: "center",
                      gap: 8,
                      padding: "6px 10px",
                      borderRadius: 6,
                      background: "var(--color-bg-elevated)",
                      border: "1px solid var(--color-border-subtle)",
                    }}
                  >
                    {c.profile_path ? (
                      <img
                        src={`https://image.tmdb.org/t/p/w45${c.profile_path}`}
                        alt={c.name}
                        style={{ width: 28, height: 28, borderRadius: "50%", objectFit: "cover" }}
                      />
                    ) : (
                      <div style={{ width: 28, height: 28, borderRadius: "50%", background: "var(--color-bg-subtle)" }} />
                    )}
                    <div>
                      <div style={{ fontSize: 12, fontWeight: 500, color: "var(--color-text-primary)" }}>{c.name}</div>
                      <div style={{ fontSize: 10, color: "var(--color-text-muted)" }}>{c.character}</div>
                    </div>
                  </Link>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Recommendations */}
      {series.recommendations?.length > 0 && (
        <div style={{ marginTop: 32 }}>
          <h3 style={{ margin: "0 0 14px", fontSize: 13, fontWeight: 600, color: "var(--color-text-primary)", textTransform: "uppercase", letterSpacing: "0.04em" }}>
            Recommended
          </h3>
          <div
            style={{
              display: "grid",
              gridTemplateColumns: "repeat(auto-fill, minmax(110px, 1fr))",
              gap: 14,
            }}
          >
            {series.recommendations.map((rec) => {
              const recPoster = tmdbPosterURL(rec.poster_path, "w185");
              return (
                <Link
                  key={rec.tmdb_id}
                  to={`/discover/${rec.tmdb_id}`}
                  style={{ textDecoration: "none", color: "inherit" }}
                >
                  {recPoster ? (
                    <img src={recPoster} alt={rec.title} loading="lazy" style={{ width: "100%", borderRadius: 6, aspectRatio: "2/3", objectFit: "cover" }} />
                  ) : (
                    <div style={{ width: "100%", aspectRatio: "2/3", borderRadius: 6, background: "var(--color-bg-elevated)" }} />
                  )}
                  <div style={{ marginTop: 6 }}>
                    <span style={{ display: "block", fontSize: 11, fontWeight: 500, color: "var(--color-text-primary)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                      {rec.title}
                    </span>
                    {rec.year > 0 && (
                      <span style={{ fontSize: 10, color: "var(--color-text-muted)" }}>{rec.year}</span>
                    )}
                  </div>
                </Link>
              );
            })}
          </div>
        </div>
      )}

      {/* Add modal */}
      {pendingAction !== null && (
        <QuickAddSeriesModal
          series={{ tmdb_id: series.tmdb_id, title: series.title, year: series.year }}
          action={pendingAction}
          onClose={() => setPendingAction(null)}
          onAdded={handleAdded}
        />
      )}

      {/* Interactive search modal — opens after an interactive add succeeds */}
      {manualSearchFor && (
        <ManualSearchModal
          seriesId={manualSearchFor.seriesId}
          seasonNumber={SEARCH_ON_ADD_SEASON}
          displaySeasonNumber={SEARCH_ON_ADD_SEASON}
          onClose={() => setManualSearchFor(null)}
        />
      )}
    </div>
  );
}
