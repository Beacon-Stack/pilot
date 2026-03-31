import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiFetch } from "./client";
import type { IndexerConfig } from "@/types";

export interface IndexerRequest {
  name: string;
  kind: string;
  enabled: boolean;
  priority: number;
  settings: Record<string, unknown>;
}

export interface TestResult {
  ok: boolean;
  message?: string;
}

export function useIndexers() {
  return useQuery({
    queryKey: ["indexers"],
    queryFn: () => apiFetch<IndexerConfig[]>("/indexers"),
  });
}

export function useCreateIndexer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: IndexerRequest) =>
      apiFetch<IndexerConfig>("/indexers", { method: "POST", body: JSON.stringify(body) }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["indexers"] });
      toast.success("Indexer added");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useUpdateIndexer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { id: string } & IndexerRequest) => {
      const { id, ...rest } = body;
      return apiFetch<IndexerConfig>(`/indexers/${id}`, { method: "PUT", body: JSON.stringify(rest) });
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["indexers"] });
      toast.success("Indexer updated");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useDeleteIndexer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      apiFetch<void>(`/indexers/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["indexers"] });
      toast.success("Indexer deleted");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useTestIndexer() {
  return useMutation({
    mutationFn: (id: string) =>
      apiFetch<TestResult>(`/indexers/${id}/test`, { method: "POST" }),
  });
}
