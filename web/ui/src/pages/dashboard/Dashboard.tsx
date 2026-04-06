import { useNavigate } from "react-router-dom";
import { useSeriesList } from "@/api/series";
import { useCollectionStats, useStorageStats } from "@/api/stats";
import { Poster } from "@/components/Poster";
import { formatBytes } from "@/lib/utils";
import type { Series } from "@/types";

function SeriesCard({ series }: { series: Series }) {
  const navigate = useNavigate();
  const hasAll = series.episode_count > 0 && series.episode_file_count >= series.episode_count;

  return (
    <div
      onClick={() => navigate(`/series/${series.id}`)}
      style={{
        cursor: "pointer",
        display: "flex",
        flexDirection: "column",
        gap: 8,
      }}
    >
      <div style={{ position: "relative" }}>
        <Poster
          src={series.poster_url}
          title={series.title}
          year={series.year}
          style={{
            transition: "opacity 150ms ease",
          }}
        />
        {/* Episode count badge */}
        {series.episode_count > 0 && (
          <div
            style={{
              position: "absolute",
              bottom: 8,
              right: 8,
              background: "rgba(0,0,0,0.75)",
              backdropFilter: "blur(4px)",
              borderRadius: 4,
              padding: "2px 6px",
              fontSize: 11,
              fontWeight: 600,
              color: hasAll ? "var(--color-success)" : "var(--color-text-secondary)",
              border: "1px solid rgba(255,255,255,0.1)",
            }}
          >
            {series.episode_file_count}/{series.episode_count}
          </div>
        )}
        {/* Status indicator */}
        {!series.monitored && (
          <div
            style={{
              position: "absolute",
              top: 8,
              left: 8,
              background: "rgba(0,0,0,0.7)",
              borderRadius: 4,
              padding: "2px 6px",
              fontSize: 10,
              fontWeight: 500,
              color: "var(--color-text-muted)",
            }}
          >
            Unmonitored
          </div>
        )}
      </div>

      <div style={{ paddingBottom: 4 }}>
        <div
          style={{
            fontSize: 13,
            fontWeight: 500,
            color: "var(--color-text-primary)",
            overflow: "hidden",
            whiteSpace: "nowrap",
            textOverflow: "ellipsis",
            lineHeight: 1.3,
          }}
        >
          {series.title}
        </div>
        <div
          style={{
            fontSize: 12,
            color: "var(--color-text-muted)",
            marginTop: 2,
          }}
        >
          {series.year}
          {series.network && (
            <span style={{ marginLeft: 6, color: "var(--color-text-muted)" }}>
              · {series.network}
            </span>
          )}
        </div>
      </div>
    </div>
  );
}

function SkeletonCard() {
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
      <div
        className="skeleton"
        style={{ aspectRatio: "2/3", width: "100%", borderRadius: 8 }}
      />
      <div className="skeleton" style={{ height: 14, width: "80%", borderRadius: 4 }} />
      <div className="skeleton" style={{ height: 12, width: "50%", borderRadius: 4 }} />
    </div>
  );
}

function StatCard({
  label,
  value,
  accent,
}: {
  label: string;
  value: string | number | undefined;
  accent?: string;
}) {
  return (
    <div
      style={{
        flex: 1,
        minWidth: 120,
        background: "var(--color-bg-surface)",
        border: "1px solid var(--color-border-subtle)",
        borderRadius: 8,
        padding: "14px 18px",
      }}
    >
      <div
        style={{
          fontSize: 22,
          fontWeight: 700,
          color: accent ?? "var(--color-text-primary)",
          lineHeight: 1,
          marginBottom: 5,
        }}
      >
        {value ?? (
          <span
            className="skeleton"
            style={{ display: "inline-block", width: 48, height: 22, borderRadius: 4, verticalAlign: "middle" }}
          />
        )}
      </div>
      <div style={{ fontSize: 11, color: "var(--color-text-muted)", fontWeight: 500, textTransform: "uppercase", letterSpacing: "0.06em" }}>
        {label}
      </div>
    </div>
  );
}

export default function Dashboard() {
  const navigate = useNavigate();
  const { data, isLoading, isError } = useSeriesList();
  const { data: collStats } = useCollectionStats();
  const { data: storStats } = useStorageStats();

  return (
    <div style={{ padding: "24px 28px" }}>
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          marginBottom: 16,
        }}
      >
        <h1
          style={{
            margin: 0,
            fontSize: 20,
            fontWeight: 600,
            color: "var(--color-text-primary)",
            letterSpacing: "-0.01em",
          }}
        >
          Series
        </h1>
        <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
          {data && (
            <span style={{ fontSize: 13, color: "var(--color-text-muted)" }}>
              {data.total} series
            </span>
          )}
          <button
            onClick={() => navigate("/discover")}
            style={{
              background: "var(--color-accent)",
              color: "var(--color-accent-fg)",
              border: "none",
              borderRadius: 6,
              padding: "6px 14px",
              fontSize: 13,
              fontWeight: 500,
              cursor: "pointer",
            }}
          >
            + Add Series
          </button>
        </div>
      </div>

      {/* Stats summary row */}
      <div style={{ display: "flex", gap: 12, marginBottom: 24, flexWrap: "wrap" }}>
        <StatCard label="Total Series" value={collStats?.total_series.toLocaleString()} />
        <StatCard label="Episodes on Disk" value={collStats?.with_file.toLocaleString()} />
        <StatCard
          label="Missing"
          value={collStats?.missing.toLocaleString()}
          accent={collStats && collStats.missing > 0 ? "var(--color-warning)" : undefined}
        />
        <StatCard label="Total Size" value={storStats ? formatBytes(storStats.total_bytes) : undefined} />
      </div>

      {isError && (
        <div
          style={{
            padding: "20px",
            background: "color-mix(in srgb, var(--color-danger) 10%, transparent)",
            border: "1px solid color-mix(in srgb, var(--color-danger) 30%, transparent)",
            borderRadius: 8,
            color: "var(--color-danger)",
            fontSize: 14,
          }}
        >
          Failed to load series. Check that the backend is running.
        </div>
      )}

      {!isError && (
        <div
          style={{
            display: "grid",
            gridTemplateColumns: "repeat(auto-fill, minmax(148px, 1fr))",
            gap: 16,
          }}
        >
          {isLoading
            ? Array.from({ length: 20 }).map((_, i) => <SkeletonCard key={i} />)
            : (data?.series ?? []).map((s) => <SeriesCard key={s.id} series={s} />)}
        </div>
      )}

      {!isLoading && !isError && data?.series.length === 0 && (
        <div
          style={{
            textAlign: "center",
            padding: "80px 24px",
            color: "var(--color-text-muted)",
          }}
        >
          <div style={{ fontSize: 48, marginBottom: 16, opacity: 0.3 }}>📺</div>
          <p style={{ margin: 0, fontSize: 15, fontWeight: 500, color: "var(--color-text-secondary)" }}>
            No series yet
          </p>
          <p style={{ margin: "8px 0 0", fontSize: 13 }}>
            Add a series to get started.
          </p>
        </div>
      )}
    </div>
  );
}
