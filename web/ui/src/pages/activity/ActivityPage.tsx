import { useState } from "react";
import { Link } from "react-router-dom";
import {
  Download,
  ArrowDownToLine,
  Clock,
  Tv,
  AlertCircle,
  CheckCircle,
} from "lucide-react";
import { useActivity, type ActivityEntry } from "@/api/activity";

const CATEGORIES = [
  { value: "", label: "All" },
  { value: "grab", label: "Grabs" },
  { value: "import", label: "Imports" },
  { value: "task", label: "Tasks" },
  { value: "health", label: "Health" },
  { value: "series", label: "Series" },
] as const;

function categoryIcon(category: string) {
  switch (category) {
    case "grab":
      return Download;
    case "import":
      return ArrowDownToLine;
    case "task":
      return Clock;
    case "health":
      return CheckCircle;
    case "series":
      return Tv;
    default:
      return AlertCircle;
  }
}

function categoryColor(category: string): string {
  switch (category) {
    case "grab":
      return "var(--color-accent)";
    case "import":
      return "var(--color-success)";
    case "task":
      return "var(--color-text-muted)";
    case "health":
      return "var(--color-warning)";
    case "series":
      return "var(--color-accent)";
    default:
      return "var(--color-text-muted)";
  }
}

function relativeTime(iso: string): string {
  const now = Date.now();
  const then = new Date(iso).getTime();
  const diffSec = Math.floor((now - then) / 1000);

  if (diffSec < 60) return "just now";
  if (diffSec < 3600) return `${Math.floor(diffSec / 60)}m ago`;
  if (diffSec < 86400) return `${Math.floor(diffSec / 3600)}h ago`;
  if (diffSec < 604800) return `${Math.floor(diffSec / 86400)}d ago`;
  return new Date(iso).toLocaleDateString();
}

function ActivityRow({ activity, isLast }: { activity: ActivityEntry; isLast: boolean }) {
  const Icon = categoryIcon(activity.category);
  const color = categoryColor(activity.category);

  return (
    <div
      style={{
        display: "flex",
        alignItems: "flex-start",
        gap: 12,
        padding: "12px 0",
        borderBottom: isLast ? "none" : "1px solid var(--color-border-subtle)",
      }}
    >
      <div
        style={{
          width: 32,
          height: 32,
          borderRadius: 8,
          background: `color-mix(in srgb, ${color} 12%, transparent)`,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          flexShrink: 0,
          marginTop: 2,
        }}
      >
        <Icon size={15} strokeWidth={2} style={{ color }} />
      </div>
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ fontSize: 13, color: "var(--color-text-primary)", lineHeight: 1.4 }}>
          {activity.series_id ? (
            <Link
              to={`/series/${activity.series_id}`}
              style={{ color: "inherit", textDecoration: "none" }}
              onMouseEnter={(e) => {
                (e.currentTarget as HTMLAnchorElement).style.color = "var(--color-accent)";
              }}
              onMouseLeave={(e) => {
                (e.currentTarget as HTMLAnchorElement).style.color = "inherit";
              }}
            >
              {activity.title}
            </Link>
          ) : (
            <span>{activity.title}</span>
          )}
        </div>
        {activity.detail && (
          <div style={{ fontSize: 11, color: "var(--color-text-secondary)", marginTop: 2, lineHeight: 1.4 }}>
            {activity.detail}
          </div>
        )}
        <div style={{ fontSize: 11, color: "var(--color-text-muted)", marginTop: 3 }}>
          {relativeTime(activity.created_at)}
        </div>
      </div>
    </div>
  );
}

const PER_PAGE = 50;

export default function ActivityPage() {
  const [page, setPage] = useState(1);
  const [category, setCategory] = useState("");

  const { data, isLoading, error } = useActivity(page, PER_PAGE);

  const activities = (data?.activities ?? []).filter(
    (a) => !category || a.category === category
  );
  const total = data?.total ?? 0;
  const pageCount = Math.max(1, Math.ceil(total / PER_PAGE));

  return (
    <div style={{ padding: 24, maxWidth: 800, display: "flex", flexDirection: "column", gap: 24 }}>
      {/* Header */}
      <div>
        <h1
          style={{
            fontSize: 20,
            fontWeight: 600,
            color: "var(--color-text-primary)",
            margin: 0,
            marginBottom: 4,
            letterSpacing: "-0.01em",
          }}
        >
          Activity
        </h1>
        <p style={{ fontSize: 13, color: "var(--color-text-secondary)", margin: 0 }}>
          Recent events across grabs, imports, tasks, and library changes.
        </p>
      </div>

      {/* Category filter pills */}
      <div style={{ display: "flex", gap: 6, flexWrap: "wrap" }}>
        {CATEGORIES.map((cat) => {
          const active = category === cat.value;
          return (
            <button
              key={cat.value}
              onClick={() => { setCategory(cat.value); setPage(1); }}
              style={{
                padding: "5px 12px",
                borderRadius: 6,
                border: active
                  ? "1px solid var(--color-accent)"
                  : "1px solid var(--color-border-default)",
                background: active ? "color-mix(in srgb, var(--color-accent) 12%, transparent)" : "transparent",
                color: active ? "var(--color-accent)" : "var(--color-text-secondary)",
                fontSize: 12,
                fontWeight: 500,
                cursor: "pointer",
              }}
            >
              {cat.label}
            </button>
          );
        })}
      </div>

      {/* Timeline card */}
      <div
        style={{
          background: "var(--color-bg-surface)",
          border: "1px solid var(--color-border-subtle)",
          borderRadius: 8,
          padding: "16px 20px",
        }}
      >
        <div
          style={{
            fontSize: 11,
            fontWeight: 600,
            letterSpacing: "0.08em",
            textTransform: "uppercase",
            color: "var(--color-text-muted)",
            marginBottom: 12,
            display: "flex",
            alignItems: "center",
            gap: 8,
          }}
        >
          Timeline
          {data && (
            <span style={{ fontWeight: 400, textTransform: "none", letterSpacing: 0, fontSize: 12 }}>
              {total.toLocaleString()} events
            </span>
          )}
        </div>

        {isLoading && (
          <div>
            {[1, 2, 3, 4, 5].map((i) => (
              <div
                key={i}
                className="skeleton"
                style={{ height: 48, borderRadius: 6, marginBottom: 8 }}
              />
            ))}
          </div>
        )}

        {error && (
          <p style={{ fontSize: 13, color: "var(--color-danger, #ef4444)", margin: 0 }}>
            Failed to load activity.
          </p>
        )}

        {!isLoading && !error && activities.length === 0 && (
          <p
            style={{
              fontSize: 13,
              color: "var(--color-text-muted)",
              margin: 0,
              padding: "24px 0",
              textAlign: "center",
            }}
          >
            No recent activity
          </p>
        )}

        {activities.map((a, i) => (
          <ActivityRow key={a.id} activity={a} isLast={i === activities.length - 1} />
        ))}
      </div>

      {/* Pagination */}
      {!isLoading && !error && pageCount > 1 && (
        <div style={{ display: "flex", alignItems: "center", justifyContent: "center", gap: 8 }}>
          <button
            disabled={page === 1}
            onClick={() => setPage((p) => p - 1)}
            style={{
              padding: "5px 12px",
              borderRadius: 5,
              border: "1px solid var(--color-border-default)",
              background: "var(--color-bg-elevated)",
              color: page === 1 ? "var(--color-text-muted)" : "var(--color-text-primary)",
              fontSize: 12,
              cursor: page === 1 ? "default" : "pointer",
              opacity: page === 1 ? 0.5 : 1,
            }}
          >
            Previous
          </button>
          <span style={{ fontSize: 12, color: "var(--color-text-muted)" }}>
            Page {page} of {pageCount}
          </span>
          <button
            disabled={page >= pageCount}
            onClick={() => setPage((p) => p + 1)}
            style={{
              padding: "5px 12px",
              borderRadius: 5,
              border: "1px solid var(--color-border-default)",
              background: "var(--color-bg-elevated)",
              color: page >= pageCount ? "var(--color-text-muted)" : "var(--color-text-primary)",
              fontSize: 12,
              cursor: page >= pageCount ? "default" : "pointer",
              opacity: page >= pageCount ? 0.5 : 1,
            }}
          >
            Next
          </button>
        </div>
      )}
    </div>
  );
}
