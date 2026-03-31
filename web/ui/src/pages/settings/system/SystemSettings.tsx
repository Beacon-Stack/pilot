import { useSystemStatus } from "@/api/system";
import PageHeader from "@/components/PageHeader";

export default function SystemSettings() {
  const { data: status } = useSystemStatus();

  const rows = status
    ? [
        { label: "App Name",    value: status.app_name },
        { label: "Version",     value: status.version },
        { label: "Build Time",  value: status.build_time },
        { label: "Go Version",  value: status.go_version },
        { label: "Database",    value: status.db_type },
        { label: "DB Path",     value: status.db_path ?? "—" },
        { label: "Uptime",      value: `${Math.floor(status.uptime_seconds / 60)}m` },
      ]
    : [];

  return (
    <>
      <PageHeader
        title="System"
        description="Runtime information about this Screenarr instance."
      />

      <div
        style={{
          background: "var(--color-bg-surface)",
          border: "1px solid var(--color-border-subtle)",
          borderRadius: 8,
          overflow: "hidden",
          boxShadow: "var(--shadow-card)",
        }}
      >
        {rows.map((row, idx) => (
          <div
            key={row.label}
            style={{
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
              padding: "12px 16px",
              borderBottom: idx < rows.length - 1 ? "1px solid var(--color-border-subtle)" : "none",
            }}
          >
            <span style={{ fontSize: 13, color: "var(--color-text-secondary)" }}>
              {row.label}
            </span>
            <span
              style={{
                fontSize: 13,
                color: "var(--color-text-primary)",
                fontFamily: row.label === "Version" || row.label === "Build Time" || row.label === "DB Path" || row.label === "Go Version"
                  ? "var(--font-family-mono)"
                  : undefined,
              }}
            >
              {row.value}
            </span>
          </div>
        ))}
        {!status && (
          <div style={{ padding: 16 }}>
            {Array.from({ length: 5 }).map((_, i) => (
              <div key={i} className="skeleton" style={{ height: 14, borderRadius: 4, marginBottom: 8, width: i % 2 === 0 ? "60%" : "40%" }} />
            ))}
          </div>
        )}
      </div>
    </>
  );
}
