import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiFetch } from "./client";
import type { DownloadClientConfig, DownloadClientRequest, TestResult } from "@/types";

export function useDownloadClients() {
  return useQuery({
    queryKey: ["download-clients"],
    queryFn: () => apiFetch<DownloadClientConfig[]>("/download-clients"),
  });
}

export function useCreateDownloadClient() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: DownloadClientRequest) =>
      apiFetch<DownloadClientConfig>("/download-clients", { method: "POST", body: JSON.stringify(body) }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["download-clients"] });
      toast.success("Download client saved");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useUpdateDownloadClient() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: DownloadClientRequest & { id: string }) =>
      apiFetch<DownloadClientConfig>(`/download-clients/${id}`, { method: "PUT", body: JSON.stringify(body) }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["download-clients"] });
      toast.success("Download client saved");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useDeleteDownloadClient() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => apiFetch<void>(`/download-clients/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["download-clients"] });
      toast.success("Download client deleted");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useTestDownloadClient() {
  return useMutation({
    mutationFn: (id: string) => apiFetch<TestResult>(`/download-clients/${id}/test`, { method: "POST" }),
    onError: (err) => toast.error((err as Error).message),
  });
}
