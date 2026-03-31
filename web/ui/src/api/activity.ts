import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "./client";

export interface ActivityEntry {
  id: string;
  type: string;
  category: string;
  series_id?: string;
  title: string;
  detail: string;
  created_at: string;
}

export interface ActivityResponse {
  activities: ActivityEntry[];
  total: number;
  page: number;
  per_page: number;
}

export function useActivity(page: number, perPage: number) {
  return useQuery({
    queryKey: ["activity", page, perPage],
    queryFn: () =>
      apiFetch<ActivityResponse>(
        `/activity?page=${page}&per_page=${perPage}`
      ),
    refetchInterval: 15_000,
  });
}
