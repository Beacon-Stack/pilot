import { toast } from "sonner";
import { Trash2 } from "lucide-react";
import PageHeader from "@/components/PageHeader";
import { useImportExclusions, useDeleteImportExclusion } from "@/api/importlists";
import type { ImportExclusion } from "@/types";

export default function ImportExclusions() {
  const { data, isLoading, error } = useImportExclusions();
  const deleteMutation = useDeleteImportExclusion();

  async function handleDelete(exclusion: ImportExclusion) {
    try {
      await deleteMutation.mutateAsync(exclusion.id);
      toast.success(`Removed "${exclusion.title}" from exclusions`);
    } catch {
      toast.error("Failed to remove exclusion");
    }
  }

  return (
    <div style={{ padding: "32px", maxWidth: 900 }}>
      <PageHeader
        title="Import Exclusions"
        description="Series that will be skipped during import list syncs."
      />

      {/* Loading skeleton */}
      {isLoading && (
        <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="skeleton" style={{ height: 48, borderRadius: 6 }} />
          ))}
        </div>
      )}

      {/* Error */}
      {error && (
        <div
          style={{
            background: "var(--color-danger-muted, rgba(239,68,68,0.08))",
            border: "1px solid var(--color-danger)",
            borderRadius: 8,
            padding: "16px 20px",
            color: "var(--color-danger)",
            fontSize: 13,
          }}
        >
          Failed to load exclusions: {(error as Error).message}
        </div>
      )}

      {/* Empty state */}
      {!isLoading && !error && data && data.length === 0 && (
        <div
          style={{
            textAlign: "center",
            padding: "64px 32px",
            color: "var(--color-text-muted)",
          }}
        >
          <div style={{ fontSize: 40, marginBottom: 12 }}>—</div>
          <div style={{ fontSize: 15, fontWeight: 500, marginBottom: 4 }}>
            No exclusions
          </div>
          <div style={{ fontSize: 13 }}>
            Series excluded from import lists will appear here.
          </div>
        </div>
      )}

      {/* Table */}
      {!isLoading && !error && data && data.length > 0 && (
        <div
          style={{
            background: "var(--color-bg-surface)",
            border: "1px solid var(--color-border-subtle)",
            borderRadius: 8,
            overflow: "hidden",
          }}
        >
          {/* Header */}
          <div
            style={{
              display: "grid",
              gridTemplateColumns: "2fr 80px 100px 40px",
              gap: "0 12px",
              padding: "10px 16px",
              borderBottom: "1px solid var(--color-border-subtle)",
              fontSize: 11,
              fontWeight: 600,
              color: "var(--color-text-muted)",
              textTransform: "uppercase",
              letterSpacing: "0.06em",
            }}
          >
            <span>Title</span>
            <span>Year</span>
            <span>TMDb ID</span>
            <span />
          </div>

          {/* Rows */}
          {data.map((exclusion, idx) => (
            <div
              key={exclusion.id}
              style={{
                display: "grid",
                gridTemplateColumns: "2fr 80px 100px 40px",
                gap: "0 12px",
                padding: "12px 16px",
                borderBottom: idx < data.length - 1 ? "1px solid var(--color-border-subtle)" : "none",
                alignItems: "center",
              }}
            >
              <div
                style={{
                  fontSize: 13,
                  color: "var(--color-text-primary)",
                  overflow: "hidden",
                  textOverflow: "ellipsis",
                  whiteSpace: "nowrap",
                }}
                title={exclusion.title}
              >
                {exclusion.title}
              </div>

              <div style={{ fontSize: 13, color: "var(--color-text-secondary)" }}>
                {exclusion.year > 0 ? exclusion.year : "—"}
              </div>

              <div
                style={{
                  fontSize: 12,
                  color: "var(--color-text-muted)",
                  fontFamily: "var(--font-family-mono)",
                }}
              >
                {exclusion.tmdb_id}
              </div>

              <div style={{ display: "flex", justifyContent: "flex-end" }}>
                <button
                  onClick={() => handleDelete(exclusion)}
                  disabled={deleteMutation.isPending}
                  title="Remove exclusion"
                  style={{
                    background: "none",
                    border: "none",
                    cursor: "pointer",
                    color: "var(--color-text-muted)",
                    display: "flex",
                    alignItems: "center",
                    padding: 4,
                    borderRadius: 4,
                    transition: "color 150ms ease",
                  }}
                  onMouseEnter={(e) => {
                    (e.currentTarget as HTMLButtonElement).style.color = "var(--color-danger)";
                  }}
                  onMouseLeave={(e) => {
                    (e.currentTarget as HTMLButtonElement).style.color = "var(--color-text-muted)";
                  }}
                >
                  <Trash2 size={14} />
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
