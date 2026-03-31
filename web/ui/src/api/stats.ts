import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "./client";

export interface CollectionStats {
  total_series: number;
  total_episodes: number;
  monitored: number;
  with_file: number;
  missing: number;
}

export interface StorageStats {
  total_bytes: number;
  file_count: number;
}

export interface QualityTier {
  resolution: string;
  source: string;
  count: number;
}

export interface GrowthPoint {
  snapshot_at: string;
  total_series: number;
  total_episodes: number;
  with_file: number;
  total_size_bytes: number;
}

export function useCollectionStats() {
  return useQuery({
    queryKey: ["stats", "collection"],
    queryFn: () => apiFetch<CollectionStats>("/stats/collection"),
    staleTime: 60_000,
  });
}

export function useStorageStats() {
  return useQuery({
    queryKey: ["stats", "storage"],
    queryFn: () => apiFetch<StorageStats>("/stats/storage"),
    staleTime: 60_000,
  });
}

export function useQualityTiers() {
  return useQuery({
    queryKey: ["stats", "quality-tiers"],
    queryFn: () => apiFetch<QualityTier[]>("/stats/quality-tiers"),
    staleTime: 60_000,
  });
}

export function useGrowthStats() {
  return useQuery({
    queryKey: ["stats", "growth"],
    queryFn: () => apiFetch<GrowthPoint[]>("/stats/growth"),
    staleTime: 60_000,
  });
}
