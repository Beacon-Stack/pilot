import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiFetch } from "./client";
import type { EpisodeFile, RenamePreviewItem } from "@/types";

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

export function useRenamePreview(seriesId: string) {
  return useQuery({
    queryKey: ["series", seriesId, "rename-preview"],
    queryFn: () =>
      apiFetch<RenamePreviewItem[]>(`/series/${seriesId}/rename?dry_run=true`, {
        method: "POST",
      }),
    enabled: false, // only triggered manually
  });
}

export function useRenameExecute(seriesId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () =>
      apiFetch<RenamePreviewItem[]>(`/series/${seriesId}/rename?dry_run=false`, {
        method: "POST",
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["series", seriesId, "files"] });
      toast.success("Rename complete");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

// ── Library scan task ────────────────────────────────────────────────────────

export function useLibraryScan() {
  return useMutation({
    mutationFn: () =>
      apiFetch<void>("/tasks/library_scan/run", { method: "POST" }),
    onSuccess: () => toast.success("Library scan started"),
    onError: (err) => toast.error((err as Error).message),
  });
}
