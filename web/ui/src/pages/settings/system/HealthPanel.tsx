import { AlertCircle, AlertTriangle, CheckCircle2, RefreshCw } from "lucide-react";
import { toast } from "sonner";
import { useCheckHealth, useHealth } from "@/api/health";
import type { HealthIssue } from "@/types";

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
  const { data, isLoading } = useHealth();
  const check = useCheckHealth();

  async function onCheck() {
    try {
      await check.mutateAsync();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Health check failed");
    }
  }

  return (
    <section
      aria-label="Health"
      style={{
        background: "var(--color-bg-surface)",
        border: "1px solid var(--color-border-subtle)",
        borderRadius: 8,
        overflow: "hidden",
        boxShadow: "var(--shadow-card)",
        marginBottom: 20,
      }}
    >
      <header
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: 12,
          flexWrap: "wrap",
          padding: "12px 16px",
          borderBottom: "1px solid var(--color-border-subtle)",
        }}
      >
        <div style={{ display: "flex", flexDirection: "column", gap: 2 }}>
          <h2 style={{ margin: 0, fontSize: 14, fontWeight: 600, color: "var(--color-text-primary)" }}>Health</h2>
          {data && <span style={{ fontSize: 11, color: "var(--color-text-muted)" }}>{checkedLabel(data.connectivity_checked_at)}</span>}
        </div>
        <button
          type="button"
          onClick={() => void onCheck()}
          disabled={check.isPending}
          style={{
            display: "inline-flex",
            alignItems: "center",
            gap: 6,
            padding: "6px 12px",
            borderRadius: 6,
            border: "1px solid var(--color-border-default)",
            background: "var(--color-bg-elevated)",
            color: "var(--color-text-primary)",
            fontSize: 12,
            cursor: "pointer",
          }}
        >
          <RefreshCw size={13} style={check.isPending ? { animation: "spin 1s linear infinite" } : undefined} />
          {check.isPending ? "Checking…" : "Check now"}
        </button>
      </header>

      {isLoading && <div className="skeleton" style={{ height: 40, margin: 16, borderRadius: 4 }} />}

      {data && data.issues.length === 0 && (
        <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "12px 16px", fontSize: 13, color: "var(--color-text-secondary)" }}>
          <CheckCircle2 size={16} color="var(--color-success)" /> No issues found.
        </div>
      )}

      {data && data.issues.length > 0 && (
        <ul style={{ listStyle: "none", margin: 0, padding: 0 }}>
          {data.issues.map((issue, idx) => {
            const isError = issue.level === "error";
            const Icon = isError ? AlertCircle : AlertTriangle;
            return (
              <li
                key={idx}
                style={{
                  display: "flex",
                  gap: 10,
                  padding: "10px 16px",
                  borderBottom: idx < data.issues.length - 1 ? "1px solid var(--color-border-subtle)" : "none",
                  fontSize: 13,
                  color: "var(--color-text-primary)",
                }}
              >
                <Icon size={16} style={{ flexShrink: 0, marginTop: 1 }} color={isError ? "var(--color-danger)" : "var(--color-warning)"} aria-label={issue.level} />
                <div style={{ minWidth: 0, overflowWrap: "anywhere" }}>
                  <span style={{ fontSize: 11, color: "var(--color-text-muted)", marginRight: 6 }}>{SOURCE_LABEL[issue.source] ?? issue.source}</span>
                  {issue.message}
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}
