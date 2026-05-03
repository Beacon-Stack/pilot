import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiFetch } from "./client";

export interface HaulRecord {
  info_hash: string;
  name: string;
  save_path: string;
  category: string;
  added_at: string;
  completed_at: string; // empty string when not completed
  removed_at: string;   // empty string when still present
  requester: string;
  series_id: string;
  episode_id: string;
  tmdb_id: number;
  season: number;
  episode: number;
}

export function useSeriesHaulHistory(seriesId: string) {
  return useQuery({
    queryKey: ["series", seriesId, "haul-history"],
    queryFn: () =>
      apiFetch<{ records: HaulRecord[] }>(`/series/${seriesId}/haul-history`)
        .then((r) => r.records),
    enabled: !!seriesId,
    // Silent failure: if Haul isn't configured the endpoint returns []
    // — don't treat a network error as a blocker for the series page.
    throwOnError: false,
  });
}

export function useReimportFromHaul(seriesId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (infoHash: string) =>
      apiFetch<{ status: string }>("/import/from-haul", {
        method: "POST",
        body: JSON.stringify({ info_hash: infoHash }),
      }),
    onSuccess: () => {
      // Synchronous completion: the import has already moved + linked
      // the file by the time this fires. "Linked" beats "queued" —
      // nothing's actually in a queue.
      toast.success("Linked into library");
      void qc.invalidateQueries({ queryKey: ["series", seriesId] });
      void qc.invalidateQueries({ queryKey: ["series", seriesId, "haul-history"] });
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

// SeriesGrabHistory mirrors the backend grabHistoryBody type. The
// status drives whether the "orphaned grab" badge appears:
// download_status === "completed" + episode has no file in library.
export interface SeriesGrabHistoryItem {
  id: string;
  series_id: string;
  episode_id?: string;
  season_number?: number;
  indexer_id?: string;
  release_guid: string;
  release_title: string;
  protocol: string;
  size: number;
  download_status: string;
  grabbed_at: string;
  // Set when Pilot received the info_hash back from the download
  // client. Absent for grabs the client never confirmed (race during
  // a haul restart, etc) — those are invisible to the haul-stall path
  // and visible to the stallwatcher's age-sweep instead.
  info_hash?: string;
}

// useSeriesGrabHistory loads Pilot's own grab_history rows for the
// series. Used to detect "orphaned" grabs — completed downloads whose
// file never got linked into the library (the anime-importer bug class
// of issue). Cheap enough to fetch on every series detail open.
export function useSeriesGrabHistory(seriesId: string) {
  return useQuery({
    queryKey: ["series", seriesId, "grab-history"],
    queryFn: () =>
      apiFetch<SeriesGrabHistoryItem[]>(`/series/${seriesId}/grab-history`),
    enabled: !!seriesId,
    throwOnError: false,
  });
}

export interface GrabFileStatus {
  exists: boolean;
  reason?: "no_info_hash" | "no_haul_client" | "haul_unreachable" | "not_in_haul_history" | "file_missing_on_disk" | "";
  path?: string;
  size?: number;
}

// useGrabFileStatus checks whether a grab's downloaded file actually
// exists on disk. Used by the OrphanedGrabBadge to choose between
// rendering an Import button (file present) or an Investigate link
// (file missing or Haul lost track). Cached for 30s — re-checking on
// every render would hammer Haul's history endpoint.
export function useGrabFileStatus(grabId: string | undefined, enabled = true) {
  return useQuery({
    queryKey: ["grabs", grabId, "file-status"],
    queryFn: () => apiFetch<GrabFileStatus>(`/grabs/${grabId}/file-status`),
    enabled: !!grabId && enabled,
    staleTime: 30_000,
    throwOnError: false,
  });
}

// useReimportGrab triggers the reimport flow against an existing grab
// — looks up the info_hash + series_id from grab_history (server side),
// finds the file in Haul, runs the importer. Use this for orphaned
// grabs where Haul lacks the requester metadata but Pilot's
// grab_history has it.
export function useReimportGrab(seriesId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (grabId: string) =>
      apiFetch<{ status: string }>(`/grabs/${grabId}/reimport`, {
        method: "POST",
      }),
    onSuccess: () => {
      toast.success("Linked into library");
      void qc.invalidateQueries({ queryKey: ["series", seriesId] });
      void qc.invalidateQueries({ queryKey: ["series", seriesId, "haul-history"] });
      void qc.invalidateQueries({ queryKey: ["series", seriesId, "grab-history"] });
    },
    onError: (err) => toast.error((err as Error).message),
  });
}
