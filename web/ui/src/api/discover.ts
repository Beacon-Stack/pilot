import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { apiFetch } from "./client";

interface DiscoverCast {
  id: number;
  name: string;
  character: string;
  profile_path: string;
}

interface DiscoverCrew {
  id: number;
  name: string;
  job: string;
  profile_path: string;
}

interface DiscoverRecommendation {
  tmdb_id: number;
  title: string;
  year: number;
  poster_path: string;
}

interface DiscoverSeason {
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

interface DiscoverListItem {
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

export function useGenreList() {
  return useQuery({
    queryKey: ["discover", "genres"],
    queryFn: () => apiFetch<TMDBGenre[]>("/discover/genres"),
    staleTime: Infinity,
  });
}

// ── Browse with filters, search, streaming services ─────────────────────────

export interface DiscoverBrowseParams {
  category: string;
  page: number;
  genre: number;
  provider: number;
  minRating: number;
  region: string;
}

export interface StreamingService {
  id: number;
  name: string;
  logo_path: string;
}

export function useDiscoverBrowse(p: DiscoverBrowseParams, enabled = true) {
  const qs = new URLSearchParams({
    category: p.category,
    page: String(p.page),
    region: p.region,
    ...(p.genre ? { genre: String(p.genre) } : {}),
    ...(p.provider ? { provider: String(p.provider) } : {}),
    ...(p.minRating ? { min_rating: String(p.minRating) } : {}),
  });
  return useQuery({
    queryKey: ["discover", "browse", p],
    queryFn: () => apiFetch<DiscoverListResponse>(`/discover/browse?${qs}`),
    enabled,
    placeholderData: keepPreviousData,
    staleTime: 5 * 60_000,
  });
}

export function useDiscoverSearch(query: string, page: number, enabled = true) {
  const q = query.trim();
  return useQuery({
    queryKey: ["discover", "search", q, page],
    queryFn: () => apiFetch<DiscoverListResponse>(`/discover/search?q=${encodeURIComponent(q)}&page=${page}`),
    enabled: enabled && q.length > 0,
    placeholderData: keepPreviousData,
    staleTime: 5 * 60_000,
  });
}

export function useDiscoverProviders(region: string) {
  return useQuery({
    queryKey: ["discover", "providers", region],
    queryFn: () => apiFetch<StreamingService[]>(`/discover/providers?region=${region}`),
    staleTime: 24 * 60 * 60_000,
  });
}
