import { useMutation } from "@tanstack/react-query";
import { apiFetch } from "./client";
import type { Quality } from "@/types";

export interface ManualEpisode {
  id: string;
  season_number: number;
  episode_number: number;
  title: string;
}

export interface ManualCandidate {
  path: string;
  size: number;
  series_id: string;
  series_title: string;
  season: number;
  episodes: ManualEpisode[];
  quality: Quality;
  languages: string[];
  rejections: string[];
}

export interface ManualImportItem {
  path: string;
  series_id: string;
  episode_ids: string[];
  quality: Quality;
}

export interface ManualImportResult {
  path: string;
  ok: boolean;
  error?: string;
}

export function useManualScan() {
  return useMutation({
    mutationFn: (body: { path: string; series_id?: string }) =>
      apiFetch<{ candidates: ManualCandidate[] }>("/manual-import/scan", { method: "POST", body: JSON.stringify(body) }),
  });
}

export function useManualImport() {
  return useMutation({
    mutationFn: (body: { items: ManualImportItem[]; mode: "copy" | "move" }) =>
      apiFetch<{ results: ManualImportResult[] }>("/manual-import", { method: "POST", body: JSON.stringify(body) }),
  });
}

export const REJECTION_LABEL: Record<string, string> = {
  sample: "Sample file",
  unknown_series: "Series not recognized",
  unknown_episode: "Episode not recognized",
  already_imported: "Already in the library",
  existing_file_is_equal_or_better: "Existing file is as good or better",
};
