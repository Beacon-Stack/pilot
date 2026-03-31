import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "./client";
import type { Series, Season, Episode, SeriesListResponse } from "@/types";

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

export function useLookupSeries(query: string) {
  return useQuery({
    queryKey: ["series", "lookup", query],
    queryFn: () => apiFetch<Series[]>(`/series/lookup?q=${encodeURIComponent(query)}`),
    enabled: query.length >= 2,
  });
}

// ── Mutations ────────────────────────────────────────────────────────────────

export function useAddSeries() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Partial<Series>) =>
      apiFetch<Series>("/series", { method: "POST", body: JSON.stringify(body) }),
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ["series"] }); },
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
      apiFetch<Season>(`/series/${seriesId}/seasons/${seasonId}`, {
        method: "PUT",
        body: JSON.stringify({ monitored }),
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["series", seriesId, "seasons"] });
    },
  });
}

// ── Episodes ─────────────────────────────────────────────────────────────────

export function useEpisodes(seriesId: string, seasonNumber: number) {
  return useQuery({
    queryKey: ["series", seriesId, "episodes", seasonNumber],
    queryFn: () =>
      apiFetch<Episode[]>(`/series/${seriesId}/episodes?season=${seasonNumber}`),
    enabled: !!seriesId,
  });
}

export function useUpdateEpisodeMonitored(seriesId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ episodeId, monitored, seasonNumber }: { episodeId: string; monitored: boolean; seasonNumber: number }) =>
      apiFetch<Episode>(`/series/${seriesId}/episodes/${episodeId}`, {
        method: "PUT",
        body: JSON.stringify({ monitored }),
      }).then((ep) => ({ ep, seasonNumber })),
    onSuccess: ({ seasonNumber }) => {
      void qc.invalidateQueries({ queryKey: ["series", seriesId, "episodes", seasonNumber] });
    },
  });
}
