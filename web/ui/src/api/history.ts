import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "./client";

export interface GrabHistoryItem {
  id: string;
  series_id: string;
  episode_id?: string;
  season_number?: number;
  release_title: string;
  release_source: string;
  release_resolution: string;
  protocol: string;
  size: number;
  download_status: string;
  grabbed_at: string;
  indexer_id?: string;
}

export interface HistoryResponse {
  items: GrabHistoryItem[];
  total: number;
  page: number;
  per_page: number;
}

export function useHistory(page: number, perPage: number) {
  return useQuery({
    queryKey: ["history", page, perPage],
    queryFn: () =>
      apiFetch<HistoryResponse>(`/history?page=${page}&per_page=${perPage}`),
    staleTime: 30_000,
  });
}
