import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "./client";

// Closed set of activity_log.category values, mirrored from
// internal/core/activity/categories.go. Keep in sync — the migration in
// internal/db/migrations/00009_activity_categories.sql is forward-only.
//
// "grab" / "import" are legacy values left valid for back-compat — the
// API still accepts them as filter inputs but the new emit pipeline
// never writes them.
export type ActivityCategory =
  | "grab_succeeded"
  | "grab_failed"
  | "import_succeeded"
  | "import_failed"
  | "stalled"
  | "show"
  | "task"
  | "health"
  | "grab"
  | "import";

export interface ActivityEntry {
  id: string;
  type: string;
  category: ActivityCategory | string; // string fallback so unknown values don't break clients
  series_id?: string;
  title: string;
  detail?: Record<string, unknown>;
  created_at: string;
}

export interface ActivityListResult {
  activities: ActivityEntry[];
  total: number;
}

export function useActivity(opts?: { category?: string; since?: string; limit?: number }) {
  const params = new URLSearchParams();
  if (opts?.category) params.set("category", opts.category);
  if (opts?.since) params.set("since", opts.since);
  if (opts?.limit) params.set("limit", String(opts.limit));
  const qs = params.toString();

  return useQuery({
    queryKey: ["activity", opts ?? {}],
    queryFn: () => apiFetch<ActivityListResult>(qs ? `/activity?${qs}` : "/activity"),
    refetchInterval: 15_000,
  });
}

export type AttentionKind = "grab_failed" | "import_failed" | "stalled";

export interface AttentionItem {
  kind: AttentionKind;
  grab_id?: string;
  series_id?: string;
  episode_id?: string;
  release_title: string;
  detail?: string;
  info_hash?: string;
  created_at: string;
}

export interface AttentionResult {
  items: AttentionItem[];
  counts: {
    grab_failed: number;
    import_failed: number;
    stalled: number;
  };
}

export function useNeedsAttention(windowHours = 48) {
  return useQuery({
    queryKey: ["activity", "needs-attention", windowHours],
    queryFn: () =>
      apiFetch<AttentionResult>(`/activity/needs-attention?window_hours=${windowHours}`),
    refetchInterval: 30_000,
  });
}
