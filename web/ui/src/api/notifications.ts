import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiFetch } from "./client";
import type { NotificationConfig, NotificationRequest } from "@/types";

export function useNotifications() {
  return useQuery({
    queryKey: ["notifications"],
    queryFn: () => apiFetch<NotificationConfig[]>("/notifications"),
  });
}

export function useCreateNotification() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: NotificationRequest) =>
      apiFetch<NotificationConfig>("/notifications", { method: "POST", body: JSON.stringify(body) }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["notifications"] });
      toast.success("Notification saved");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useUpdateNotification() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: NotificationRequest & { id: string }) =>
      apiFetch<NotificationConfig>(`/notifications/${id}`, { method: "PUT", body: JSON.stringify(body) }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["notifications"] });
      toast.success("Notification saved");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useDeleteNotification() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => apiFetch<void>(`/notifications/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["notifications"] });
      toast.success("Notification deleted");
    },
    onError: (err) => toast.error((err as Error).message),
  });
}

export function useTestNotification() {
  return useMutation({
    mutationFn: (id: string) => apiFetch<void>(`/notifications/${id}/test`, { method: "POST" }),
    onError: (err) => toast.error((err as Error).message),
  });
}
