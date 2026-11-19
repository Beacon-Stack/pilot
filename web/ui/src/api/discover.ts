import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "./client";

export interface DiscoverCast {
  id: number;
  name: string;
  character: string;
  profile_path: string;
}

export interface DiscoverCrew {
  id: number;
  name: string;
  job: string;
  profile_path: string;
}

export interface DiscoverRecommendation {
  tmdb_id: number;
  title: string;
  year: number;
  poster_path: string;
}

export interface DiscoverSeason {
  season_number: number;
  episode_count: number;
  air_date: string;
}

export interface DiscoverSeriesDetail {
  tmdb_id: number;
  title: string;
  original_title: string;
  year: number;
  overview: string;
  first_air_date: string;
  rating: number;
  status: string;
  network: string;
  genres: string[];
  poster_path: string;
  backdrop_path: string;
  seasons: DiscoverSeason[];
  cast: DiscoverCast[];
  crew: DiscoverCrew[];
  recommendations: DiscoverRecommendation[];
  in_library: boolean;
  library_series_id?: string;
}

export function useDiscoverSeries(tmdbId: number) {
  return useQuery({
    queryKey: ["discover", "series", tmdbId],
    queryFn: () => apiFetch<DiscoverSeriesDetail>(`/discover/${tmdbId}`),
    enabled: tmdbId > 0,
    staleTime: 5 * 60_000,
  });
}

// ── Browse (auto-populated categories) ───────────────────────────────────────

export interface DiscoverListItem {
  tmdb_id: number;
  title: string;
  year: number;
  overview: string;
  poster_path: string;
  rating: number;
}

export interface DiscoverListResponse {
  results: DiscoverListItem[];
  page: number;
  total_pages: number;
}

export interface TMDBGenre {
  id: number;
  name: string;
}

export type DiscoverCategory = "trending" | "popular" | "top-rated" | "on-the-air";

export function useDiscover(category: DiscoverCategory, page: number) {
  return useQuery({
    queryKey: ["discover", category, page],
    queryFn: () => apiFetch<DiscoverListResponse>(`/discover/${category}?page=${page}`),
    enabled: page > 0,
    staleTime: 5 * 60_000,
  });
}

export function useDiscoverByGenre(genreId: number, page: number) {
  return useQuery({
    queryKey: ["discover", "genre", genreId, page],
    queryFn: () => apiFetch<DiscoverListResponse>(`/discover/genre/${genreId}?page=${page}`),
    enabled: genreId > 0 && page > 0,
    staleTime: 5 * 60_000,
  });
}

export function useGenreList() {
  return useQuery({
    queryKey: ["discover", "genres"],
    queryFn: () => apiFetch<TMDBGenre[]>("/discover/genres"),
    staleTime: Infinity,
  });
}
