import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "./client";
import type { Library } from "@/types";

export function useLibraries() {
  return useQuery({
    queryKey: ["libraries"],
    queryFn: () => apiFetch<Library[]>("/libraries"),
  });
}

export function useCreateLibrary() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Omit<Library, "id" | "created_at" | "updated_at">) =>
      apiFetch<Library>("/libraries", { method: "POST", body: JSON.stringify(body) }),
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ["libraries"] }); },
  });
}

export function useUpdateLibrary(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Partial<Library>) =>
      apiFetch<Library>(`/libraries/${id}`, { method: "PUT", body: JSON.stringify(body) }),
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ["libraries"] }); },
  });
}

export function useDeleteLibrary() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      apiFetch<void>(`/libraries/${id}`, { method: "DELETE" }),
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ["libraries"] }); },
  });
}
