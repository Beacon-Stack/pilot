import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiFetch } from "./client";
import type { DownloadHandling, RemotePathMapping, CreateRemotePathMappingRequest } from "@/types";

export function useDownloadHandling() {
  return useQuery({
    queryKey: ["download-handling"],
    queryFn: () => apiFetch<DownloadHandling>("/download-handling"),
  });
}

export function useUpdateDownloadHandling() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: DownloadHandling) =>
      apiFetch<DownloadHandling>("/download-handling", {
        method: "PUT",
        body: JSON.stringify(body),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["download-handling"] });
      toast.success("Download handling settings saved");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useRemotePathMappings() {
  return useQuery({
    queryKey: ["remote-path-mappings"],
    queryFn: () => apiFetch<RemotePathMapping[]>("/download-handling/remote-path-mappings"),
  });
}

export function useCreateRemotePathMapping() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: CreateRemotePathMappingRequest) =>
      apiFetch<RemotePathMapping>("/download-handling/remote-path-mappings", {
        method: "POST",
        body: JSON.stringify(body),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["remote-path-mappings"] });
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useDeleteRemotePathMapping() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      apiFetch<void>(`/download-handling/remote-path-mappings/${id}`, {
        method: "DELETE",
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["remote-path-mappings"] });
    },
    onError: (err) => toast.error((err as Error).message),
  });
}
