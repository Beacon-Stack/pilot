import { useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  BarChart,
  Bar,
  Cell,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
} from "recharts";
import {
  useCollectionStats,
  useStorageStats,
  useQualityStats,
  useQualityTiers,
  useGrowthStats,
  type CollectionStats,
  type StorageStats,
  type QualityBucket,
  type QualityTier,
  type GrowthPoint,
} from "@/api/stats";
import { formatBytes } from "@/lib/utils";

const tooltipStyle = {
  contentStyle: {
    background: "var(--color-bg-elevated)",
    border: "1px solid var(--color-border-subtle)",
    borderRadius: 8,
    fontSize: 12,
    color: "var(--color-text-primary)",
  },
  wrapperStyle: { transition: "none" },
  cursor: { fill: "color-mix(in srgb, var(--color-accent) 8%, transparent)" },
};

const axisStyle = { fontSize: 11, fill: "var(--color-text-muted)" };

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

// ── Quality card (dimension/tier dual-view, recharts) ────────────────────────

const RESOLUTION_ORDER = ["2160p", "1080p", "720p", "SD", "unknown"];
const SOURCE_ORDER = ["Remux", "Bluray", "WEBDL", "WEBRip", "HDTV", "DVD", "unknown"];
const CODEC_ORDER = ["AV1", "x265", "HEVC", "x264", "H264", "unknown"];
const HDR_ORDER = ["DolbyVision", "HDR10", "HDR10+", "HLG", "none", "unknown"];

function aggregateBy(buckets: QualityBucket[], key: keyof QualityBucket) {
  const map: Record<string, number> = {};
  for (const b of buckets) {
    const k = b[key] as string;
    map[k] = (map[k] ?? 0) + b.count;
  }
  return Object.entries(map).map(([label, count]) => ({ label, count }));
}

function sortedGroup(
  buckets: QualityBucket[],
  key: keyof QualityBucket,
  order: string[]
) {
  const items = aggregateBy(buckets, key);
  return items
    .sort((a, b) => {
      const ai = order.indexOf(a.label);
      const bi = order.indexOf(b.label);
      if (ai === -1 && bi === -1) return b.count - a.count;
      if (ai === -1) return 1;
      if (bi === -1) return -1;
      return ai - bi;
    })
    .filter((it) => it.count > 0);
}

type QualityDimension = "dimension" | "tier";

function QualityMiniChart({
  title,
  data,
  onBarClick,
}: {
  title: string;
  data: { label: string; count: number }[];
  onBarClick?: (label: string) => void;
}) {
  if (data.length === 0) return null;
  return (
    <div>
      <div
        style={{
          fontSize: 11,
          fontWeight: 600,
          color: "var(--color-text-muted)",
          textTransform: "uppercase",
          letterSpacing: "0.07em",
          marginBottom: 8,
        }}
      >
        {title}
      </div>
      <ResponsiveContainer width="100%" height={data.length * 28 + 8}>
        <BarChart
          data={data}
          layout="vertical"
          margin={{ top: 0, right: 40, left: 0, bottom: 0 }}
          style={onBarClick ? { cursor: "pointer" } : undefined}
        >
          <XAxis type="number" hide />
          <YAxis
            type="category"
            dataKey="label"
            tick={axisStyle}
            axisLine={false}
            tickLine={false}
            width={84}
          />
          <Tooltip
            contentStyle={tooltipStyle.contentStyle}
            wrapperStyle={tooltipStyle.wrapperStyle}
            cursor={tooltipStyle.cursor}
            formatter={(v: number | undefined) => [(v ?? 0).toLocaleString(), "Series"]}
          />
          <Bar
            dataKey="count"
            fill="var(--color-accent)"
            radius={[0, 4, 4, 0]}
            // eslint-disable-next-line @typescript-eslint/no-explicit-any
            onClick={onBarClick ? (entry: any) => {
              const label = entry?.payload?.label;
              if (label) onBarClick(String(label));
            } : undefined}
          >
            {data.map((_, i) => (
              <Cell
                key={i}
                fill="var(--color-accent)"
                fillOpacity={1 - i * (0.5 / Math.max(data.length - 1, 1))}
              />
            ))}
          </Bar>
        </BarChart>
      </ResponsiveContainer>
    </div>
  );
}

function QualityCard({ data, tierData }: { data: QualityBucket[]; tierData?: QualityTier[] }) {
  const navigate = useNavigate();
  const [view, setView] = useState<QualityDimension>("dimension");

  const total = data.reduce((s, b) => s + b.count, 0);
  if (total === 0) {
    return (
      <Card title="Quality Distribution">
        <p style={{ color: "var(--color-text-muted)", fontSize: 13, margin: 0 }}>
          No episode files yet.
        </p>
      </Card>
    );
  }

  const resolutions = sortedGroup(data, "resolution", RESOLUTION_ORDER);
  const sources = sortedGroup(data, "source", SOURCE_ORDER);
  const codecs = sortedGroup(data, "codec", CODEC_ORDER);
  const hdrs = sortedGroup(data, "hdr", HDR_ORDER);

  const tiers = (tierData ?? [])
    .filter((t) => t.count > 0)
    .sort((a, b) => {
      const ra = RESOLUTION_ORDER.indexOf(a.resolution);
      const rb = RESOLUTION_ORDER.indexOf(b.resolution);
      if (ra !== rb) {
        if (ra === -1) return 1;
        if (rb === -1) return -1;
        return ra - rb;
      }
      const sa = SOURCE_ORDER.indexOf(a.source);
      const sb = SOURCE_ORDER.indexOf(b.source);
      if (sa === -1 && sb === -1) return b.count - a.count;
      if (sa === -1) return 1;
      if (sb === -1) return -1;
      return sa - sb;
    })
    .map((t) => ({ label: `${t.resolution} ${t.source}`, count: t.count, resolution: t.resolution, source: t.source }));

  function handleTierClick(label: string) {
    const tier = tiers.find((t) => t.label === label);
    if (!tier) return;
    const params = new URLSearchParams();
    params.set("quality_resolution", tier.resolution);
    params.set("quality_source", tier.source);
    navigate(`/?${params.toString()}`);
  }

  const toggleStyle = (active: boolean): React.CSSProperties => ({
    background: active ? "var(--color-bg-elevated)" : "transparent",
    border: active ? "1px solid var(--color-border-default)" : "1px solid transparent",
    borderRadius: 5,
    padding: "3px 10px",
    fontSize: 11,
    fontWeight: active ? 600 : 400,
    color: active ? "var(--color-text-primary)" : "var(--color-text-muted)",
    cursor: "pointer",
  });

  return (
    <Card title="Quality Distribution">
      <div style={{ display: "flex", gap: 4, marginBottom: 20 }}>
        <button style={toggleStyle(view === "dimension")} onClick={() => setView("dimension")}>
          By Dimension
        </button>
        <button style={toggleStyle(view === "tier")} onClick={() => setView("tier")}>
          By Tier
        </button>
      </div>

      {view === "dimension" ? (
        <div
          style={{
            display: "grid",
            gridTemplateColumns: "repeat(auto-fill, minmax(220px, 1fr))",
            gap: 24,
          }}
        >
          <QualityMiniChart title="Resolution" data={resolutions} />
          <QualityMiniChart title="Source" data={sources} />
          <QualityMiniChart title="Codec" data={codecs} />
          <QualityMiniChart title="HDR" data={hdrs} />
        </div>
      ) : (
        <div>
          <p style={{ margin: "0 0 10px", fontSize: 11, color: "var(--color-text-muted)" }}>
            Click a tier to filter the series library.
          </p>
          <QualityMiniChart
            title="Resolution + Source"
            data={tiers}
            onBarClick={handleTierClick}
          />
        </div>
      )}
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
  const quality = useQualityStats();
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
        {quality.isLoading ? (
          <CardSkeleton height={260} />
        ) : quality.error ? (
          <ErrorCard title="Quality Distribution" />
        ) : quality.data ? (
          <QualityCard data={quality.data} tierData={qualityTiers.data} />
        ) : null}
      </div>
    </div>
  );
}
