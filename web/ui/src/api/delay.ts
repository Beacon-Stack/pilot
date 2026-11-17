import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "./client";
import type { DelayProfile, PendingRelease } from "@/types";

export function useDelayProfile() {
  return useQuery({
    queryKey: ["delay-profile"],
    queryFn: () => apiFetch<DelayProfile>("/delay-profile"),
  });
}

export function useSaveDelayProfile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: DelayProfile) =>
      apiFetch<DelayProfile>("/delay-profile", { method: "PUT", body: JSON.stringify(body) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["delay-profile"] }),
  });
}

export function usePendingReleases() {
  return useQuery({
    queryKey: ["queue", "pending"],
    queryFn: () => apiFetch<PendingRelease[]>("/queue/pending"),
    refetchInterval: 60_000,
  });
}

export function useDropPendingRelease() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => apiFetch<void>(`/queue/pending/${id}`, { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["queue", "pending"] }),
  });
}

/** "in 45 min", "in 2 h 5 min", or "next RSS sync" once the time has passed. */
export function formatWait(eligibleAt: string, now: Date = new Date()): string {
  const ms = new Date(eligibleAt).getTime() - now.getTime();
  if (!Number.isFinite(ms) || ms <= 0) return "next RSS sync";
  const totalMin = Math.ceil(ms / 60_000);
  const h = Math.floor(totalMin / 60);
  const m = totalMin % 60;
  if (h === 0) return `in ${m} min`;
  return m === 0 ? `in ${h} h` : `in ${h} h ${m} min`;
}
