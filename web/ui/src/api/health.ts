import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "./client";
import type { HealthReport } from "@/types";

const key = ["system", "health"];

export function useHealth() {
  return useQuery({
    queryKey: key,
    queryFn: () => apiFetch<HealthReport>("/system/health"),
    refetchInterval: 5 * 60_000,
  });
}

/** Re-tests download clients and indexers now. */
export function useCheckHealth() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => apiFetch<HealthReport>("/system/health/check", { method: "POST" }),
    onSuccess: (report) => qc.setQueryData(key, report),
  });
}

/** "2 errors, 1 warning"; "" when there are no issues. */
export function summarizeHealth(report: HealthReport | undefined): string {
  if (!report) return "";
  const errors = report.issues.filter((i) => i.level === "error").length;
  const warnings = report.issues.length - errors;
  const part = (n: number, word: string) => (n === 0 ? "" : `${n} ${word}${n === 1 ? "" : "s"}`);
  return [part(errors, "error"), part(warnings, "warning")].filter(Boolean).join(", ");
}
