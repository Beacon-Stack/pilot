import type { CSSProperties } from "react";

export const card: CSSProperties = {
  background: "var(--color-bg-surface)",
  border: "1px solid var(--color-border-subtle)",
  borderRadius: 8,
  padding: 20,
  boxShadow: "var(--shadow-card)",
};

export const sectionHeader: CSSProperties = {
  fontSize: 11,
  fontWeight: 600,
  letterSpacing: "0.08em",
  textTransform: "uppercase",
  color: "var(--color-text-muted)",
  marginBottom: 16,
  marginTop: 0,
};
