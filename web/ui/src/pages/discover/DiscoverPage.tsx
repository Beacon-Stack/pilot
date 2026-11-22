import { useMemo } from "react";
import { useLibraryTmdbIds } from "@/api/series";
import { useDiscoverBrowse, useDiscoverProviders, useDiscoverSearch, useGenreList } from "@/api/discover";
import PageHeader from "@/components/PageHeader";
import { APIError } from "@beacon-shared/api";
import {
  DiscoverControls,
  DiscoverEmpty,
  DiscoverGrid,
  DiscoverLoading,
  DiscoverPager,
  DiscoverUnavailable,
} from "@beacon-shared/Discover";
import { discoverRegion, useDiscoverState } from "@beacon-shared/discoverState";
import DiscoverCard from "./DiscoverCard";

const region = discoverRegion();

export default function DiscoverPage() {
  const s = useDiscoverState();
  const browse = useDiscoverBrowse({ category: s.category, page: s.page, region, ...s.filters }, !s.searching);
  const search = useDiscoverSearch(s.query, s.page, s.searching);
  const { data: genres } = useGenreList();
  const { data: providers } = useDiscoverProviders(region);
  const { data: tmdbIds } = useLibraryTmdbIds();
  const added = useMemo(() => new Set(tmdbIds ?? []), [tmdbIds]);

  const active = s.searching ? search : browse;
  const data = active.data;
  const unavailable = active.error instanceof APIError && active.error.status === 503 ? active.error : null;

  return (
    <div style={{ padding: 24, maxWidth: 1200 }}>
      <PageHeader title="Discover" description="Find TV series to add to your library." />

      <DiscoverControls
        noun="TV series"
        query={s.query}
        onQuery={s.setQuery}
        busy={s.searching && search.isFetching}
        category={s.category}
        onCategory={s.setCategory}
        filters={s.filters}
        filterCount={s.filterCount}
        onFilter={s.setFilter}
        onClearFilters={s.clearFilters}
        genres={genres}
        providers={providers}
        region={region}
      />

      {unavailable ? (
        <DiscoverUnavailable app="Pilot" message={unavailable.detail} />
      ) : active.isLoading ? (
        <DiscoverLoading />
      ) : active.isError ? (
        <DiscoverEmpty title="Couldn't load this list" hint="TMDB didn't answer. Try again in a moment." />
      ) : data && data.results.length === 0 ? (
        s.searching ? (
          <DiscoverEmpty title="No series found" hint="Try a different title." />
        ) : (
          <DiscoverEmpty
            title="Nothing matches these filters"
            hint={s.filterCount ? "Try fewer filters or a different list." : undefined}
            action={
              s.filterCount ? (
                <button type="button" onClick={s.clearFilters} style={{ background: "none", border: "none", color: "var(--color-accent)", fontSize: 13, cursor: "pointer" }}>
                  Clear filters
                </button>
              ) : undefined
            }
          />
        )
      ) : data ? (
        <div style={{ opacity: active.isPlaceholderData ? 0.6 : 1, transition: "opacity 120ms" }}>
          <DiscoverGrid>
            {data.results.map((r) => (
              <DiscoverCard key={r.tmdb_id} result={r} isAdded={added.has(r.tmdb_id)} />
            ))}
          </DiscoverGrid>
          <DiscoverPager page={data.page} totalPages={data.total_pages} onPage={s.setPage} />
        </div>
      ) : null}
    </div>
  );
}
