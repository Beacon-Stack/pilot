import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch, APIError } from "./client";
import type { ReleaseSearchResponse } from "@/types";

interface SearchReleasesParams {
  seriesId: string;
  season?: number;
  episode?: number;
}

// Interactive search streams: the server sends one JSON line each time an
// indexer answers, holding everything found so far, and a last line with
// done=true. Partial lines are put in the query cache as they arrive, so
// fast indexers' results show while slow ones are still working. While a
// search is streaming, `data.done` is false and `isFetching` is true.
export function useSearchReleases({ seriesId, season, episode }: SearchReleasesParams, enabled: boolean) {
  const queryClient = useQueryClient();
  const params = new URLSearchParams();
  if (season !== undefined) params.set("season", String(season));
  if (episode !== undefined) params.set("episode", String(episode));
  const qs = params.toString();
  const queryKey = ["releases", seriesId, season, episode];

  return useQuery({
    queryKey,
    queryFn: ({ signal }) =>
      streamReleases(`/series/${seriesId}/releases/stream${qs ? `?${qs}` : ""}`, signal, (partial) => {
        // A re-search keeps the finished results on screen until the new
        // search finishes, rather than blanking them for its first lines.
        const shown = queryClient.getQueryData<ReleaseSearchResponse>(queryKey);
        if (shown?.done) return;
        queryClient.setQueryData(queryKey, partial);
      }),
    enabled: enabled && !!seriesId,
    staleTime: 0,
    gcTime: 0, // don't cache stale/error results across modal opens
    // No automatic retry: a search that failed already waited out every
    // indexer, and retrying doubled the wait before the error showed.
    // Per-indexer failures come back inside a successful response.
    retry: 0,
  });
}

async function streamReleases(
  path: string,
  signal: AbortSignal,
  onPartial: (partial: ReleaseSearchResponse) => void,
): Promise<ReleaseSearchResponse> {
  const res = await fetch(`/api/v1${path}`, { signal });
  if (!res.ok) {
    let message = res.statusText || "Search failed";
    try {
      const body = (await res.json()) as { title?: string; detail?: string; errors?: { message?: string }[] };
      // huma puts the underlying error in errors[0].message, not detail
      message = body.errors?.[0]?.message ?? body.detail ?? body.title ?? message;
    } catch {
      // not JSON; keep the status text
    }
    throw new APIError(res.status, message);
  }
  if (!res.body) throw new APIError(res.status, "Search returned no data");

  const reader = res.body.pipeThrough(new TextDecoderStream()).getReader();
  let buffered = "";
  let last: ReleaseSearchResponse | undefined;
  const take = (line: string) => {
    if (!line.trim()) return;
    last = JSON.parse(line) as ReleaseSearchResponse;
    if (!last.done) onPartial(last);
  };
  for (;;) {
    const { value, done } = await reader.read();
    if (done) break;
    buffered += value;
    let newline: number;
    while ((newline = buffered.indexOf("\n")) >= 0) {
      take(buffered.slice(0, newline));
      buffered = buffered.slice(newline + 1);
    }
  }
  take(buffered);
  if (!last) throw new APIError(502, "Search ended without results");
  // A stream cut short (server restart) ends on a partial line; show it
  // as finished rather than spinning forever.
  return { ...last, done: true };
}

interface GrabReleaseInput {
  guid: string;
  title: string;
  indexer_id: string;
  protocol: string;
  download_url: string;
  size: number;
  quality: { resolution: string; source: string; codec: string; hdr: string; name: string };
  season_number?: number;
  episode_id?: string;
  // When true, removes the release from the blocklist before grabbing.
  // Sent by the "override and grab anyway" button on grayed (filtered) rows.
  override?: boolean;
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

interface AutoSearchInput {
  season: number;
  episode?: number;
  episode_id?: string;
}

interface AutoSearchResult {
  result: "grabbed" | "no_match";
  release_title?: string;
  reason?: string;
}

export function useAutoSearch(seriesId: string) {
  return useMutation({
    mutationFn: (input: AutoSearchInput) =>
      apiFetch<AutoSearchResult>(`/series/${seriesId}/auto-search`, {
        method: "POST",
        body: JSON.stringify(input),
      }),
  });
}
