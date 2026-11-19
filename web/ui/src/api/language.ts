import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "./client";
import type { LanguagePreference } from "@/types";

export function useLanguagePreference() {
  return useQuery({
    queryKey: ["language-preference"],
    queryFn: () => apiFetch<LanguagePreference>("/language-preference"),
  });
}

export function useSaveLanguagePreference() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (languages: string[]) =>
      apiFetch<LanguagePreference>("/language-preference", { method: "PUT", body: JSON.stringify({ languages }) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["language-preference"] }),
  });
}

/** "english" → "English". */
export function languageLabel(lang: string): string {
  return lang.charAt(0).toUpperCase() + lang.slice(1);
}

/**
 * Display text for a release filter reason. Coded reasons get words;
 * the rest (e.g. "below minimum seeders (2 < 5)") are already readable.
 */
export function filterReasonLabel(reason: string, release: { languages?: string[] }): string {
  switch (reason) {
    case "unwanted_language": {
      const langs = release.languages?.length ? release.languages.map(languageLabel).join(", ") : "English (untagged)";
      return `unwanted language: ${langs}`;
    }
    case "release_profile":
      return "rejected by a release profile";
    case "below_quality_profile":
      return "not allowed by the quality profile";
    default:
      return reason;
  }
}
