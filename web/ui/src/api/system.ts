import { useQuery, useMutation } from "@tanstack/react-query";
import { apiFetch } from "./client";
import type { SystemStatus } from "@/types";

export type { SystemStatus };

export function useSystemStatus() {
  return useQuery({
    queryKey: ["system", "status"],
    queryFn: () => apiFetch<SystemStatus>("/system/status"),
    refetchInterval: 30_000,
  });
}

export function useRunTask() {
  return useMutation({
    mutationFn: (taskName: string) =>
      apiFetch<void>(`/tasks/${taskName}/run`, { method: "POST" }),
  });
}
