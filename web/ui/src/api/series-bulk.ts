import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "./client";

export interface SeriesBulkEdit {
  ids: string[];
  monitored?: boolean;
  quality_profile_id?: string;
  library_id?: string;
  series_type?: "standard" | "anime" | "daily";
}

export function useBulkEditSeries() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: SeriesBulkEdit) =>
      apiFetch<{ count: number }>("/series/bulk", { method: "PUT", body: JSON.stringify(body) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["series"] }),
  });
}

export function useBulkDeleteSeries() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { ids: string[]; delete_files: boolean }) =>
      apiFetch<{ count: number }>("/series/bulk/delete", { method: "POST", body: JSON.stringify(body) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["series"] }),
  });
}
