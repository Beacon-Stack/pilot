import { toast } from "sonner";
import { Link } from "react-router-dom";
import { formatWait, useDropPendingRelease, usePendingReleases } from "@/api/delay";

function episodeCode(s: number, e: number) {
  return `S${String(s).padStart(2, "0")}E${String(e).padStart(2, "0")}`;
}

/** Releases RSS sync is holding back because of the delay profile. */
export default function PendingReleases() {
  const { data } = usePendingReleases();
  const drop = useDropPendingRelease();
  const pending = data ?? [];
  if (pending.length === 0) return null;

  return (
    <section style={{ marginTop: 24 }}>
      <h2 style={{ margin: "0 0 4px", fontSize: 14, fontWeight: 600, color: "var(--color-text-primary)" }}>
        Waiting for delay ({pending.length})
      </h2>
      <p style={{ margin: "0 0 10px", fontSize: 12, color: "var(--color-text-muted)" }}>
        Held back by the <Link to="/settings/delay-profile" style={{ color: "var(--color-accent)" }}>delay profile</Link> in
        case a better release appears. Each is grabbed once its wait is over, if it's still the best one.
      </p>
      <div style={{ background: "var(--color-bg-surface)", border: "1px solid var(--color-border-subtle)", borderRadius: 8 }}>
        {pending.map((p, idx) => (
          <div
            key={p.id}
            style={{
              display: "grid",
              gridTemplateColumns: "minmax(0, 1fr) auto auto",
              gap: 12,
              alignItems: "center",
              padding: "10px 16px",
              borderTop: idx === 0 ? "none" : "1px solid var(--color-border-subtle)",
              fontSize: 13,
            }}
          >
            <div style={{ minWidth: 0 }}>
              <div style={{ color: "var(--color-text-primary)" }}>
                <Link to={`/series/${p.series_id}`} style={{ color: "inherit", textDecoration: "none", fontWeight: 500 }}>
                  {p.series_title}
                </Link>{" "}
                <span style={{ color: "var(--color-text-secondary)" }}>{episodeCode(p.season_number, p.episode_number)}</span>
              </div>
              <div style={{ fontSize: 12, color: "var(--color-text-muted)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }} title={p.release_title}>
                {p.release_title} · {p.quality || "unknown quality"} · {p.protocol === "nzb" ? "usenet" : p.protocol}
              </div>
            </div>
            <span style={{ fontSize: 12, color: "var(--color-text-secondary)", whiteSpace: "nowrap" }}>
              Grabs {formatWait(p.eligible_at)}
            </span>
            <button
              type="button"
              onClick={() =>
                drop.mutate(p.id, {
                  onSuccess: () => toast.success("Dropped the pending release"),
                  onError: (err) => toast.error(err.message),
                })
              }
              style={{
                padding: "4px 10px",
                fontSize: 12,
                borderRadius: 6,
                border: "1px solid var(--color-border-default)",
                background: "none",
                color: "var(--color-text-secondary)",
                cursor: "pointer",
              }}
            >
              Drop
            </button>
          </div>
        ))}
      </div>
    </section>
  );
}
