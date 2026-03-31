interface QualityBadgeProps {
  quality?: { source?: string; resolution?: string };
  source?: string;
  resolution?: string;
}

export default function QualityBadge({ quality, source, resolution }: QualityBadgeProps) {
  const src = quality?.source ?? source;
  const res = quality?.resolution ?? resolution;
  const label = [res, src].filter(Boolean).join(" ");

  if (!label) {
    return <span style={{ color: "var(--color-text-muted)", fontSize: 11 }}>—</span>;
  }

  return (
    <span
      style={{
        display: "inline-block",
        padding: "2px 6px",
        borderRadius: 4,
        fontSize: 10,
        fontWeight: 600,
        textTransform: "uppercase",
        letterSpacing: "0.05em",
        background: "color-mix(in srgb, var(--color-accent) 12%, transparent)",
        color: "var(--color-accent)",
      }}
    >
      {label}
    </span>
  );
}
