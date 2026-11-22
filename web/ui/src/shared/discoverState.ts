import { useSearchParams } from "react-router-dom";

/** The Discover lists, the same in Pilot and Prism. */
export const DISCOVER_CATEGORIES = [
  { value: "trending", label: "Trending" },
  { value: "popular", label: "Popular" },
  { value: "top-rated", label: "Top rated" },
  { value: "new", label: "New on streaming" },
  { value: "coming-soon", label: "Coming soon" },
] as const;

export type DiscoverCategory = (typeof DISCOVER_CATEGORIES)[number]["value"];

export interface DiscoverFilters {
  genre: number;
  provider: number;
  minRating: number;
}

/** Searches start at this many characters. */
export const MIN_SEARCH = 2;

/**
 * The viewer's country from the browser locale ("en-GB" → "GB"), for
 * streaming services and release dates. US when the locale has none.
 */
export function discoverRegion(): string {
  try {
    const langs = typeof navigator !== "undefined" ? [...(navigator.languages ?? []), navigator.language] : [];
    for (const l of langs) {
      if (!l) continue;
      const region = new Intl.Locale(l).maximize().region;
      if (region && /^[A-Z]{2}$/.test(region)) return region;
    }
  } catch {
    // Old browser or odd locale string: fall through.
  }
  return "US";
}

function num(v: string | null): number {
  const n = Number(v);
  return Number.isFinite(n) && n > 0 ? n : 0;
}

/**
 * Discover's search, list, filters and page, kept in the URL so the back
 * button from a title's page returns to the same list.
 */
export function useDiscoverState() {
  const [params, setParams] = useSearchParams();

  const raw = params.get("category");
  const category: DiscoverCategory = DISCOVER_CATEGORIES.some((c) => c.value === raw) ? (raw as DiscoverCategory) : "trending";
  const query = params.get("q") ?? "";
  const page = Math.max(1, Math.floor(num(params.get("page"))) || 1);
  const filters: DiscoverFilters = {
    genre: num(params.get("genre")),
    provider: num(params.get("provider")),
    minRating: num(params.get("min_rating")),
  };
  const filterCount = [filters.genre, filters.provider, filters.minRating].filter(Boolean).length;

  function update(patch: Record<string, string | number | null>, keepPage = false) {
    const next = new URLSearchParams(params);
    for (const [k, v] of Object.entries(patch)) {
      if (v === null || v === "" || v === 0) next.delete(k);
      else next.set(k, String(v));
    }
    if (!keepPage) next.delete("page");
    setParams(next, { replace: true });
  }

  return {
    query,
    category,
    page,
    filters,
    filterCount,
    searching: query.trim().length >= MIN_SEARCH,
    setQuery: (q: string) => update({ q: q.trim() ? q : null }),
    // Trending is a fixed list that can't be filtered, so choosing it
    // clears the filters.
    setCategory: (c: DiscoverCategory) =>
      update(c === "trending" ? { category: null, genre: null, provider: null, min_rating: null } : { category: c }),
    // ...and choosing a filter while on Trending moves to Popular.
    setFilter: (key: keyof DiscoverFilters, value: number) =>
      update({
        [key === "minRating" ? "min_rating" : key]: value,
        ...(category === "trending" && value ? { category: "popular" } : {}),
      }),
    clearFilters: () => update({ genre: null, provider: null, min_rating: null }),
    setPage: (p: number) => update({ page: p > 1 ? p : null }, true),
  };
}
