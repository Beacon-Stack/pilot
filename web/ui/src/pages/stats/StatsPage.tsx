import {
  useCollectionStats,
  useStorageStats,
  useQualityTiers,
  useGrowthStats,
  type CollectionStats,
  type StorageStats,
  type QualityTier,
  type GrowthPoint,
} from "@/api/stats";
import { formatBytes } from "@/lib/utils";

// ── Card shell ────────────────────────────────────────────────────────────────

function Card({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div
      style={{
        background: "var(--color-bg-elevated)",
        borderRadius: 12,
        border: "1px solid var(--color-border-subtle)",
        padding: "20px 24px",
      }}
    >
      <h2
        style={{
          margin: "0 0 18px",
          fontSize: 13,
          fontWeight: 600,
          color: "var(--color-text-muted)",
          textTransform: "uppercase",
          letterSpacing: "0.08em",
        }}
      >
        {title}
      </h2>
      {children}
    </div>
  );
}

// ── Stat block ────────────────────────────────────────────────────────────────

function StatBlock({
  label,
  value,
  accent,
}: {
  label: string;
  value: string | number;
  accent?: string;
}) {
  return (
    <div style={{ flex: 1, minWidth: 100 }}>
      <div
        style={{
          fontSize: 28,
          fontWeight: 700,
          color: accent ?? "var(--color-text-primary)",
          lineHeight: 1,
          marginBottom: 6,
        }}
      >
        {value}
      </div>
      <div style={{ fontSize: 12, color: "var(--color-text-muted)", fontWeight: 500 }}>
        {label}
      </div>
    </div>
  );
}

// ── Skeleton ──────────────────────────────────────────────────────────────────

function CardSkeleton({ height = 200 }: { height?: number }) {
  return (
    <div
      className="skeleton"
      style={{ borderRadius: 12, height, background: "var(--color-bg-elevated)" }}
    />
  );
}

function ErrorCard({ title }: { title: string }) {
  return (
    <Card title={title}>
      <p style={{ color: "var(--color-danger, #ef4444)", margin: 0, fontSize: 13 }}>
        Failed to load.
      </p>
    </Card>
  );
}

// ── Collection card ───────────────────────────────────────────────────────────

function CollectionCard({ data }: { data: CollectionStats }) {
  return (
    <Card title="Collection">
      <div style={{ display: "flex", gap: 24, flexWrap: "wrap" }}>
        <StatBlock label="Total Series" value={data.total_series.toLocaleString()} />
        <StatBlock label="Total Episodes" value={data.total_episodes.toLocaleString()} />
        <StatBlock label="Monitored" value={data.monitored.toLocaleString()} />
        <StatBlock label="Have File" value={data.with_file.toLocaleString()} />
        <StatBlock
          label="Missing"
          value={data.missing.toLocaleString()}
          accent={data.missing > 0 ? "var(--color-warning)" : undefined}
        />
      </div>
    </Card>
  );
}

// ── Storage card ──────────────────────────────────────────────────────────────

function StorageCard({ data }: { data: StorageStats }) {
  return (
    <Card title="Storage">
      <div style={{ display: "flex", gap: 32, flexWrap: "wrap" }}>
        <StatBlock label="Total Used" value={formatBytes(data.total_bytes)} />
        <StatBlock label="Files" value={data.file_count.toLocaleString()} />
        {data.file_count > 0 && (
          <StatBlock
            label="Avg per File"
            value={formatBytes(Math.round(data.total_bytes / data.file_count))}
          />
        )}
      </div>
    </Card>
  );
}

// ── Quality tiers card (CSS bar chart — no recharts) ──────────────────────────

function QualityTiersCard({ data }: { data: QualityTier[] }) {
  const sorted = [...data]
    .filter((t) => t.count > 0)
    .sort((a, b) => b.count - a.count);

  if (sorted.length === 0) {
    return (
      <Card title="Quality Distribution">
        <p style={{ color: "var(--color-text-muted)", fontSize: 13, margin: 0 }}>
          No episode files yet.
        </p>
      </Card>
    );
  }

  const max = sorted[0].count;

  return (
    <Card title="Quality Distribution">
      <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
        {sorted.map((tier) => {
          const label = [tier.resolution, tier.source].filter(Boolean).join(" ");
          const pct = max > 0 ? (tier.count / max) * 100 : 0;
          return (
            <div key={label} style={{ display: "flex", alignItems: "center", gap: 12 }}>
              <div
                style={{
                  width: 100,
                  fontSize: 12,
                  color: "var(--color-text-secondary)",
                  flexShrink: 0,
                  textAlign: "right",
                  fontWeight: 500,
                }}
              >
                {label || "Unknown"}
              </div>
              <div
                style={{
                  flex: 1,
                  height: 8,
                  borderRadius: 4,
                  background: "var(--color-bg-surface)",
                  overflow: "hidden",
                }}
              >
                <div
                  style={{
                    height: "100%",
                    width: `${pct}%`,
                    borderRadius: 4,
                    background: "var(--color-accent)",
                    transition: "width 400ms ease",
                  }}
                />
              </div>
              <div
                style={{
                  width: 36,
                  fontSize: 12,
                  color: "var(--color-text-muted)",
                  flexShrink: 0,
                  fontVariantNumeric: "tabular-nums",
                }}
              >
                {tier.count.toLocaleString()}
              </div>
            </div>
          );
        })}
      </div>
    </Card>
  );
}

// ── Growth card (CSS sparkline — no recharts) ─────────────────────────────────

function GrowthCard({ data }: { data: GrowthPoint[] }) {
  if (data.length < 2) {
    return (
      <Card title="Library Growth">
        <p style={{ color: "var(--color-text-muted)", fontSize: 13, margin: 0 }}>
          Keep adding series — growth chart will appear here.
        </p>
      </Card>
    );
  }

  const max = Math.max(...data.map((p) => p.total_series));
  const chartH = 80;
  const chartW = 400;

  const points = data.map((p, i) => {
    const x = (i / (data.length - 1)) * chartW;
    const y = max > 0 ? chartH - (p.total_series / max) * chartH : chartH;
    return `${x},${y}`;
  });

  const polyline = points.join(" ");
  const areaPath = `M0,${chartH} L${polyline.replace(/,/g, " L").split(" L").join(" L")} L${chartW},${chartH} Z`;

  return (
    <Card title="Library Growth">
      <svg
        viewBox={`0 0 ${chartW} ${chartH}`}
        style={{ width: "100%", height: chartH, display: "block", overflow: "visible" }}
        preserveAspectRatio="none"
      >
        <defs>
          <linearGradient id="growthGrad" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="var(--color-accent)" stopOpacity="0.25" />
            <stop offset="100%" stopColor="var(--color-accent)" stopOpacity="0" />
          </linearGradient>
        </defs>
        <path d={areaPath} fill="url(#growthGrad)" />
        <polyline
          points={polyline}
          fill="none"
          stroke="var(--color-accent)"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </svg>
      <div style={{ display: "flex", justifyContent: "space-between", marginTop: 8, fontSize: 11, color: "var(--color-text-muted)" }}>
        <span>{new Date(data[0].snapshot_at).toLocaleDateString(undefined, { month: "short", year: "2-digit" })}</span>
        <span style={{ color: "var(--color-text-secondary)", fontWeight: 500 }}>
          {data[data.length - 1].total_series.toLocaleString()} series now
        </span>
        <span>{new Date(data[data.length - 1].snapshot_at).toLocaleDateString(undefined, { month: "short", year: "2-digit" })}</span>
      </div>
    </Card>
  );
}

// ── Page ──────────────────────────────────────────────────────────────────────

export default function StatsPage() {
  const collection = useCollectionStats();
  const storage = useStorageStats();
  const qualityTiers = useQualityTiers();
  const growth = useGrowthStats();

  const twoCol: React.CSSProperties = {
    display: "grid",
    gridTemplateColumns: "repeat(auto-fill, minmax(340px, 1fr))",
    gap: 20,
  };

  return (
    <div style={{ padding: "32px 32px 64px", maxWidth: 1100, margin: "0 auto" }}>
      <h1
        style={{
          fontSize: 20,
          fontWeight: 600,
          color: "var(--color-text-primary)",
          marginBottom: 24,
          letterSpacing: "-0.01em",
        }}
      >
        Statistics
      </h1>

      <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
        {/* Collection — full width */}
        {collection.isLoading ? (
          <CardSkeleton height={110} />
        ) : collection.error ? (
          <ErrorCard title="Collection" />
        ) : collection.data ? (
          <CollectionCard data={collection.data} />
        ) : null}

        {/* Storage | Growth */}
        <div style={twoCol}>
          {storage.isLoading ? (
            <CardSkeleton height={140} />
          ) : storage.error ? (
            <ErrorCard title="Storage" />
          ) : storage.data ? (
            <StorageCard data={storage.data} />
          ) : null}

          {growth.isLoading ? (
            <CardSkeleton height={160} />
          ) : growth.error ? (
            <ErrorCard title="Library Growth" />
          ) : growth.data ? (
            <GrowthCard data={growth.data} />
          ) : null}
        </div>

        {/* Quality distribution — full width */}
        {qualityTiers.isLoading ? (
          <CardSkeleton height={200} />
        ) : qualityTiers.error ? (
          <ErrorCard title="Quality Distribution" />
        ) : qualityTiers.data ? (
          <QualityTiersCard data={qualityTiers.data} />
        ) : null}
      </div>
    </div>
  );
}
