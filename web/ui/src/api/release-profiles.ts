import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "./client";
import type { ReleaseProfile, ReleaseProfileRequest } from "@/types";

const KEY = ["release-profiles"];

export function useReleaseProfiles() {
  return useQuery({
    queryKey: KEY,
    queryFn: () => apiFetch<ReleaseProfile[]>("/release-profiles"),
  });
}

export function useSaveReleaseProfile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, body }: { id?: string; body: ReleaseProfileRequest }) =>
      apiFetch<ReleaseProfile>(id ? `/release-profiles/${id}` : "/release-profiles", {
        method: id ? "PUT" : "POST",
        body: JSON.stringify(body),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: KEY }),
  });
}

export function useDeleteReleaseProfile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => apiFetch<void>(`/release-profiles/${id}`, { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: KEY }),
  });
}
