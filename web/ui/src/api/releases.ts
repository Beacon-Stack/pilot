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
    staleTime: 0,
    gcTime: 0,       // don't cache stale/error results across modal opens
    retry: 1,        // retry once then show error
  });
}

interface GrabReleaseInput {
  guid: string;
  title: string;
  indexer_id: string;
  protocol: string;
  download_url: string;
  size: number;
  quality: { resolution: string; source: string; codec: string; hdr: string; name: string };
}

export function useGrabRelease(seriesId: string) {
  return useMutation({
    mutationFn: (release: GrabReleaseInput) =>
      apiFetch<void>(`/series/${seriesId}/releases/grab`, {
        method: "POST",
        body: JSON.stringify(release),
      }),
  });
}
