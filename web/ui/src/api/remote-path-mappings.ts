import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "./client";
import type { RemotePathMapping, RemotePathMappingRequest } from "@/types";

const KEY = ["remote-path-mappings"];

export function useRemotePathMappings() {
  return useQuery({
    queryKey: KEY,
    queryFn: () => apiFetch<RemotePathMapping[]>("/remote-path-mappings"),
  });
}

export function useSaveRemotePathMapping() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, body }: { id?: string; body: RemotePathMappingRequest }) =>
      apiFetch<RemotePathMapping>(id ? `/remote-path-mappings/${id}` : "/remote-path-mappings", {
        method: id ? "PUT" : "POST",
        body: JSON.stringify(body),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: KEY }),
  });
}

export function useDeleteRemotePathMapping() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => apiFetch<void>(`/remote-path-mappings/${id}`, { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: KEY }),
  });
}
