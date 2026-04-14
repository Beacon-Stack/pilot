import { useState, useEffect } from "react";
import { Link } from "react-router-dom";
import { Plus, Trash2, Pencil, X, FolderSync } from "lucide-react";
import { toast } from "sonner";
import { useConfirm } from "@beacon-shared/ConfirmDialog";
import { useLibraries, useCreateLibrary, useUpdateLibrary, useDeleteLibrary } from "@/api/libraries";
import { useQualityProfiles } from "@/api/quality";
import { useLibraryScan } from "@/api/episode-files";
import PageHeader from "@/components/PageHeader";
import Modal from "@beacon-shared/Modal";
import type { Library } from "@/types";

// ── Library form modal ────────────────────────────────────────────────────────

interface LibraryFormProps {
  initial?: Library;
  onClose: () => void;
}

function LibraryForm({ initial, onClose }: LibraryFormProps) {
  const { data: profiles } = useQualityProfiles();
  const [name, setName] = useState(initial?.name ?? "");
  const [rootPath, setRootPath] = useState(initial?.root_path ?? "");
  const [minFreeSpace, setMinFreeSpace] = useState(initial?.min_free_space_gb ?? 1);
  const [qualityProfileId, setQualityProfileId] = useState(
    initial?.default_quality_profile_id ?? "",
  );

  // Once profiles load, seed the dropdown to the first available profile so
  // the state matches the option the browser is visually displaying. Without
  // this, the initial state is "" while the <select> shows the first <option>,
  // which means submitting without touching the dropdown fails validation.
  useEffect(() => {
    if (qualityProfileId) return;
    const first = profiles?.[0]?.id;
    if (first) setQualityProfileId(first);
  }, [profiles, qualityProfileId]);

  const create = useCreateLibrary();
  const update = useUpdateLibrary(initial?.id ?? "");

  const isEdit = !!initial;
  const hasProfiles = (profiles?.length ?? 0) > 0;

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!qualityProfileId) {
      toast.error("Select a quality profile");
      return;
    }
    const body = {
      name,
      root_path: rootPath,
      min_free_space_gb: minFreeSpace,
      tags: [],
      default_quality_profile_id: qualityProfileId,
    };

    if (isEdit) {
      update.mutate(body, {
        onSuccess: () => { toast.success("Library updated"); onClose(); },
        onError: (err) => toast.error(err.message),
      });
    } else {
      create.mutate(body, {
        onSuccess: () => { toast.success("Library created"); onClose(); },
        onError: (err) => toast.error(err.message),
      });
    }
  }

  const isPending = create.isPending || update.isPending;

  return (
    <Modal onClose={onClose} width={480}>
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
          {isEdit ? "Edit Library" : "Add Library"}
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

      <form onSubmit={handleSubmit} style={{ padding: 20, display: "flex", flexDirection: "column", gap: 16 }}>
        <div>
          <label
            style={{
              display: "block",
              fontSize: 12,
              fontWeight: 500,
              color: "var(--color-text-secondary)",
              marginBottom: 6,
            }}
          >
            Name
          </label>
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            placeholder="TV Shows"
            style={{
              width: "100%",
              padding: "8px 12px",
              background: "var(--color-bg-elevated)",
              border: "1px solid var(--color-border-default)",
              borderRadius: 6,
              fontSize: 14,
              color: "var(--color-text-primary)",
              outline: "none",
            }}
          />
        </div>

        <div>
          <label
            style={{
              display: "block",
              fontSize: 12,
              fontWeight: 500,
              color: "var(--color-text-secondary)",
              marginBottom: 6,
            }}
          >
            Root Path
          </label>
          <input
            value={rootPath}
            onChange={(e) => setRootPath(e.target.value)}
            required
            placeholder="/mnt/media/tv"
            style={{
              width: "100%",
              padding: "8px 12px",
              background: "var(--color-bg-elevated)",
              border: "1px solid var(--color-border-default)",
              borderRadius: 6,
              fontSize: 14,
              color: "var(--color-text-primary)",
              fontFamily: "var(--font-family-mono)",
              outline: "none",
            }}
          />
        </div>

        <div>
          <label
            style={{
              display: "block",
              fontSize: 12,
              fontWeight: 500,
              color: "var(--color-text-secondary)",
              marginBottom: 6,
            }}
          >
            Default Quality Profile
          </label>
          {hasProfiles ? (
            <select
              value={qualityProfileId}
              onChange={(e) => setQualityProfileId(e.target.value)}
              required
              style={{
                width: "100%",
                padding: "8px 12px",
                background: "var(--color-bg-elevated)",
                border: "1px solid var(--color-border-default)",
                borderRadius: 6,
                fontSize: 14,
                color: "var(--color-text-primary)",
                outline: "none",
              }}
            >
              {profiles!.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
          ) : (
            <div
              style={{
                padding: "10px 12px",
                background: "color-mix(in srgb, var(--color-warning) 10%, var(--color-bg-elevated))",
                border: "1px solid color-mix(in srgb, var(--color-warning) 30%, transparent)",
                borderRadius: 6,
                fontSize: 12,
                color: "var(--color-text-secondary)",
                lineHeight: 1.5,
              }}
            >
              No quality profiles exist yet. <Link to="/settings/quality-profiles" style={{ color: "var(--color-accent)", textDecoration: "underline" }}>Create one first</Link> before adding a library.
            </div>
          )}
        </div>

        <div>
          <label
            style={{
              display: "block",
              fontSize: 12,
              fontWeight: 500,
              color: "var(--color-text-secondary)",
              marginBottom: 6,
            }}
          >
            Minimum Free Space (GB)
          </label>
          <input
            type="number"
            min={0}
            value={minFreeSpace}
            onChange={(e) => setMinFreeSpace(Number(e.target.value))}
            style={{
              width: "100%",
              padding: "8px 12px",
              background: "var(--color-bg-elevated)",
              border: "1px solid var(--color-border-default)",
              borderRadius: 6,
              fontSize: 14,
              color: "var(--color-text-primary)",
              outline: "none",
            }}
          />
        </div>

        <div style={{ display: "flex", justifyContent: "flex-end", gap: 8, paddingTop: 4 }}>
          <button
            type="button"
            onClick={onClose}
            style={{
              padding: "8px 16px",
              background: "none",
              border: "1px solid var(--color-border-default)",
              borderRadius: 6,
              cursor: "pointer",
              fontSize: 13,
              color: "var(--color-text-secondary)",
            }}
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={isPending || !hasProfiles}
            style={{
              padding: "8px 16px",
              background: "var(--color-accent)",
              border: "none",
              borderRadius: 6,
              cursor: (isPending || !hasProfiles) ? "not-allowed" : "pointer",
              fontSize: 13,
              fontWeight: 600,
              color: "var(--color-accent-fg)",
              opacity: (isPending || !hasProfiles) ? 0.7 : 1,
            }}
          >
            {isPending ? "Saving..." : isEdit ? "Save Changes" : "Add Library"}
          </button>
        </div>
      </form>
    </Modal>
  );
}

// ── Main page ─────────────────────────────────────────────────────────────────

export default function LibraryList() {
  const { data: libraries, isLoading } = useLibraries();
  const deleteLibrary = useDeleteLibrary();
  const libraryScan = useLibraryScan();
  const [showForm, setShowForm] = useState(false);
  const [editing, setEditing] = useState<Library | null>(null);

  const confirm = useConfirm();

  async function handleDelete(lib: Library) {
    if (!await confirm({ title: "Delete Library", message: `Delete library "${lib.name}"? This will not delete any files.` })) return;
    deleteLibrary.mutate(lib.id, {
      onSuccess: () => toast.success("Library deleted"),
      onError: (err) => toast.error(err.message),
    });
  }

  return (
    <div style={{ padding: 24, maxWidth: 900 }}>
      <PageHeader
        title="Libraries"
        description="Define root paths where Pilot will store and manage TV series."
        action={
          <div style={{ display: "flex", gap: 8 }}>
            <button
              onClick={() => libraryScan.mutate()}
              disabled={libraryScan.isPending}
              style={{
                display: "flex",
                alignItems: "center",
                gap: 6,
                padding: "8px 14px",
                background: "none",
                border: "1px solid var(--color-border-default)",
                borderRadius: 6,
                cursor: libraryScan.isPending ? "not-allowed" : "pointer",
                fontSize: 13,
                fontWeight: 500,
                color: "var(--color-text-secondary)",
                opacity: libraryScan.isPending ? 0.7 : 1,
              }}
            >
              <FolderSync size={15} strokeWidth={1.5} style={libraryScan.isPending ? { animation: "spin 1s linear infinite" } : undefined} />
              {libraryScan.isPending ? "Scanning…" : "Scan All"}
            </button>
            <button
              onClick={() => setShowForm(true)}
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
              Add Library
            </button>
          </div>
        }
      />

      {isLoading && (
        <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
          {Array.from({ length: 2 }).map((_, i) => (
            <div key={i} className="skeleton" style={{ height: 72, borderRadius: 8 }} />
          ))}
        </div>
      )}

      {!isLoading && (!libraries || libraries.length === 0) && (
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
          No libraries configured. Add one to get started.
        </div>
      )}

      {!isLoading && libraries && libraries.length > 0 && (
        <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
          {libraries.map((lib) => (
            <div
              key={lib.id}
              style={{
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
                padding: "14px 16px",
                background: "var(--color-bg-surface)",
                border: "1px solid var(--color-border-subtle)",
                borderRadius: 8,
                boxShadow: "var(--shadow-card)",
              }}
            >
              <div>
                <div style={{ fontSize: 14, fontWeight: 600, color: "var(--color-text-primary)" }}>
                  {lib.name}
                </div>
                <div
                  style={{
                    fontSize: 12,
                    color: "var(--color-text-muted)",
                    fontFamily: "var(--font-family-mono)",
                    marginTop: 3,
                  }}
                >
                  {lib.root_path}
                </div>
              </div>
              <div style={{ display: "flex", gap: 8 }}>
                <button
                  onClick={() => setEditing(lib)}
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
                  onClick={() => handleDelete(lib)}
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
          ))}
        </div>
      )}

      {showForm && <LibraryForm onClose={() => setShowForm(false)} />}
      {editing && <LibraryForm initial={editing} onClose={() => setEditing(null)} />}
    </div>
  );
}
