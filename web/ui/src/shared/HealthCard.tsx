import { useState } from "react";
import type { ReactNode } from "react";
import SystemCard from "./SystemCard";

export type HealthLevel = "ok" | "warning" | "error";
export type HealthOverall = "healthy" | "degraded" | "unhealthy";

export interface HealthLine {
  name: string;
  level: HealthLevel;
  message: string;
}

interface HealthCardProps {
  /** Overall state; undefined while loading. */
  overall?: HealthOverall;
  /** One line per check or problem, worst first. */
  lines?: HealthLine[];
  loading?: boolean;
  failed?: boolean;
  /** e.g. when the checks last ran. */
  subtitle?: ReactNode;
  /** Runs the checks again (or reloads them). */
  onCheck: () => Promise<unknown>;
}

const COLOR: Record<HealthLevel, string> = {
  ok: "var(--color-success)",
  warning: "var(--color-warning)",
  error: "var(--color-danger)",
};
const OVERALL: Record<HealthOverall, { text: string; level: HealthLevel }> = {
  healthy: { text: "All systems healthy", level: "ok" },
  degraded: { text: "Degraded", level: "warning" },
  unhealthy: { text: "Unhealthy", level: "error" },
};

function Dot({ level, size }: { level: HealthLevel; size: number }) {
  return <span aria-label={level} style={{ width: size, height: size, borderRadius: "50%", background: COLOR[level], flexShrink: 0 }} />;
}

/**
 * The Health card on every Beacon app's System page: an overall line,
 * one line per check or problem, and a Check now button.
 */
export default function HealthCard({ overall, lines = [], loading, failed, subtitle, onCheck }: HealthCardProps) {
  const [checking, setChecking] = useState(false);
  const [checkError, setCheckError] = useState<string | null>(null);

  async function check() {
    setChecking(true);
    setCheckError(null);
    try {
      await onCheck();
    } catch (err) {
      setCheckError(err instanceof Error ? err.message : "Health check failed");
    } finally {
      setChecking(false);
    }
  }

  return (
    <SystemCard
      title="Health"
      subtitle={subtitle}
      action={
        <button
          type="button"
          onClick={() => void check()}
          disabled={checking}
          style={{
            padding: "6px 12px",
            borderRadius: 6,
            border: "1px solid var(--color-border-default)",
            background: "var(--color-bg-elevated)",
            color: "var(--color-text-primary)",
            fontSize: 12,
            cursor: checking ? "not-allowed" : "pointer",
          }}
        >
          {checking ? "Checking…" : "Check now"}
        </button>
      }
    >
      {checkError && (
        <div role="alert" style={{ padding: "10px 16px", fontSize: 13, color: "var(--color-danger)", borderBottom: "1px solid var(--color-border-subtle)" }}>
          {checkError}
        </div>
      )}
      {loading && <div className="skeleton" style={{ height: 40, margin: 16, borderRadius: 4 }} />}
      {failed && !loading && <div style={{ padding: 16, fontSize: 13, color: "var(--color-text-secondary)" }}>Failed to load health data.</div>}
      {overall && !loading && (
        <>
          <div
            style={{
              display: "flex",
              alignItems: "center",
              gap: 8,
              padding: "12px 16px",
              borderBottom: lines.length ? "1px solid var(--color-border-subtle)" : "none",
              fontSize: 13,
              fontWeight: 500,
              color: COLOR[OVERALL[overall].level],
            }}
          >
            <Dot level={OVERALL[overall].level} size={8} />
            {OVERALL[overall].text}
          </div>
          {lines.map((l, i) => (
            <div
              key={`${l.name}-${i}`}
              style={{
                display: "flex",
                alignItems: "center",
                gap: 10,
                padding: "10px 16px",
                borderBottom: i < lines.length - 1 ? "1px solid var(--color-border-subtle)" : "none",
                fontSize: 13,
              }}
            >
              <Dot level={l.level} size={6} />
              <span style={{ color: "var(--color-text-primary)", fontWeight: 500, minWidth: 140 }}>{l.name}</span>
              <span style={{ color: "var(--color-text-secondary)", minWidth: 0, overflowWrap: "anywhere" }}>{l.message}</span>
            </div>
          ))}
        </>
      )}
    </SystemCard>
  );
}
