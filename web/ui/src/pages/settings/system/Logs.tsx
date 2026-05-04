// Settings → System → Logs page. Thin wrapper around the shared
// LogViewer component (web-shared/LogViewer.tsx) — every Beacon
// service's log UI uses the same component so look + feel stays
// consistent across Pulse, Pilot, Prism, Haul.

import LogViewer from "@beacon-shared/LogViewer";

export default function LogsPage() {
  return (
    <div style={{ padding: "24px 32px", maxWidth: 1300, margin: "0 auto" }}>
      <div style={{ marginBottom: 20 }}>
        <h1 style={{ fontSize: 22, fontWeight: 600, color: "var(--color-text-primary)", margin: 0 }}>
          Logs
        </h1>
        <p style={{ fontSize: 13, color: "var(--color-text-secondary)", margin: "4px 0 0" }}>
          Inspect Pilot's recent log entries. Switch the source to{" "}
          <strong>Docker stdout</strong> for full history (when{" "}
          <code style={{ fontSize: 12 }}>/var/run/docker.sock</code> is mounted).
          Bump the runtime level to <strong>debug</strong> while
          troubleshooting and back to <strong>info</strong> when done — no
          restart needed.
        </p>
      </div>

      <LogViewer serviceName="Pilot" />
    </div>
  );
}
