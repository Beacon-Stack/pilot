import { useState } from "react";
import { Plus, Trash2, Pencil, ChevronDown, ChevronRight } from "lucide-react";
import { toast } from "sonner";
import { useConfirm } from "@beacon-shared/ConfirmDialog";
import { useQualityProfiles, useDeleteQualityProfile } from "@/api/quality";
import PageHeader from "@/components/PageHeader";
import QualityProfileEditor from "./QualityProfileEditor";
import type { QualityProfile } from "@/types";

// Profile card

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
            <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
              <div style={{ fontSize: 14, fontWeight: 600, color: "var(--color-text-primary)" }}>
                {profile.name}
              </div>
              {profile.managed_by_pulse && (
                <span
                  title="This profile is managed by Pulse. Editing disconnects it."
                  style={{
                    fontSize: 10,
                    fontWeight: 600,
                    letterSpacing: "0.05em",
                    textTransform: "uppercase",
                    padding: "2px 6px",
                    borderRadius: 3,
                    background: "color-mix(in srgb, var(--color-accent) 15%, transparent)",
                    color: "var(--color-accent)",
                    border: "1px solid color-mix(in srgb, var(--color-accent) 30%, transparent)",
                  }}
                >
                  Pulse
                </span>
              )}
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
            disabled={profile.managed_by_pulse}
            style={{
              display: "flex",
              alignItems: "center",
              padding: "6px 10px",
              background: "none",
              border: "1px solid var(--color-border-default)",
              borderRadius: 6,
              cursor: profile.managed_by_pulse ? "not-allowed" : "pointer",
              color: "var(--color-danger)",
              opacity: profile.managed_by_pulse ? 0.4 : 1,
            }}
            title={profile.managed_by_pulse ? "Managed by Pulse — delete via Pulse instead" : "Delete"}
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

// Main page

export default function QualityProfileList() {
  const { data: profiles, isLoading } = useQualityProfiles();
  const deleteProfile = useDeleteQualityProfile();
  // undefined = closed, null = new profile
  const [editing, setEditing] = useState<QualityProfile | null | undefined>(undefined);

  const confirm = useConfirm();

  async function handleDelete(profile: QualityProfile) {
    if (!await confirm({ title: "Delete Quality Profile", message: `Delete quality profile "${profile.name}"?` })) return;
    deleteProfile.mutate(profile.id, {
      onSuccess: () => toast.success("Profile deleted"),
      onError: (err) => toast.error(err.message),
    });
  }

  return (
    <div style={{ padding: 24, maxWidth: 900 }}>
      <PageHeader
        title="Quality Profiles"
        description="Define which quality tiers Pilot should download and when to upgrade."
        action={
          <button
            onClick={() => setEditing(null)}
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
              onEdit={() => setEditing(profile)}
              onDelete={() => handleDelete(profile)}
            />
          ))}
        </div>
      )}

      {editing !== undefined && (
        <QualityProfileEditor profile={editing} onClose={() => setEditing(undefined)} />
      )}
    </div>
  );
}
