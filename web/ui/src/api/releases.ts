import { useQuery, useMutation } from "@tanstack/react-query";
import { apiFetch } from "./client";
import type { ReleaseResult } from "@/types";

interface SearchReleasesParams {
  seriesId: string;
  season?: number;
  episode?: number;
}

export function useSearchReleases({ seriesId, season, episode }: SearchReleasesParams, enabled: boolean) {
  const params = new URLSearchParams();
  if (season !== undefined) params.set("season", String(season));
  if (episode !== undefined) params.set("episode", String(episode));
  const qs = params.toString();

  return useQuery({
    queryKey: ["releases", seriesId, season, episode],
    queryFn: () =>
      apiFetch<ReleaseResult[]>(`/series/${seriesId}/releases${qs ? `?${qs}` : ""}`),
    enabled: enabled && !!seriesId,
    staleTime: 0, // always re-fetch when modal opens
  });
}

export function useGrabRelease(seriesId: string) {
  return useMutation({
    mutationFn: (guid: string) =>
      apiFetch<void>(`/series/${seriesId}/releases/grab`, {
        method: "POST",
        body: JSON.stringify({ guid }),
      }),
  });
}
