import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "./client";

// How series progress is shown.
export interface DisplaySettings {
  count_specials: boolean; // count specials (season 0) in a series' totals and status
}

export function useDisplaySettings() {
  return useQuery({
    queryKey: ["display-settings"],
    queryFn: () => apiFetch<DisplaySettings>("/display-settings"),
  });
}

export function useSaveDisplaySettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: DisplaySettings) =>
      apiFetch<DisplaySettings>("/display-settings", { method: "PUT", body: JSON.stringify(body) }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["display-settings"] });
      // Series counts and states depend on it.
      qc.invalidateQueries({ queryKey: ["series"] });
    },
  });
}
