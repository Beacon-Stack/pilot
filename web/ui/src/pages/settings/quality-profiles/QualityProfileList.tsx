import { useState } from "react";
import { Plus, Trash2, Pencil, X, ChevronDown, ChevronRight } from "lucide-react";
import { toast } from "sonner";
import { useQualityProfiles, useDeleteQualityProfile } from "@/api/quality";
import PageHeader from "@/components/PageHeader";
import Modal from "@/components/Modal";
import type { QualityProfile } from "@/types";

// ── Quality profile detail modal ──────────────────────────────────────────────

function ProfileDetailModal({ profile, onClose }: { profile: QualityProfile; onClose: () => void }) {
  return (
    <Modal onClose={onClose} width={520}>
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          padding: "18px 20px",
          borderBottom: "1px solid var(--color-border-subtle)",
        }}
      >
        <h2 style={{ margin: 0, fontSize: 16, fontWeight: 600, color: "var(--color-text-primary)" }}>
          {profile.name}
        </h2>
        <button
          onClick={onClose}
          style={{
            background: "none",
            border: "none",
            cursor: "pointer",
            color: "var(--color-text-muted)",
            display: "flex",
            padding: 4,
          }}
        >
          <X size={18} />
        </button>
      </div>

      <div style={{ padding: 20, overflowY: "auto" }}>
        <div style={{ display: "flex", gap: 24, marginBottom: 20 }}>
          <div>
            <div style={{ fontSize: 11, fontWeight: 600, color: "var(--color-text-muted)", textTransform: "uppercase", letterSpacing: "0.06em", marginBottom: 4 }}>
              Cutoff
            </div>
            <div style={{ fontSize: 14, color: "var(--color-text-primary)" }}>
              {profile.cutoff.name}
            </div>
          </div>
          <div>
            <div style={{ fontSize: 11, fontWeight: 600, color: "var(--color-text-muted)", textTransform: "uppercase", letterSpacing: "0.06em", marginBottom: 4 }}>
              Upgrades
            </div>
            <div style={{ fontSize: 14, color: "var(--color-text-primary)" }}>
              {profile.upgrade_allowed ? "Allowed" : "Not allowed"}
            </div>
          </div>
        </div>

        <div
          style={{
            fontSize: 11,
            fontWeight: 600,
            color: "var(--color-text-muted)",
            textTransform: "uppercase",
            letterSpacing: "0.06em",
            marginBottom: 10,
          }}
        >
          Qualities ({profile.qualities.length})
        </div>
        <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
          {profile.qualities.map((q, idx) => (
            <div
              key={idx}
              style={{
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
                padding: "8px 12px",
                background: "var(--color-bg-elevated)",
                borderRadius: 6,
                fontSize: 13,
              }}
            >
              <span style={{ fontWeight: 500, color: "var(--color-text-primary)" }}>{q.name}</span>
              <span style={{ fontSize: 12, color: "var(--color-text-muted)" }}>
                {q.resolution} · {q.source}
              </span>
            </div>
          ))}
        </div>
      </div>
    </Modal>
  );
}

// ── Profile card ──────────────────────────────────────────────────────────────

function ProfileCard({
  profile,
  onEdit,
  onDelete,
}: {
  profile: QualityProfile;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const Chevron = expanded ? ChevronDown : ChevronRight;

  return (
    <div
      style={{
        background: "var(--color-bg-surface)",
        border: "1px solid var(--color-border-subtle)",
        borderRadius: 8,
        boxShadow: "var(--shadow-card)",
        overflow: "hidden",
      }}
    >
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          padding: "14px 16px",
        }}
      >
        <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
          <button
            onClick={() => setExpanded((x) => !x)}
            style={{
              background: "none",
              border: "none",
              cursor: "pointer",
              color: "var(--color-text-muted)",
              display: "flex",
              padding: 0,
            }}
          >
            <Chevron size={16} strokeWidth={1.5} />
          </button>
          <div>
            <div style={{ fontSize: 14, fontWeight: 600, color: "var(--color-text-primary)" }}>
              {profile.name}
            </div>
            <div style={{ fontSize: 12, color: "var(--color-text-muted)", marginTop: 2 }}>
              {profile.qualities.length} qualities · cutoff: {profile.cutoff.name}
              {profile.upgrade_allowed && " · upgrades on"}
            </div>
          </div>
        </div>
        <div style={{ display: "flex", gap: 8 }}>
          <button
            onClick={onEdit}
            style={{
              display: "flex",
              alignItems: "center",
              padding: "6px 10px",
              background: "none",
              border: "1px solid var(--color-border-default)",
              borderRadius: 6,
              cursor: "pointer",
              color: "var(--color-text-secondary)",
            }}
            title="Edit"
          >
            <Pencil size={14} strokeWidth={1.5} />
          </button>
          <button
            onClick={onDelete}
            style={{
              display: "flex",
              alignItems: "center",
              padding: "6px 10px",
              background: "none",
              border: "1px solid var(--color-border-default)",
              borderRadius: 6,
              cursor: "pointer",
              color: "var(--color-danger)",
            }}
            title="Delete"
          >
            <Trash2 size={14} strokeWidth={1.5} />
          </button>
        </div>
      </div>

      {expanded && (
        <div
          style={{
            padding: "0 16px 14px",
            borderTop: "1px solid var(--color-border-subtle)",
          }}
        >
          <div
            style={{
              display: "flex",
              flexDirection: "column",
              gap: 4,
              paddingTop: 12,
            }}
          >
            {profile.qualities.map((q, idx) => (
              <div
                key={idx}
                style={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  padding: "6px 10px",
                  background: "var(--color-bg-elevated)",
                  borderRadius: 6,
                  fontSize: 13,
                }}
              >
                <span style={{ fontWeight: 500, color: "var(--color-text-primary)" }}>{q.name}</span>
                <span style={{ fontSize: 12, color: "var(--color-text-muted)" }}>
                  {q.resolution} · {q.source} · {q.codec}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

// ── Main page ─────────────────────────────────────────────────────────────────

export default function QualityProfileList() {
  const { data: profiles, isLoading } = useQualityProfiles();
  const deleteProfile = useDeleteQualityProfile();
  const [viewing, setViewing] = useState<QualityProfile | null>(null);

  function handleDelete(profile: QualityProfile) {
    if (!confirm(`Delete quality profile "${profile.name}"?`)) return;
    deleteProfile.mutate(profile.id, {
      onSuccess: () => toast.success("Profile deleted"),
      onError: (err) => toast.error(err.message),
    });
  }

  return (
    <>
      <PageHeader
        title="Quality Profiles"
        description="Define which quality tiers Pilot should download and when to upgrade."
        action={
          <button
            onClick={() => toast.info("Quality profile editor coming soon")}
            style={{
              display: "flex",
              alignItems: "center",
              gap: 6,
              padding: "8px 14px",
              background: "var(--color-accent)",
              border: "none",
              borderRadius: 6,
              cursor: "pointer",
              fontSize: 13,
              fontWeight: 600,
              color: "var(--color-accent-fg)",
            }}
          >
            <Plus size={15} strokeWidth={2.5} />
            Add Profile
          </button>
        }
      />

      {isLoading && (
        <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
          {Array.from({ length: 3 }).map((_, i) => (
            <div key={i} className="skeleton" style={{ height: 68, borderRadius: 8 }} />
          ))}
        </div>
      )}

      {!isLoading && (!profiles || profiles.length === 0) && (
        <div
          style={{
            textAlign: "center",
            padding: "48px 24px",
            background: "var(--color-bg-surface)",
            border: "1px solid var(--color-border-subtle)",
            borderRadius: 8,
            color: "var(--color-text-muted)",
            fontSize: 14,
          }}
        >
          No quality profiles configured. Add one to define download preferences.
        </div>
      )}

      {!isLoading && profiles && profiles.length > 0 && (
        <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
          {profiles.map((profile) => (
            <ProfileCard
              key={profile.id}
              profile={profile}
              onEdit={() => setViewing(profile)}
              onDelete={() => handleDelete(profile)}
            />
          ))}
        </div>
      )}

      {viewing && (
        <ProfileDetailModal profile={viewing} onClose={() => setViewing(null)} />
      )}
    </>
  );
}
