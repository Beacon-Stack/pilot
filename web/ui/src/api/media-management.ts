import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiFetch } from "./client";
import type { MediaManagement } from "@/types";

export function useMediaManagement() {
  return useQuery({
    queryKey: ["media-management"],
    queryFn: () => apiFetch<MediaManagement>("/media-management"),
  });
}

export function useEmptyRecycleBin() {
  return useMutation({
    mutationFn: () => apiFetch<{ deleted: number }>("/recycle-bin/empty", { method: "POST" }),
    onSuccess: (r) => toast.success(r.deleted === 1 ? "Deleted 1 file from the recycle bin" : `Deleted ${r.deleted} files from the recycle bin`),
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useUpdateMediaManagement() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: MediaManagement) =>
      apiFetch<MediaManagement>("/media-management", {
        method: "PUT",
        body: JSON.stringify(body),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["media-management"] });
      toast.success("Media management settings saved");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}
