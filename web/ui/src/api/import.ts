import { useMutation } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiFetch } from "./client";
import type {
  SonarrPreviewResult,
  SonarrImportOptions,
  SonarrImportResult,
} from "@/types";

export function useSonarrPreview() {
  return useMutation({
    mutationFn: (body: { url: string; api_key: string }) =>
      apiFetch<SonarrPreviewResult>("/import/sonarr/preview", {
        method: "POST",
        body: JSON.stringify(body),
      }),
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useSonarrImport() {
  return useMutation({
    mutationFn: (body: {
      url: string;
      api_key: string;
      options: SonarrImportOptions;
    }) =>
      apiFetch<SonarrImportResult>("/import/sonarr/execute", {
        method: "POST",
        body: JSON.stringify(body),
      }),
    onError: (err) => toast.error((err as Error).message),
  });
}
