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
      toast.success("Re-import queued");
      void qc.invalidateQueries({ queryKey: ["series", seriesId] });
      void qc.invalidateQueries({ queryKey: ["series", seriesId, "haul-history"] });
    },
    onError: (err) => toast.error((err as Error).message),
  });
}
