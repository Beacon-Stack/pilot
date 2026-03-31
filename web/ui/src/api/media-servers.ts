import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiFetch } from "./client";
import type { MediaServerConfig, MediaServerRequest } from "@/types";

export function useMediaServers() {
  return useQuery({
    queryKey: ["media-servers"],
    queryFn: () => apiFetch<MediaServerConfig[]>("/media-servers"),
  });
}

export function useCreateMediaServer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: MediaServerRequest) =>
      apiFetch<MediaServerConfig>("/media-servers", { method: "POST", body: JSON.stringify(body) }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["media-servers"] });
      toast.success("Media server saved");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useUpdateMediaServer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: MediaServerRequest & { id: string }) =>
      apiFetch<MediaServerConfig>(`/media-servers/${id}`, { method: "PUT", body: JSON.stringify(body) }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["media-servers"] });
      toast.success("Media server saved");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useDeleteMediaServer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => apiFetch<void>(`/media-servers/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["media-servers"] });
      toast.success("Media server deleted");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useTestMediaServer() {
  return useMutation({
    mutationFn: (id: string) => apiFetch<void>(`/media-servers/${id}/test`, { method: "POST" }),
    onError: (err) => toast.error((err as Error).message),
  });
}
