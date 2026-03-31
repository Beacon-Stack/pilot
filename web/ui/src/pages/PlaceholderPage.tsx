import PageHeader from "@/components/PageHeader";

interface PlaceholderPageProps {
  title: string;
  description?: string;
}

export default function PlaceholderPage({ title, description }: PlaceholderPageProps) {
  return (
    <div style={{ padding: 24, maxWidth: 900 }}>
      <PageHeader
        title={title}
        description={description ?? ""}
      />

      <div
        style={{
          background: "var(--color-bg-surface)",
          border: "1px solid var(--color-border-subtle)",
          borderRadius: 8,
          boxShadow: "var(--shadow-card)",
          overflow: "hidden",
          padding: 48,
          textAlign: "center",
        }}
      >
        <p
          style={{
            margin: 0,
            fontSize: 14,
            color: "var(--color-text-secondary)",
            fontWeight: 500,
          }}
        >
          Coming soon
        </p>
        <p
          style={{
            margin: "6px 0 0",
            fontSize: 13,
            color: "var(--color-text-muted)",
          }}
        >
          This feature is planned for a future update.
        </p>
      </div>
    </div>
  );
}
