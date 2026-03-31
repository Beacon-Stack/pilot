export default function Pill({ ok, labelTrue, labelFalse }: { ok: boolean; labelTrue: string; labelFalse: string }) {
  return (
    <span
      style={{
        display: "inline-flex",
        alignItems: "center",
        padding: "2px 8px",
        borderRadius: 4,
        fontSize: 12,
        fontWeight: 500,
        color: ok ? "var(--color-success)" : "var(--color-text-muted)",
        background: ok
          ? "color-mix(in srgb, var(--color-success) 12%, transparent)"
          : "var(--color-bg-subtle)",
      }}
    >
      {ok ? labelTrue : labelFalse}
    </span>
  );
}
