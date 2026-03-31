import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "./client";
import type { FsBrowseResult } from "@/types";

export function useFsBrowse(path: string) {
  return useQuery({
    queryKey: ["fs", "browse", path],
    queryFn: () =>
      apiFetch<FsBrowseResult>(`/fs/browse?path=${encodeURIComponent(path)}`),
    enabled: !!path,
  });
}
