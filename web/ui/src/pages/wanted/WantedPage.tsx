import { useState } from "react";
import { Link } from "react-router-dom";
import { useMissingEpisodes, useCutoffUnmet, useSearchAllWanted } from "@/api/wanted";
import { useConfirm } from "@beacon-shared/ConfirmDialog";
import ManualSearchModal from "@/components/ManualSearchModal";
import type { WantedEpisode } from "@/types";

// ── Helpers ───────────────────────────────────────────────────────────────────

function formatEpisodeCode(season: number, episode: number): string {
  return `S${String(season).padStart(2, "0")}E${String(episode).padStart(2, "0")}`;
}

function formatAirDate(iso?: string): string {
  if (!iso) return "—";
  const d = new Date(iso);
  return d.toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

// ── Episode row ───────────────────────────────────────────────────────────────

function EpisodeRow({
  episode,
  onSearch,
}: {
  episode: WantedEpisode;
  onSearch: () => void;
}) {
  return (
    <tr style={{ borderBottom: "1px solid var(--color-border-subtle)" }}>
      {/* Series */}
      <td style={{ padding: "10px 14px", minWidth: 0 }}>
        <Link
          to={`/series/${episode.series_id}`}
          style={{
            fontSize: 13,
            fontWeight: 500,
            color: "var(--color-accent)",
            textDecoration: "none",
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
            display: "block",
            maxWidth: 220,
          }}
          onMouseEnter={(e) => {
            (e.currentTarget as HTMLAnchorElement).style.textDecoration =
              "underline";
          }}
          onMouseLeave={(e) => {
            (e.currentTarget as HTMLAnchorElement).style.textDecoration =
              "none";
          }}
        >
          {episode.series_title}
        </Link>
      </td>

      {/* Episode code */}
      <td
        style={{
          padding: "10px 14px",
          fontSize: 13,
          color: "var(--color-text-secondary)",
          whiteSpace: "nowrap",
          fontVariantNumeric: "tabular-nums",
        }}
      >
        {formatEpisodeCode(episode.season_number, episode.episode_number)}
      </td>

      {/* Episode title */}
      <td
        style={{
          padding: "10px 14px",
          fontSize: 13,
          color: "var(--color-text-primary)",
          minWidth: 0,
        }}
      >
        <span
          style={{
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
            display: "block",
            maxWidth: 260,
          }}
          title={episode.episode_title}
        >
          {episode.episode_title}
          {episode.current_quality && (
            <div style={{ fontSize: 11, color: "var(--color-text-muted)", marginTop: 2 }}>
              {episode.current_quality} → cutoff {episode.cutoff_quality}
            </div>
          )}
        </span>
      </td>

      {/* Air date */}
      <td
        style={{
          padding: "10px 14px",
          fontSize: 12,
          color: "var(--color-text-muted)",
          whiteSpace: "nowrap",
        }}
      >
        {formatAirDate(episode.air_date)}
      </td>

      {/* Actions */}
      <td style={{ padding: "10px 14px", width: 72 }}>
        <button
          onClick={onSearch}
          style={{
            background: "none",
            border: "1px solid var(--color-border-default)",
            borderRadius: 5,
            padding: "4px 10px",
            fontSize: 12,
            color: "var(--color-text-muted)",
            cursor: "pointer",
            whiteSpace: "nowrap",
          }}
          onMouseEnter={(e) => {
            const el = e.currentTarget as HTMLButtonElement;
            el.style.color = "var(--color-accent)";
            el.style.borderColor = "var(--color-accent)";
            el.style.background =
              "color-mix(in srgb, var(--color-accent) 8%, transparent)";
          }}
          onMouseLeave={(e) => {
            const el = e.currentTarget as HTMLButtonElement;
            el.style.color = "var(--color-text-muted)";
            el.style.borderColor = "var(--color-border-default)";
            el.style.background = "none";
          }}
        >
          Search
        </button>
      </td>
    </tr>
  );
}

// ── Table wrapper ─────────────────────────────────────────────────────────────

const COL_HEADERS = ["Series", "Episode", "Episode Title", "Air Date", "Actions"];

function EpisodeTable({
  episodes,
  onSearch,
}: {
  episodes: WantedEpisode[];
  onSearch: (ep: WantedEpisode) => void;
}) {
  return (
    <table
      style={{
        width: "100%",
        borderCollapse: "collapse",
        background: "var(--color-bg-surface)",
        border: "1px solid var(--color-border-subtle)",
        borderRadius: 8,
        overflow: "hidden",
      }}
    >
      <thead>
        <tr>
          {COL_HEADERS.map((h) => (
            <th
              key={h}
              style={{
                textAlign: "left",
                padding: "8px 14px",
                fontSize: 11,
                fontWeight: 600,
                letterSpacing: "0.06em",
                textTransform: "uppercase",
                color: "var(--color-text-muted)",
                borderBottom: "1px solid var(--color-border-subtle)",
                background: "var(--color-bg-elevated)",
                whiteSpace: "nowrap",
              }}
            >
              {h}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {episodes.map((ep) => (
          <EpisodeRow
            key={ep.episode_id}
            episode={ep}
            onSearch={() => onSearch(ep)}
          />
        ))}
      </tbody>
    </table>
  );
}

// ── Pagination ────────────────────────────────────────────────────────────────

function Pagination({
  page,
  totalPages,
  onPrev,
  onNext,
}: {
  page: number;
  totalPages: number;
  onPrev: () => void;
  onNext: () => void;
}) {
  if (totalPages <= 1) return null;
  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        gap: 8,
        marginTop: 20,
      }}
    >
      <button
        onClick={onPrev}
        disabled={page === 1}
        style={{
          background: "var(--color-bg-elevated)",
          border: "1px solid var(--color-border-default)",
          borderRadius: 6,
          padding: "6px 14px",
          fontSize: 12,
          color: page === 1 ? "var(--color-text-muted)" : "var(--color-text-primary)",
          cursor: page === 1 ? "default" : "pointer",
        }}
      >
        Previous
      </button>
      <span style={{ fontSize: 12, color: "var(--color-text-muted)" }}>
        {page} / {totalPages}
      </span>
      <button
        onClick={onNext}
        disabled={page === totalPages}
        style={{
          background: "var(--color-bg-elevated)",
          border: "1px solid var(--color-border-default)",
          borderRadius: 6,
          padding: "6px 14px",
          fontSize: 12,
          color:
            page === totalPages
              ? "var(--color-text-muted)"
              : "var(--color-text-primary)",
          cursor: page === totalPages ? "default" : "pointer",
        }}
      >
        Next
      </button>
    </div>
  );
}

// ── Missing tab ───────────────────────────────────────────────────────────────

const PER_PAGE = 50;

function MissingTab({ onSearch }: { onSearch: (ep: WantedEpisode) => void }) {
  const [page, setPage] = useState(1);
  const { data, isLoading, error } = useMissingEpisodes(page, PER_PAGE);

  if (isLoading) {
    return (
      <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
        {[...Array(8)].map((_, i) => (
          <div
            key={i}
            className="skeleton"
            style={{ height: 44, borderRadius: 6 }}
          />
        ))}
      </div>
    );
  }

  if (error) {
    return (
      <p style={{ margin: 0, fontSize: 13, color: "var(--color-danger)" }}>
        Failed to load: {(error as Error).message}
      </p>
    );
  }

  const episodes = data?.episodes ?? [];
  const total = data?.total ?? 0;
  const totalPages = Math.ceil(total / PER_PAGE);

  if (episodes.length === 0) {
    return (
      <div style={{ padding: "48px 0", textAlign: "center" }}>
        <p
          style={{
            margin: 0,
            fontSize: 15,
            fontWeight: 600,
            color: "var(--color-text-primary)",
          }}
        >
          All caught up!
        </p>
        <p
          style={{
            margin: "6px 0 0",
            fontSize: 13,
            color: "var(--color-text-muted)",
          }}
        >
          No monitored episodes are missing a file.
        </p>
      </div>
    );
  }

  return (
    <div>
      <p
        style={{
          margin: "0 0 12px",
          fontSize: 12,
          color: "var(--color-text-muted)",
        }}
      >
        {total} episode{total !== 1 ? "s" : ""} missing a file
      </p>
      <EpisodeTable episodes={episodes} onSearch={onSearch} />
      <Pagination
        page={page}
        totalPages={totalPages}
        onPrev={() => setPage((p) => Math.max(1, p - 1))}
        onNext={() => setPage((p) => Math.min(totalPages, p + 1))}
      />
    </div>
  );
}

// ── Cutoff unmet tab ──────────────────────────────────────────────────────────

function CutoffTab({ onSearch }: { onSearch: (ep: WantedEpisode) => void }) {
  const [page, setPage] = useState(1);
  const { data, isLoading, error } = useCutoffUnmet(page, PER_PAGE);

  if (isLoading) {
    return (
      <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
        {[...Array(6)].map((_, i) => (
          <div
            key={i}
            className="skeleton"
            style={{ height: 44, borderRadius: 6 }}
          />
        ))}
      </div>
    );
  }

  if (error) {
    return (
      <p style={{ margin: 0, fontSize: 13, color: "var(--color-danger)" }}>
        Failed to load: {(error as Error).message}
      </p>
    );
  }

  const episodes = data?.episodes ?? [];
  const total = data?.total ?? 0;
  const totalPages = Math.ceil(total / PER_PAGE);

  if (episodes.length === 0) {
    return (
      <div style={{ padding: "48px 0", textAlign: "center" }}>
        <p
          style={{
            margin: 0,
            fontSize: 15,
            fontWeight: 600,
            color: "var(--color-text-primary)",
          }}
        >
          All at cutoff!
        </p>
        <p
          style={{
            margin: "6px 0 0",
            fontSize: 13,
            color: "var(--color-text-muted)",
          }}
        >
          No episodes below cutoff quality.
        </p>
      </div>
    );
  }

  return (
    <div>
      <p
        style={{
          margin: "0 0 12px",
          fontSize: 12,
          color: "var(--color-text-muted)",
        }}
      >
        {total} episode{total !== 1 ? "s" : ""} below cutoff quality
      </p>
      <EpisodeTable episodes={episodes} onSearch={onSearch} />
      <Pagination
        page={page}
        totalPages={totalPages}
        onPrev={() => setPage((p) => Math.max(1, p - 1))}
        onNext={() => setPage((p) => Math.min(totalPages, p + 1))}
      />
    </div>
  );
}

// ── Page ──────────────────────────────────────────────────────────────────────

type WantedTab = "missing" | "cutoff";

interface SearchTarget {
  seriesId: string;
  seasonNumber: number;
  episodeNumber: number;
}

export default function WantedPage() {
  const [tab, setTab] = useState<WantedTab>("missing");
  const [searchTarget, setSearchTarget] = useState<SearchTarget | null>(null);
  const confirm = useConfirm();
  const searchAll = useSearchAllWanted();

  async function handleSearchAll() {
    const ok = await confirm({
      title: tab === "missing" ? "Search all missing episodes" : "Search for upgrades",
      message:
        tab === "missing"
          ? "Search every indexer for each missing episode and grab the best release your quality profiles accept. This runs in the background and can take a while."
          : "Search every indexer for an upgrade to each episode below its quality cutoff. This runs in the background and can take a while.",
      confirmLabel: "Search",
    });
    if (ok) searchAll.mutate(tab);
  }

  function handleSearch(ep: WantedEpisode) {
    setSearchTarget({
      seriesId: ep.series_id,
      seasonNumber: ep.season_number,
      episodeNumber: ep.episode_number,
    });
  }

  return (
    <div style={{ padding: 24, maxWidth: 900 }}>
      <h1
        style={{
          margin: "0 0 20px",
          fontSize: 20,
          fontWeight: 700,
          color: "var(--color-text-primary)",
          letterSpacing: "-0.02em",
        }}
      >
        Wanted
      </h1>

      {/* Tab bar */}
      <div
        style={{
          display: "flex",
          gap: 0,
          borderBottom: "1px solid var(--color-border-subtle)",
          marginBottom: 20,
        }}
      >
        {(["missing", "cutoff"] as WantedTab[]).map((t) => {
          const label = t === "missing" ? "Missing" : "Cutoff Unmet";
          return (
            <button
              key={t}
              onClick={() => setTab(t)}
              style={{
                background: "none",
                border: "none",
                borderBottom: `2px solid ${
                  tab === t ? "var(--color-accent)" : "transparent"
                }`,
                padding: "10px 18px",
                fontSize: 13,
                fontWeight: tab === t ? 600 : 400,
                color:
                  tab === t
                    ? "var(--color-accent)"
                    : "var(--color-text-muted)",
                cursor: "pointer",
                marginBottom: -1,
                transition: "color 0.1s, border-color 0.1s",
              }}
            >
              {label}
            </button>
          );
        })}
        <button
          type="button"
          onClick={handleSearchAll}
          disabled={searchAll.isPending}
          style={{
            marginLeft: "auto",
            alignSelf: "center",
            padding: "6px 12px",
            fontSize: 12,
            fontWeight: 500,
            borderRadius: 6,
            border: "1px solid var(--color-border-default)",
            background: "var(--color-bg-elevated)",
            color: "var(--color-text-primary)",
            cursor: searchAll.isPending ? "default" : "pointer",
            opacity: searchAll.isPending ? 0.6 : 1,
          }}
        >
          {tab === "missing" ? "Search all missing" : "Search all cutoff unmet"}
        </button>
      </div>

      {tab === "missing" && <MissingTab onSearch={handleSearch} />}
      {tab === "cutoff" && <CutoffTab onSearch={handleSearch} />}

      {searchTarget && (
        <ManualSearchModal
          seriesId={searchTarget.seriesId}
          seasonNumber={searchTarget.seasonNumber}
          episodeNumber={searchTarget.episodeNumber}
          onClose={() => setSearchTarget(null)}
        />
      )}
    </div>
  );
}
