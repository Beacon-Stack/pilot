import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "./client";

// Shape returned by GET /api/v1/settings/providers/{name}. Value is
// NEVER returned — only a redacted preview of the last 3 characters.
export interface ProviderStatus {
  name: string;
  source: "default" | "override";
  preview: string;
  hasDefault: boolean;
  hasOverride: boolean;
}

export function useProviderStatus(name: string) {
  return useQuery({
    queryKey: ["providers", name],
    queryFn: () => apiFetch<ProviderStatus>(`/settings/providers/${name}`),
  });
}

export function useSetProviderOverride(name: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (value: string) =>
      apiFetch<ProviderStatus>(`/settings/providers/${name}`, {
        method: "PUT",
        body: JSON.stringify({ value }),
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["providers", name] });
    },
  });
}

export function useClearProviderOverride(name: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () =>
      apiFetch<ProviderStatus>(`/settings/providers/${name}`, {
        method: "DELETE",
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["providers", name] });
    },
  });
}
