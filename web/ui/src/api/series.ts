import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "./client";
import type { Series, Season, Cour, Episode, SeriesListResponse } from "@/types";

// ── Series list & detail ─────────────────────────────────────────────────────

export function useSeriesList() {
  return useQuery({
    queryKey: ["series"],
    queryFn: () => apiFetch<SeriesListResponse>("/series"),
  });
}

export function useSeriesDetail(id: string) {
  return useQuery({
    queryKey: ["series", id],
    queryFn: () => apiFetch<Series>(`/series/${id}`),
    enabled: !!id,
  });
}

// ── Lookup (search TMDB before adding) ──────────────────────────────────────

export interface LookupResult {
  tmdb_id: number;
  title: string;
  original_title: string;
  overview: string;
  first_air_date: string;
  year: number;
  poster_path: string;
  backdrop_path: string;
  popularity: number;
}

export function tmdbPosterURL(path: string, size = "w500"): string {
  if (!path) return "";
  if (path.startsWith("http")) return path;
  return `https://image.tmdb.org/t/p/${size}${path}`;
}

export function useLookupSeries(query: string) {
  return useQuery({
    queryKey: ["series", "lookup", query],
    queryFn: () => apiFetch<LookupResult[]>("/series/lookup", {
      method: "POST",
      body: JSON.stringify({ query }),
    }),
    enabled: query.length >= 3,
  });
}

// ── Library TMDB IDs (for "already added" detection) ────────────────────────

export function useLibraryTmdbIds() {
  return useQuery({
    queryKey: ["series", "tmdb-ids"],
    queryFn: () => apiFetch<number[]>("/series/tmdb-ids"),
  });
}

// ── Mutations ────────────────────────────────────────────────────────────────

interface AddSeriesInput {
  tmdb_id: number;
  library_id: string;
  quality_profile_id: string;
  monitored: boolean;
  monitor_type: string;
  series_type: string;
}

export function useAddSeries() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: AddSeriesInput) =>
      apiFetch<Series>("/series", { method: "POST", body: JSON.stringify(body) }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["series"] });
      void qc.invalidateQueries({ queryKey: ["series", "tmdb-ids"] });
    },
  });
}

export function useUpdateSeries(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Partial<Series>) =>
      apiFetch<Series>(`/series/${id}`, { method: "PUT", body: JSON.stringify(body) }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["series"] });
      void qc.invalidateQueries({ queryKey: ["series", id] });
    },
  });
}

export function useDeleteSeries() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      apiFetch<void>(`/series/${id}`, { method: "DELETE" }),
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ["series"] }); },
  });
}

export function useRefreshSeriesMetadata(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () =>
      apiFetch<Series>(`/series/${id}/refresh`, { method: "POST" }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["series", id] });
    },
  });
}

// ── Seasons ──────────────────────────────────────────────────────────────────

export function useSeasons(seriesId: string) {
  return useQuery({
    queryKey: ["series", seriesId, "seasons"],
    queryFn: () => apiFetch<Season[]>(`/series/${seriesId}/seasons`),
    enabled: !!seriesId,
  });
}

export function useUpdateSeasonMonitored(seriesId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ seasonId, monitored }: { seasonId: string; monitored: boolean }) =>
      apiFetch<Season>(`/seasons/${seasonId}`, {
        method: "PUT",
        body: JSON.stringify({ monitored }),
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["series", seriesId, "seasons"] });
    },
  });
}

// ── Cours (anime) ────────────────────────────────────────────────────────────

// useCours returns the cour-shaped projection for an anime series.
// The backend returns an empty array for non-anime or unmapped series;
// the SeriesDetail component checks length to decide whether to fall
// back to useSeasons.
export function useCours(seriesId: string, enabled: boolean) {
  return useQuery({
    queryKey: ["series", seriesId, "cours"],
    queryFn: () => apiFetch<Cour[]>(`/series/${seriesId}/cours`),
    enabled: !!seriesId && enabled,
  });
}

export function useUpdateCourMonitored(seriesId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ tvdbSeason, monitored }: { tvdbSeason: number; monitored: boolean }) =>
      apiFetch<Cour>(`/series/${seriesId}/cours/${tvdbSeason}/monitored`, {
        method: "PUT",
        body: JSON.stringify({ monitored }),
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["series", seriesId, "cours"] });
    },
  });
}

// ── Episodes ─────────────────────────────────────────────────────────────────

export function useEpisodes(seriesId: string, seasonNumber: number) {
  return useQuery({
    queryKey: ["series", seriesId, "episodes", seasonNumber],
    queryFn: () =>
      apiFetch<Episode[]>(`/series/${seriesId}/seasons/${seasonNumber}/episodes`),
    enabled: !!seriesId,
  });
}

export function useUpdateEpisodeMonitored(seriesId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ episodeId, monitored, seasonNumber }: { episodeId: string; monitored: boolean; seasonNumber: number }) =>
      apiFetch<Episode>(`/episodes/${episodeId}`, {
        method: "PUT",
        body: JSON.stringify({ monitored }),
      }).then((ep) => ({ ep, seasonNumber })),
    onSuccess: ({ seasonNumber }) => {
      void qc.invalidateQueries({ queryKey: ["series", seriesId, "episodes", seasonNumber] });
    },
  });
}
