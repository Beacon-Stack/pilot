import { useState, useMemo } from "react";
import { Search, Loader2, AlertTriangle } from "lucide-react";
import { Link } from "react-router-dom";
import { useLookupSeries, useLibraryTmdbIds } from "@/api/series";
import {
  useDiscover,
  useDiscoverByGenre,
  useGenreList,
  type DiscoverCategory,
} from "@/api/discover";
import { APIError } from "@beacon-shared/api";
import DiscoverCard from "./DiscoverCard";

type Tab = DiscoverCategory | "genre";

const TABS: { value: Tab; label: string }[] = [
  { value: "trending", label: "Trending" },
  { value: "popular", label: "Popular" },
  { value: "top-rated", label: "Top Rated" },
  { value: "on-the-air", label: "On The Air" },
  { value: "genre", label: "By Genre" },
];

export default function DiscoverPage() {
  const [query, setQuery] = useState("");
  const [tab, setTab] = useState<Tab>("trending");
  const [page, setPage] = useState(1);
  const [genreId, setGenreId] = useState(0);

  // Browse mode is active whenever the search box isn't holding a real query.
  const searching = query.trim().length >= 3;

  const { data: results, isLoading, isFetching, error } = useLookupSeries(query);
  const { data: tmdbIds } = useLibraryTmdbIds();

  // Browse queries — disabled (page 0) while searching so they don't fire.
  const listCategory: DiscoverCategory = tab === "genre" ? "trending" : tab;
  const listQuery = useDiscover(
    listCategory,
    !searching && tab !== "genre" ? page : 0
  );
  const genreQuery = useDiscoverByGenre(
    !searching && tab === "genre" ? genreId : 0,
    !searching && tab === "genre" ? page : 0
  );
  const { data: genres } = useGenreList();

  const browseQuery = tab === "genre" ? genreQuery : listQuery;

  // Surface "metadata provider not configured" (503) as an actionable
  // empty state rather than a silent zero-results screen.
  const lookupConfigError =
    error instanceof APIError && error.status === 503 ? error : null;

  const addedSet = useMemo(() => new Set(tmdbIds ?? []), [tmdbIds]);

  const showSearchLoading = searching && (isLoading || isFetching);

  function switchTab(t: Tab) {
    setTab(t);
    setPage(1);
  }

  return (
    <div style={{ padding: 24, maxWidth: 1200 }}>
      {/* Header */}
      <div style={{ marginBottom: 20 }}>
        <h1 style={{ margin: 0, fontSize: 20, fontWeight: 600, color: "var(--color-text-primary)" }}>
          Discover
        </h1>
        <p style={{ margin: "4px 0 0", fontSize: 13, color: "var(--color-text-secondary)" }}>
          Browse and search for TV series to add to your library
        </p>
      </div>

      {/* Search bar */}
      <div style={{ position: "relative", maxWidth: 560, marginBottom: 24 }}>
        <Search
          size={16}
          style={{
            position: "absolute", left: 14, top: "50%", transform: "translateY(-50%)",
            color: "var(--color-text-muted)", pointerEvents: "none",
          }}
        />
        <input
          type="text"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search TV series by name..."
          autoFocus
          style={{
            width: "100%", padding: "11px 14px 11px 40px", borderRadius: 8,
            border: "1px solid var(--color-border-default)",
            background: "var(--color-bg-elevated)", color: "var(--color-text-primary)",
            fontSize: 14, outline: "none",
          }}
        />
        {showSearchLoading && (
          <Loader2
            size={16}
            style={{
              position: "absolute", right: 14, top: "50%", transform: "translateY(-50%)",
              color: "var(--color-accent)", animation: "spin 1s linear infinite",
            }}
          />
        )}
      </div>

      {searching ? (
        /* ── Search results ────────────────────────────────────────────── */
        lookupConfigError ? (
          <ConfigErrorState err={lookupConfigError} />
        ) : showSearchLoading && !results?.length ? (
          <LoadingGrid />
        ) : results && results.length === 0 ? (
          <div style={{ textAlign: "center", padding: "60px 20px" }}>
            <div style={{ color: "var(--color-text-secondary)", fontSize: 14, fontWeight: 500 }}>No series found</div>
            <div style={{ color: "var(--color-text-muted)", fontSize: 13, marginTop: 4 }}>Try a different search term</div>
          </div>
        ) : results && results.length > 0 ? (
          <Grid>
            {results.map((r) => (
              <DiscoverCard key={r.tmdb_id} result={r} isAdded={addedSet.has(r.tmdb_id)} />
            ))}
          </Grid>
        ) : null
      ) : (
        /* ── Browse ────────────────────────────────────────────────────── */
        <>
          {/* Tab pills + genre dropdown */}
          <div style={{ display: "flex", gap: 6, marginBottom: 20, flexWrap: "wrap", alignItems: "center" }}>
            {TABS.map((t) => {
              const active = tab === t.value;
              return (
                <button
                  key={t.value}
                  onClick={() => switchTab(t.value)}
                  style={{
                    padding: "5px 12px",
                    borderRadius: 6,
                    border: active ? "1px solid var(--color-accent)" : "1px solid var(--color-border-default)",
                    background: active ? "color-mix(in srgb, var(--color-accent) 12%, transparent)" : "transparent",
                    color: active ? "var(--color-accent)" : "var(--color-text-secondary)",
                    fontSize: 12,
                    fontWeight: 500,
                    cursor: "pointer",
                  }}
                >
                  {t.label}
                </button>
              );
            })}

            {tab === "genre" && genres && (
              <select
                value={genreId}
                onChange={(e) => {
                  setGenreId(Number(e.target.value));
                  setPage(1);
                }}
                style={{
                  padding: "5px 10px",
                  borderRadius: 6,
                  border: "1px solid var(--color-border-default)",
                  background: "var(--color-bg-elevated)",
                  color: "var(--color-text-primary)",
                  fontSize: 12,
                }}
              >
                <option value={0}>Select genre…</option>
                {genres.map((g) => (
                  <option key={g.id} value={g.id}>{g.name}</option>
                ))}
              </select>
            )}
          </div>

          {browseQuery.error instanceof APIError && browseQuery.error.status === 503 ? (
            <ConfigErrorState err={browseQuery.error} />
          ) : browseQuery.isLoading ? (
            <LoadingGrid />
          ) : tab === "genre" && genreId === 0 ? (
            <div style={{ textAlign: "center", padding: "60px 20px", color: "var(--color-text-muted)", fontSize: 13 }}>
              Select a genre to browse
            </div>
          ) : browseQuery.isError ? (
            <div style={{ textAlign: "center", padding: "60px 20px", color: "var(--color-danger)", fontSize: 13 }}>
              Failed to load. Check that TMDB is configured.
            </div>
          ) : browseQuery.data && browseQuery.data.results.length === 0 ? (
            <div style={{ textAlign: "center", padding: "60px 20px", color: "var(--color-text-muted)", fontSize: 13 }}>
              No series found
            </div>
          ) : browseQuery.data ? (
            <>
              <Grid>
                {browseQuery.data.results.map((r) => (
                  <DiscoverCard key={r.tmdb_id} result={r} isAdded={addedSet.has(r.tmdb_id)} />
                ))}
              </Grid>

              {browseQuery.data.total_pages > 1 && (
                <div style={{ display: "flex", justifyContent: "center", gap: 12, marginTop: 32 }}>
                  <PageButton disabled={page <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>
                    Previous
                  </PageButton>
                  <span style={{ fontSize: 12, color: "var(--color-text-muted)", alignSelf: "center" }}>
                    Page {browseQuery.data.page} of {browseQuery.data.total_pages}
                  </span>
                  <PageButton
                    disabled={page >= browseQuery.data.total_pages}
                    onClick={() => setPage((p) => Math.min(browseQuery.data!.total_pages, p + 1))}
                  >
                    Next
                  </PageButton>
                </div>
              )}
            </>
          ) : null}
        </>
      )}
    </div>
  );
}

// ── Small presentational helpers ──────────────────────────────────────────────

function Grid({ children }: { children: React.ReactNode }) {
  return (
    <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(180px, 1fr))", gap: 16 }}>
      {children}
    </div>
  );
}

function LoadingGrid() {
  return (
    <Grid>
      {Array.from({ length: 18 }).map((_, i) => (
        <div key={i}>
          <div className="skeleton" style={{ paddingBottom: "150%", borderRadius: 8 }} />
          <div className="skeleton" style={{ height: 14, width: "80%", marginTop: 8, borderRadius: 4 }} />
        </div>
      ))}
    </Grid>
  );
}

function PageButton({
  disabled,
  onClick,
  children,
}: {
  disabled: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      style={{
        padding: "6px 14px",
        borderRadius: 6,
        border: "1px solid var(--color-border-default)",
        background: "transparent",
        color: disabled ? "var(--color-text-muted)" : "var(--color-text-secondary)",
        fontSize: 12,
        cursor: disabled ? "not-allowed" : "pointer",
      }}
    >
      {children}
    </button>
  );
}

// ConfigErrorState surfaces backend "service unavailable" (503) responses —
// the expected case is a missing TMDB key — as an actionable empty state.
function ConfigErrorState({ err }: { err: APIError }) {
  const isTMDB = (err.detail ?? err.message ?? "").toLowerCase().includes("tmdb");
  return (
    <div
      style={{
        margin: "40px auto",
        maxWidth: 520,
        padding: "24px 28px",
        borderRadius: 10,
        border: "1px solid var(--color-warning)",
        background: "color-mix(in srgb, var(--color-warning) 8%, transparent)",
      }}
    >
      <div style={{ display: "flex", alignItems: "flex-start", gap: 14 }}>
        <AlertTriangle size={22} style={{ color: "var(--color-warning)", flexShrink: 0, marginTop: 2 }} />
        <div style={{ flex: 1 }}>
          <div style={{ fontSize: 14, fontWeight: 600, color: "var(--color-text-primary)", marginBottom: 4 }}>
            {isTMDB ? "TMDB metadata provider not configured" : "Discover is unavailable"}
          </div>
          <div style={{ fontSize: 13, color: "var(--color-text-secondary)", lineHeight: 1.5 }}>
            {err.detail ?? err.message ?? "Pilot couldn't reach its metadata provider."}
          </div>
          {isTMDB && (
            <div style={{ marginTop: 12, fontSize: 13, color: "var(--color-text-secondary)" }}>
              Add a TMDB key in{" "}
              <Link to="/settings/providers" style={{ color: "var(--color-accent)", textDecoration: "none", fontWeight: 500 }}>
                Settings → Providers
              </Link>
              . Free keys at{" "}
              <a href="https://www.themoviedb.org/settings/api" target="_blank" rel="noopener noreferrer" style={{ color: "var(--color-accent)", textDecoration: "none" }}>
                themoviedb.org/settings/api
              </a>
              .
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
