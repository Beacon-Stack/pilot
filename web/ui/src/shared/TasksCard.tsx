import { useEffect, useState } from "react";
import { apiFetch } from "./api";
import SystemCard from "./SystemCard";

interface Task {
  name: string;
  interval: string;
}

/** "6h0m0s" → "6h", "15m0s" → "15m", "30s" stays. */
function formatInterval(go: string): string {
  return go.replace(/(\d)m0s$/, "$1m").replace(/(\d)h0m$/, "$1h") || go;
}

/** "library_scan" → "Library scan". */
function taskLabel(name: string): string {
  const s = name.replace(/_/g, " ");
  return s.charAt(0).toUpperCase() + s.slice(1);
}

/**
 * Scheduled background tasks (GET /api/v1/tasks) with a Run now button
 * for each. Shared by the apps that have a task scheduler.
 */
export default function TasksCard() {
  const [tasks, setTasks] = useState<Task[] | null>(null);
  const [failed, setFailed] = useState(false);
  const [running, setRunning] = useState<string | null>(null);
  const [done, setDone] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let live = true;
    apiFetch<Task[]>("/tasks")
      .then((t) => live && setTasks(Array.isArray(t) ? t : []))
      .catch(() => live && setFailed(true));
    return () => {
      live = false;
    };
  }, []);

  async function run(name: string) {
    setRunning(name);
    setError(null);
    try {
      await apiFetch<void>(`/tasks/${encodeURIComponent(name)}/run`, { method: "POST" });
      setDone(name);
      setTimeout(() => setDone((d) => (d === name ? null : d)), 2000);
    } catch (err) {
      setError(`${taskLabel(name)}: ${err instanceof Error ? err.message : "failed to start"}`);
    } finally {
      setRunning(null);
    }
  }

  return (
    <SystemCard title="Tasks" subtitle="Run on their own on a schedule; start one early here.">
      {!tasks && !failed && <div className="skeleton" style={{ height: 40, margin: 16, borderRadius: 4 }} />}
      {failed && <div style={{ padding: 16, fontSize: 13, color: "var(--color-text-secondary)" }}>Couldn't load tasks.</div>}
      {tasks && tasks.length === 0 && (
        <div style={{ padding: 16, fontSize: 13, color: "var(--color-text-secondary)" }}>No tasks registered.</div>
      )}
      {error && (
        <div role="alert" style={{ padding: "10px 16px", fontSize: 13, color: "var(--color-danger)", borderBottom: "1px solid var(--color-border-subtle)" }}>
          {error}
        </div>
      )}
      {tasks?.map((t, i) => (
        <div
          key={t.name}
          style={{
            display: "flex",
            alignItems: "center",
            gap: 12,
            padding: "8px 16px",
            borderBottom: i < tasks.length - 1 ? "1px solid var(--color-border-subtle)" : "none",
            fontSize: 13,
          }}
        >
          <span style={{ flex: 1, minWidth: 0, color: "var(--color-text-primary)" }}>{taskLabel(t.name)}</span>
          <span style={{ color: "var(--color-text-muted)", fontSize: 12 }}>every {formatInterval(t.interval)}</span>
          <span style={{ width: 96, textAlign: "right" }}>
            {done === t.name ? (
              <span style={{ fontSize: 12, color: "var(--color-success)" }}>Running ✓</span>
            ) : (
              <button
                type="button"
                disabled={running !== null}
                onClick={() => void run(t.name)}
                aria-label={`Run ${taskLabel(t.name)} now`}
                style={{
                  padding: "4px 10px",
                  borderRadius: 6,
                  border: "1px solid var(--color-border-default)",
                  background: "var(--color-bg-elevated)",
                  color: "var(--color-text-secondary)",
                  fontSize: 12,
                  cursor: running ? "not-allowed" : "pointer",
                }}
              >
                {running === t.name ? "Starting…" : "Run now"}
              </button>
            )}
          </span>
        </div>
      ))}
    </SystemCard>
  );
}
