import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "./client";
import type { BackupInfo, BackupList, BackupSettings } from "@/types";

const LIST = ["system", "backups"];
const SETTINGS = ["system", "backup-settings"];

export function useBackups() {
  return useQuery({ queryKey: LIST, queryFn: () => apiFetch<BackupList>("/system/backups") });
}

export function useCreateBackup() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => apiFetch<BackupInfo>("/system/backups", { method: "POST" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: LIST }),
  });
}

export function useDeleteBackup() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => apiFetch<void>(`/system/backups/${encodeURIComponent(name)}`, { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: LIST }),
  });
}

export function useRestoreBackup() {
  return useMutation({
    mutationFn: (name: string) =>
      apiFetch<{ message: string }>(`/system/backups/${encodeURIComponent(name)}/restore`, { method: "POST" }),
  });
}

export function useBackupSettings() {
  return useQuery({ queryKey: SETTINGS, queryFn: () => apiFetch<BackupSettings>("/system/backup-settings") });
}

export function useSaveBackupSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: BackupSettings) =>
      apiFetch<BackupSettings>("/system/backup-settings", { method: "PUT", body: JSON.stringify(body) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: SETTINGS }),
  });
}

export function backupDownloadURL(name: string): string {
  return `/api/v1/system/backups/${encodeURIComponent(name)}/download`;
}

/** 1536 → "1.5 KB", 5_000_000 → "4.8 MB". */
export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  const units = ["KB", "MB", "GB"];
  let v = n / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(1)} ${units[i]}`;
}
