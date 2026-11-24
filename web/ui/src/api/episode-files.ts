import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiFetch } from "./client";
import type { EpisodeFile } from "@/types";

// ── Episode files ─────────────────────────────────────────────────────────────

export function useEpisodeFiles(seriesId: string) {
  return useQuery({
    queryKey: ["series", seriesId, "files"],
    queryFn: () => apiFetch<EpisodeFile[]>(`/series/${seriesId}/files`),
    enabled: !!seriesId,
  });
}

export function useDeleteEpisodeFile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ fileId, deleteFromDisk }: { fileId: string; deleteFromDisk: boolean }) =>
      apiFetch<void>(`/episodefiles/${fileId}?delete_from_disk=${deleteFromDisk}`, {
        method: "DELETE",
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["series"] });
      toast.success("Episode file deleted");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

// ── Rename preview / execute ─────────────────────────────────────────────────

// ── Library scan task ────────────────────────────────────────────────────────

export function useLibraryScan() {
  return useMutation({
    mutationFn: () =>
      apiFetch<void>("/tasks/library_scan/run", { method: "POST" }),
    onSuccess: () => toast.success("Library scan started"),
    onError: (err) => toast.error((err as Error).message),
  });
}
