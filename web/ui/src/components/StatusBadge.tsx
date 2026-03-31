const STATUS_STYLES: Record<string, { bg: string; color: string; label: string }> = {
  downloading: {
    bg: "color-mix(in srgb, var(--color-accent) 15%, transparent)",
    color: "var(--color-accent)",
    label: "Downloading",
  },
  queued: {
    bg: "color-mix(in srgb, var(--color-warning) 15%, transparent)",
    color: "var(--color-warning)",
    label: "Queued",
  },
  completed: {
    bg: "color-mix(in srgb, var(--color-success) 15%, transparent)",
    color: "var(--color-success)",
    label: "Completed",
  },
  paused: {
    bg: "color-mix(in srgb, var(--color-text-muted) 15%, transparent)",
    color: "var(--color-text-muted)",
    label: "Paused",
  },
  failed: {
    bg: "color-mix(in srgb, var(--color-danger) 15%, transparent)",
    color: "var(--color-danger)",
    label: "Failed",
  },
  removed: {
    bg: "color-mix(in srgb, var(--color-text-muted) 15%, transparent)",
    color: "var(--color-text-muted)",
    label: "Removed",
  },
};

const FALLBACK = {
  bg: "color-mix(in srgb, var(--color-text-muted) 15%, transparent)",
  color: "var(--color-text-muted)",
};

export default function StatusBadge({ status }: { status: string }) {
  const style = STATUS_STYLES[status];
  return (
    <span
      style={{
        display: "inline-block",
        background: style?.bg ?? FALLBACK.bg,
        color: style?.color ?? FALLBACK.color,
        borderRadius: 4,
        padding: "2px 8px",
        fontSize: 11,
        fontWeight: 600,
        letterSpacing: "0.04em",
        textTransform: "capitalize",
        whiteSpace: "nowrap",
      }}
    >
      {style?.label ?? status}
    </span>
  );
}
