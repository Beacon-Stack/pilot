import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiFetch } from "./client";
import type {
  ImportListConfig,
  ImportListRequest,
  ImportExclusion,
  ImportListPreviewItem,
  ImportListSyncResult,
} from "@/types";

export function useImportLists() {
  return useQuery({
    queryKey: ["importlists"],
    queryFn: () => apiFetch<ImportListConfig[]>("/importlists"),
  });
}

export function useCreateImportList() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: ImportListRequest) =>
      apiFetch<ImportListConfig>("/importlists", {
        method: "POST",
        body: JSON.stringify(body),
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["importlists"] });
      toast.success("Import list added");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useUpdateImportList() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { id: string } & ImportListRequest) => {
      const { id, ...rest } = body;
      return apiFetch<ImportListConfig>(`/importlists/${id}`, {
        method: "PUT",
        body: JSON.stringify(rest),
      });
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["importlists"] });
      toast.success("Import list updated");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useDeleteImportList() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      apiFetch<void>(`/importlists/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["importlists"] });
      toast.success("Import list deleted");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useTestImportList() {
  return useMutation({
    mutationFn: (id: string) =>
      apiFetch<void>(`/importlists/${id}/test`, { method: "POST" }),
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useSyncAllImportLists() {
  return useMutation({
    mutationFn: () =>
      apiFetch<ImportListSyncResult>("/importlists/sync", { method: "POST" }),
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useSyncImportList() {
  return useMutation({
    mutationFn: (id: string) =>
      apiFetch<ImportListSyncResult>(`/importlists/${id}/sync`, {
        method: "POST",
      }),
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useImportListPreview() {
  return useMutation({
    mutationFn: (body: { kind: string; settings: Record<string, unknown> }) =>
      apiFetch<ImportListPreviewItem[]>("/importlists/preview", {
        method: "POST",
        body: JSON.stringify(body),
      }),
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useImportExclusions() {
  return useQuery({
    queryKey: ["importExclusions"],
    queryFn: () => apiFetch<ImportExclusion[]>("/importlists/exclusions"),
  });
}

export function useCreateImportExclusion() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Omit<ImportExclusion, "id" | "created_at">) =>
      apiFetch<ImportExclusion>("/importlists/exclusions", {
        method: "POST",
        body: JSON.stringify(body),
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["importExclusions"] });
      toast.success("Exclusion added");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useDeleteImportExclusion() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      apiFetch<void>(`/importlists/exclusions/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["importExclusions"] });
      toast.success("Exclusion removed");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}
