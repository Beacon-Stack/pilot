import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiFetch, APIError } from "./client";
import type { CustomFormat, CustomFormatPreset } from "@/types";

export function useCustomFormats() {
  return useQuery({
    queryKey: ["custom-formats"],
    queryFn: () => apiFetch<CustomFormat[]>("/custom-formats"),
    staleTime: 30_000,
  });
}

export function useDeleteCustomFormat() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      apiFetch<void>(`/custom-formats/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["custom-formats"] });
    },
  });
}

export function useCustomFormatPresets() {
  return useQuery({
    queryKey: ["custom-format-presets"],
    queryFn: () => apiFetch<CustomFormatPreset[]>("/custom-formats/presets"),
    staleTime: 60_000,
  });
}

export function useImportPreset() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (presetId: string) =>
      apiFetch<CustomFormat>(`/custom-formats/presets/${presetId}`, {
        method: "POST",
      }),
    onSuccess: (_data, presetId) => {
      toast.success(`Preset "${presetId}" imported`);
      qc.invalidateQueries({ queryKey: ["custom-formats"] });
    },
    onError: (err: Error) => {
      if (err instanceof APIError && err.status === 409) {
        toast.error("A custom format with this name already exists");
      } else {
        toast.error("Failed to import preset");
      }
    },
  });
}
