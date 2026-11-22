import { useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { AlertTriangle, Loader2, Search, X } from "lucide-react";
import { DISCOVER_CATEGORIES, MIN_SEARCH, type DiscoverCategory, type DiscoverFilters } from "./discoverState";

// Shared pieces of the Discover page in Pilot and Prism: the search box,
// list buttons and filters, plus the grid, loading, empty and paging
// states. Each app supplies its own result card.

interface Option {
  id: number;
  name: string;
}

interface DiscoverControlsProps {
  /** "movies" or "TV series". */
  noun: string;
  query: string;
  onQuery: (q: string) => void;
  /** A search is running. */
  busy?: boolean;
  category: DiscoverCategory;
  onCategory: (c: DiscoverCategory) => void;
  filters: DiscoverFilters;
  filterCount: number;
  onFilter: (key: keyof DiscoverFilters, value: number) => void;
  onClearFilters: () => void;
  genres?: Option[];
  providers?: Option[];
  region: string;
}

const RATINGS = [6, 7, 8];

const pill = (active: boolean) => ({
  padding: "5px 12px",
  borderRadius: 6,
  border: active ? "1px solid var(--color-accent)" : "1px solid var(--color-border-default)",
  background: active ? "color-mix(in srgb, var(--color-accent) 12%, transparent)" : "transparent",
  color: active ? "var(--color-accent)" : "var(--color-text-secondary)",
  fontSize: 12,
  fontWeight: 500,
  cursor: "pointer",
});

const select = (active: boolean) => ({
  padding: "5px 10px",
  borderRadius: 6,
  border: active ? "1px solid var(--color-accent)" : "1px solid var(--color-border-default)",
  background: "var(--color-bg-elevated)",
  color: "var(--color-text-primary)",
  fontSize: 12,
  maxWidth: 220,
});

export function DiscoverControls(props: DiscoverControlsProps) {
  const { noun, query, onQuery, busy, category, onCategory, filters, filterCount, onFilter, onClearFilters, genres, providers, region } = props;
  const [text, setText] = useState(query);
  const sent = useRef(query);
  // The parent's callback changes every render; keep the latest without
  // restarting the typing timer each time results load.
  const onQueryRef = useRef(onQuery);
  useEffect(() => {
    onQueryRef.current = onQuery;
  }, [onQuery]);

  // Search as you type, a moment after typing stops.
  useEffect(() => {
    if (text === sent.current) return;
    const t = setTimeout(() => {
      sent.current = text;
      onQueryRef.current(text);
    }, 300);
    return () => clearTimeout(t);
  }, [text]);

  function clearSearch() {
    setText("");
    sent.current = "";
    onQuery("");
  }

  const searching = query.trim().length >= MIN_SEARCH;

  return (
    <div style={{ marginBottom: 20 }}>
      <div style={{ position: "relative", maxWidth: 560, marginBottom: 16 }}>
        <Search
          size={16}
          aria-hidden
          style={{ position: "absolute", left: 14, top: "50%", transform: "translateY(-50%)", color: "var(--color-text-muted)", pointerEvents: "none" }}
        />
        <input
          type="search"
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder={`Search ${noun} by title…`}
          aria-label={`Search ${noun}`}
          style={{
            width: "100%",
            padding: "11px 40px",
            borderRadius: 8,
            border: "1px solid var(--color-border-default)",
            background: "var(--color-bg-elevated)",
            color: "var(--color-text-primary)",
            fontSize: 14,
            outline: "none",
          }}
        />
        {busy ? (
          <Loader2
            size={16}
            aria-label="Searching"
            style={{ position: "absolute", right: 14, top: "50%", transform: "translateY(-50%)", color: "var(--color-accent)", animation: "spin 1s linear infinite" }}
          />
        ) : (
          text && (
            <button
              type="button"
              onClick={clearSearch}
              aria-label="Clear search"
              style={{ position: "absolute", right: 10, top: "50%", transform: "translateY(-50%)", background: "none", border: "none", padding: 4, cursor: "pointer", color: "var(--color-text-muted)", display: "flex" }}
            >
              <X size={16} />
            </button>
          )
        )}
      </div>

      {!searching && (
        <>
          <div role="group" aria-label="List" style={{ display: "flex", gap: 6, flexWrap: "wrap", marginBottom: 10 }}>
            {DISCOVER_CATEGORIES.map((c) => (
              <button key={c.value} type="button" aria-pressed={category === c.value} onClick={() => onCategory(c.value)} style={pill(category === c.value)}>
                {c.label}
              </button>
            ))}
          </div>

          <div role="group" aria-label="Filters" style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
            <select aria-label="Genre" value={filters.genre} onChange={(e) => onFilter("genre", Number(e.target.value))} style={select(filters.genre > 0)}>
              <option value={0}>Any genre</option>
              {genres?.map((g) => (
                <option key={g.id} value={g.id}>{g.name}</option>
              ))}
            </select>
            <select aria-label="Streaming service" value={filters.provider} onChange={(e) => onFilter("provider", Number(e.target.value))} style={select(filters.provider > 0)}>
              <option value={0}>Any service ({region})</option>
              {providers?.map((p) => (
                <option key={p.id} value={p.id}>{p.name}</option>
              ))}
            </select>
            <select aria-label="Minimum rating" value={filters.minRating} onChange={(e) => onFilter("minRating", Number(e.target.value))} style={select(filters.minRating > 0)}>
              <option value={0}>Any rating</option>
              {RATINGS.map((r) => (
                <option key={r} value={r}>{r}+ rating</option>
              ))}
            </select>
            {filterCount > 0 && (
              <button type="button" onClick={onClearFilters} style={{ background: "none", border: "none", padding: "4px 6px", fontSize: 12, color: "var(--color-accent)", cursor: "pointer" }}>
                Clear filters
              </button>
            )}
          </div>
        </>
      )}
    </div>
  );
}

export function DiscoverGrid({ children }: { children: ReactNode }) {
  return (
    <div data-testid="discover-grid" style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(160px, 1fr))", gap: 16 }}>
      {children}
    </div>
  );
}

export function DiscoverLoading() {
  return (
    <DiscoverGrid>
      {Array.from({ length: 18 }).map((_, i) => (
        <div key={i}>
          <div className="skeleton" style={{ paddingBottom: "150%", borderRadius: 8 }} />
          <div className="skeleton" style={{ height: 14, width: "80%", marginTop: 8, borderRadius: 4 }} />
        </div>
      ))}
    </DiscoverGrid>
  );
}

export function DiscoverEmpty({ title, hint, action }: { title: string; hint?: string; action?: ReactNode }) {
  return (
    <div data-testid="empty-state" style={{ textAlign: "center", padding: "60px 20px" }}>
      <div style={{ color: "var(--color-text-secondary)", fontSize: 14, fontWeight: 500 }}>{title}</div>
      {hint && <div style={{ color: "var(--color-text-muted)", fontSize: 13, marginTop: 4 }}>{hint}</div>}
      {action && <div style={{ marginTop: 12 }}>{action}</div>}
    </div>
  );
}

/** TMDB serves at most 500 pages of any list. */
const MAX_PAGES = 500;

export function DiscoverPager({ page, totalPages, onPage }: { page: number; totalPages: number; onPage: (p: number) => void }) {
  const last = Math.min(totalPages, MAX_PAGES);
  if (last <= 1) return null;
  const btn = (disabled: boolean) => ({
    padding: "6px 14px",
    borderRadius: 6,
    border: "1px solid var(--color-border-default)",
    background: "transparent",
    color: disabled ? "var(--color-text-muted)" : "var(--color-text-secondary)",
    fontSize: 12,
    cursor: disabled ? "not-allowed" : "pointer",
  });
  function go(p: number) {
    onPage(p);
    window.scrollTo?.({ top: 0, behavior: "smooth" });
  }
  return (
    <nav aria-label="Pages" style={{ display: "flex", justifyContent: "center", gap: 12, marginTop: 32 }}>
      <button type="button" disabled={page <= 1} onClick={() => go(page - 1)} style={btn(page <= 1)}>
        Previous
      </button>
      <span style={{ fontSize: 12, color: "var(--color-text-muted)", alignSelf: "center" }}>
        Page {page} of {last}
      </span>
      <button type="button" disabled={page >= last} onClick={() => go(page + 1)} style={btn(page >= last)}>
        Next
      </button>
    </nav>
  );
}

/** Shown when the server says TMDB isn't set up (HTTP 503). */
export function DiscoverUnavailable({ app, message }: { app: string; message?: string }) {
  return (
    <div
      style={{
        margin: "40px auto",
        maxWidth: 520,
        padding: "24px 28px",
        borderRadius: 10,
        border: "1px solid var(--color-warning)",
        background: "color-mix(in srgb, var(--color-warning) 8%, transparent)",
        display: "flex",
        gap: 14,
      }}
    >
      <AlertTriangle size={22} style={{ color: "var(--color-warning)", flexShrink: 0, marginTop: 2 }} />
      <div style={{ fontSize: 13, color: "var(--color-text-secondary)", lineHeight: 1.5 }}>
        <div style={{ fontSize: 14, fontWeight: 600, color: "var(--color-text-primary)", marginBottom: 4 }}>TMDB isn't set up</div>
        {message ?? `${app} needs a TMDB key to browse and search.`} Add one in{" "}
        <Link to="/settings/providers" style={{ color: "var(--color-accent)", textDecoration: "none", fontWeight: 500 }}>
          Settings → Providers
        </Link>
        .
      </div>
    </div>
  );
}
