import type { ReactNode } from "react";

interface SystemCardProps {
  title: string;
  /** A short line under the title, e.g. when something was last checked. */
  subtitle?: ReactNode;
  /** A button or link on the right of the header. */
  action?: ReactNode;
  children: ReactNode;
}

/**
 * The frame every section of a Beacon app's System page sits in, so
 * About, Health and the app's own sections look the same in every app.
 */
export default function SystemCard({ title, subtitle, action, children }: SystemCardProps) {
  return (
    <section
      aria-label={title}
      style={{
        background: "var(--color-bg-surface)",
        border: "1px solid var(--color-border-subtle)",
        borderRadius: 8,
        overflow: "hidden",
        boxShadow: "var(--shadow-card)",
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
          <h2 style={{ margin: 0, fontSize: 14, fontWeight: 600, color: "var(--color-text-primary)" }}>{title}</h2>
          {subtitle && <span style={{ fontSize: 11, color: "var(--color-text-muted)" }}>{subtitle}</span>}
        </div>
        {action}
      </header>
      {children}
    </section>
  );
}

/** One label/value line inside a SystemCard. */
export function SystemRow({ label, children, last }: { label: string; children: ReactNode; last?: boolean }) {
  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        justifyContent: "space-between",
        gap: 16,
        padding: "10px 16px",
        borderBottom: last ? "none" : "1px solid var(--color-border-subtle)",
        fontSize: 13,
      }}
    >
      <span style={{ color: "var(--color-text-secondary)", flexShrink: 0 }}>{label}</span>
      <span style={{ color: "var(--color-text-primary)", minWidth: 0, overflowWrap: "anywhere", textAlign: "right" }}>{children}</span>
    </div>
  );
}
