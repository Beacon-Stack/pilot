import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "./client";
import type { QualityProfile } from "@/types";

export function useQualityProfiles() {
  return useQuery({
    queryKey: ["quality-profiles"],
    queryFn: () => apiFetch<QualityProfile[]>("/quality-profiles"),
  });
}

export function useCreateQualityProfile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Omit<QualityProfile, "id">) =>
      apiFetch<QualityProfile>("/quality-profiles", { method: "POST", body: JSON.stringify(body) }),
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ["quality-profiles"] }); },
  });
}

export function useUpdateQualityProfile(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Partial<QualityProfile>) =>
      apiFetch<QualityProfile>(`/quality-profiles/${id}`, { method: "PUT", body: JSON.stringify(body) }),
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ["quality-profiles"] }); },
  });
}

export function useDeleteQualityProfile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      apiFetch<void>(`/quality-profiles/${id}`, { method: "DELETE" }),
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ["quality-profiles"] }); },
  });
}
