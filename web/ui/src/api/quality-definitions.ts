import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiFetch } from "./client";
import type { QualityDefinition, QualityDefinitionUpdate } from "@/types";

export function useQualityDefinitions() {
  return useQuery({
    queryKey: ["quality-definitions"],
    queryFn: () => apiFetch<QualityDefinition[]>("/quality-definitions"),
  });
}

export function useUpdateQualityDefinitions() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (updates: QualityDefinitionUpdate[]) =>
      apiFetch<void>("/quality-definitions", {
        method: "PUT",
        body: JSON.stringify(updates),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["quality-definitions"] });
      toast.success("Quality definitions saved");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}
