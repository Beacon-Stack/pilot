import { useMutation, useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiFetch } from "./client";
import type { WantedResponse, WantedSearchResult } from "@/types";

export function useMissingEpisodes(page: number, perPage: number) {
  return useQuery({
    queryKey: ["wanted", "missing", page, perPage],
    queryFn: () =>
      apiFetch<WantedResponse>(
        `/wanted/missing?page=${page}&per_page=${perPage}`
      ),
  });
}

export function useCutoffUnmet(page: number, perPage: number) {
  return useQuery({
    queryKey: ["wanted", "cutoff", page, perPage],
    queryFn: () =>
      apiFetch<WantedResponse>(
        `/wanted/cutoff?page=${page}&per_page=${perPage}`
      ),
  });
}

/** Starts a background search of every missing or cutoff-unmet episode. */
export function useSearchAllWanted() {
  return useMutation({
    mutationFn: (kind: "missing" | "cutoff") =>
      apiFetch<WantedSearchResult>(`/wanted/${kind}/search`, { method: "POST" }),
    onSuccess: (res) => toast.success(res.message),
    onError: (err: Error) => toast.error(err.message),
  });
}
