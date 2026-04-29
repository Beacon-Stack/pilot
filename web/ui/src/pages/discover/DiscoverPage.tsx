import { useState, useMemo } from "react";
import { Search, Loader2, AlertTriangle } from "lucide-react";
import { Link } from "react-router-dom";
import { useLookupSeries, useLibraryTmdbIds } from "@/api/series";
import type { LookupResult } from "@/api/series";
import { APIError } from "@beacon-shared/api";
import DiscoverCard from "./DiscoverCard";
import AddSeriesDrawer from "./AddSeriesDrawer";

export default function DiscoverPage() {
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState<LookupResult | null>(null);

  const { data: results, isLoading, isFetching, error } = useLookupSeries(query);
  const { data: tmdbIds } = useLibraryTmdbIds();

  // Surface "metadata provider not configured" as a clear actionable
  // empty state rather than a silent zero-results screen. Pilot returns
  // 503 with a specific detail string when the TMDB key is missing.
  // Any other 5xx is treated generically.
  const lookupConfigError =
    error instanceof APIError && error.status === 503 ? error : null;

  const addedSet = useMemo(
    () => new Set(tmdbIds ?? []),
    [tmdbIds]
  );

  const showLoading = query.length >= 3 && (isLoading || isFetching);

  return (
    <div style={{ padding: 24, maxWidth: 1200 }}>
      {/* Header */}
      <div style={{ marginBottom: 20 }}>
        <h1 style={{ margin: 0, fontSize: 20, fontWeight: 600, color: "var(--color-text-primary)" }}>
          Discover
        </h1>
        <p style={{ margin: "4px 0 0", fontSize: 13, color: "var(--color-text-secondary)" }}>
          Search for TV series to add to your library
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
        {showLoading && (
          <Loader2
            size={16}
            style={{
              position: "absolute", right: 14, top: "50%", transform: "translateY(-50%)",
              color: "var(--color-accent)", animation: "spin 1s linear infinite",
            }}
          />
        )}
      </div>

      {/* Results */}
      {query.length < 3 ? (
        <div style={{ textAlign: "center", padding: "60px 20px", color: "var(--color-text-muted)", fontSize: 13 }}>
          Type at least 3 characters to search
        </div>
      ) : lookupConfigError ? (
        <ConfigErrorState err={lookupConfigError} />
      ) : showLoading && !results?.length ? (
        <div style={{ textAlign: "center", padding: "60px 20px", color: "var(--color-text-muted)", fontSize: 13 }}>
          <Loader2 size={24} style={{ animation: "spin 1s linear infinite", marginBottom: 8 }} />
          <div>Searching...</div>
        </div>
      ) : results && results.length === 0 ? (
        <div style={{ textAlign: "center", padding: "60px 20px" }}>
          <div style={{ color: "var(--color-text-secondary)", fontSize: 14, fontWeight: 500 }}>No series found</div>
          <div style={{ color: "var(--color-text-muted)", fontSize: 13, marginTop: 4 }}>Try a different search term</div>
        </div>
      ) : results && results.length > 0 ? (
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(180px, 1fr))", gap: 16 }}>
          {results.map((r) => (
            <DiscoverCard
              key={r.tmdb_id}
              result={r}
              isAdded={addedSet.has(r.tmdb_id)}
              onClick={() => setSelected(r)}
            />
          ))}
        </div>
      ) : null}

      {/* Add drawer */}
      {selected && (
        <AddSeriesDrawer
          result={selected}
          isAdded={addedSet.has(selected.tmdb_id)}
          onClose={() => setSelected(null)}
        />
      )}
    </div>
  );
}

// ConfigErrorState surfaces backend "service unavailable" responses for
// the lookup endpoint as an actionable empty state. The expected case is
// a missing TMDB key, but we render the backend's `detail` as the body so
// future config-related 503s (e.g. provider quota exceeded) read the same.
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
          <div
            style={{
              fontSize: 14,
              fontWeight: 600,
              color: "var(--color-text-primary)",
              marginBottom: 4,
            }}
          >
            {isTMDB ? "TMDB metadata provider not configured" : "Series lookup is unavailable"}
          </div>
          <div style={{ fontSize: 13, color: "var(--color-text-secondary)", lineHeight: 1.5 }}>
            {err.detail ?? err.message ?? "Pilot couldn't reach its metadata provider."}
          </div>
          {isTMDB && (
            <div style={{ marginTop: 12, fontSize: 13, color: "var(--color-text-secondary)" }}>
              Add a TMDB key in{" "}
              <Link
                to="/settings/providers"
                style={{
                  color: "var(--color-accent)",
                  textDecoration: "none",
                  fontWeight: 500,
                }}
              >
                Settings → Providers
              </Link>
              . Free keys at{" "}
              <a
                href="https://www.themoviedb.org/settings/api"
                target="_blank"
                rel="noopener noreferrer"
                style={{ color: "var(--color-accent)", textDecoration: "none" }}
              >
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
