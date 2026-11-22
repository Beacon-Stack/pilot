import { useCheckHealth, useHealth } from "@/api/health";
import type { HealthIssue } from "@/types";
import HealthCard, { type HealthOverall } from "@beacon-shared/HealthCard";

const SOURCE_LABEL: Record<HealthIssue["source"], string> = {
  library: "Library",
  download_client: "Download client",
  indexer: "Indexer",
};

function checkedLabel(at: string): string {
  if (!at) return "Clients and indexers not tested yet";
  return `Clients and indexers tested ${new Date(at).toLocaleString()}`;
}

export default function HealthPanel() {
  const { data, isLoading, error } = useHealth();
  const check = useCheckHealth();

  let overall: HealthOverall | undefined;
  if (data) {
    overall = data.issues.some((i) => i.level === "error")
      ? "unhealthy"
      : data.issues.length > 0
        ? "degraded"
        : "healthy";
  }

  return (
    <HealthCard
      overall={overall}
      lines={data?.issues.map((i) => ({ name: SOURCE_LABEL[i.source] ?? i.source, level: i.level, message: i.message }))}
      loading={isLoading}
      failed={!!error}
      subtitle={data ? checkedLabel(data.connectivity_checked_at) : undefined}
      onCheck={() => check.mutateAsync()}
    />
  );
}
