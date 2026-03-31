import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "./client";
import type { BlocklistEntry, BlocklistPage } from "@/types";

export function useBlocklist(page = 1, perPage = 50) {
  return useQuery({
    queryKey: ["blocklist", page, perPage],
    queryFn: () =>
      apiFetch<BlocklistPage>(`/blocklist?page=${page}&per_page=${perPage}`),
    staleTime: 30_000,
  });
}

export function useDeleteBlocklistEntry() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      apiFetch<void>(`/blocklist/${id}`, { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["blocklist"] }),
  });
}

export function useClearBlocklist() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () =>
      apiFetch<void>("/blocklist", { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["blocklist"] }),
  });
}

// Re-export types so callers can import from one place.
export type { BlocklistEntry };
