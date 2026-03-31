import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "./client";
import type { WantedResponse } from "@/types";

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
