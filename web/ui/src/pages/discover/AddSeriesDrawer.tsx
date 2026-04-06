import { useState } from "react";
import { X, Loader2 } from "lucide-react";
import Drawer from "@/components/Drawer";
import { useAddSeries, tmdbPosterURL } from "@/api/series";
import { useLibraries } from "@/api/libraries";
import { useQualityProfiles } from "@/api/quality";
import { toast } from "sonner";
import type { LookupResult } from "@/api/series";

const MONITOR_TYPES = [
  { value: "all", label: "All Episodes" },
  { value: "future", label: "Future Only" },
  { value: "missing", label: "Missing Only" },
  { value: "none", label: "None" },
  { value: "pilot", label: "Pilot Only" },
  { value: "first_season", label: "First Season" },
  { value: "last_season", label: "Latest Season" },
];

const SERIES_TYPES = [
  { value: "standard", label: "Standard" },
  { value: "daily", label: "Daily" },
  { value: "anime", label: "Anime" },
];

interface Props {
  result: LookupResult;
  isAdded: boolean;
  onClose: () => void;
}

export default function AddSeriesDrawer({ result, isAdded, onClose }: Props) {
  const { data: libraries } = useLibraries();
  const { data: profiles } = useQualityProfiles();
  const addSeries = useAddSeries();

  const [libraryId, setLibraryId] = useState("");
  const [profileId, setProfileId] = useState("");
  const [monitorType, setMonitorType] = useState("all");
  const [seriesType, setSeriesType] = useState("standard");
  const [monitored, setMonitored] = useState(true);

  // Set defaults when data loads
  if (libraries?.length && !libraryId) setLibraryId(libraries[0].id);
  if (profiles?.length && !profileId) setProfileId(profiles[0].id);

  const posterSrc = tmdbPosterURL(result.poster_path, "w500");

  const handleAdd = () => {
    if (!libraryId) { toast.error("Select a library first"); return; }
    if (!profileId) { toast.error("Select a quality profile first"); return; }

    addSeries.mutate(
      {
        tmdb_id: result.tmdb_id,
        library_id: libraryId,
        quality_profile_id: profileId,
        monitored,
        monitor_type: monitorType,
        series_type: seriesType,
      },
      {
        onSuccess: () => {
          toast.success(`${result.title} added to library`);
          onClose();
        },
        onError: (err) => {
          const msg = (err as Error).message;
          if (msg.includes("409") || msg.toLowerCase().includes("already")) {
            toast.error("This series is already in your library");
          } else {
            toast.error(msg);
          }
        },
      }
    );
  };

  const inputStyle: React.CSSProperties = {
    width: "100%", padding: "8px 12px", borderRadius: 6,
    border: "1px solid var(--color-border-default)",
    background: "var(--color-bg-elevated)", color: "var(--color-text-primary)",
    fontSize: 13, outline: "none",
  };

  const labelStyle: React.CSSProperties = {
    display: "block", fontSize: 12, fontWeight: 500,
    color: "var(--color-text-secondary)", marginBottom: 4,
  };

  return (
    <Drawer onClose={onClose} width={420}>
      {/* Header */}
      <div style={{ padding: "16px 20px", borderBottom: "1px solid var(--color-border-subtle)", display: "flex", alignItems: "center", gap: 12 }}>
        <h2 style={{ margin: 0, fontSize: 16, fontWeight: 600, color: "var(--color-text-primary)", flex: 1 }}>{result.title}</h2>
        <button onClick={onClose} style={{ background: "none", border: "none", cursor: "pointer", color: "var(--color-text-muted)", padding: 4 }}>
          <X size={18} />
        </button>
      </div>

      {/* Content */}
      <div style={{ flex: 1, overflowY: "auto", padding: 20 }}>
        {/* Poster + meta */}
        <div style={{ display: "flex", gap: 16, marginBottom: 20 }}>
          {posterSrc && (
            <img src={posterSrc} alt={result.title} style={{ width: 120, borderRadius: 6, flexShrink: 0 }} />
          )}
          <div>
            {result.year > 0 && (
              <div style={{ fontSize: 14, fontWeight: 500, color: "var(--color-text-primary)" }}>{result.year}</div>
            )}
            {result.popularity > 0 && (
              <div style={{ fontSize: 12, color: "var(--color-text-muted)", marginTop: 4 }}>
                Popularity: {result.popularity.toFixed(0)}
              </div>
            )}
          </div>
        </div>

        {/* Overview */}
        {result.overview && (
          <p style={{ margin: "0 0 20px", fontSize: 13, color: "var(--color-text-secondary)", lineHeight: 1.5 }}>
            {result.overview}
          </p>
        )}

        {/* Already added notice */}
        {isAdded && (
          <div style={{
            padding: 12, borderRadius: 6, marginBottom: 20,
            background: "color-mix(in srgb, var(--color-success) 8%, transparent)",
            border: "1px solid color-mix(in srgb, var(--color-success) 20%, transparent)",
            fontSize: 13, color: "var(--color-success)", fontWeight: 500,
          }}>
            This series is already in your library
          </div>
        )}

        {/* Add form */}
        {!isAdded && (
          <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
            <div style={{ fontSize: 11, fontWeight: 600, color: "var(--color-text-muted)", textTransform: "uppercase", letterSpacing: "0.06em" }}>
              Add to Library
            </div>

            {/* Library */}
            <div>
              <label style={labelStyle}>Library</label>
              {!libraries?.length ? (
                <div style={{ fontSize: 12, color: "var(--color-warning)" }}>
                  No libraries configured. <a href="/settings/libraries" style={{ color: "var(--color-accent)" }}>Create one first</a>
                </div>
              ) : (
                <select style={inputStyle} value={libraryId} onChange={(e) => setLibraryId(e.target.value)}>
                  {libraries.map((lib) => (
                    <option key={lib.id} value={lib.id}>{lib.name}</option>
                  ))}
                </select>
              )}
            </div>

            {/* Quality Profile */}
            <div>
              <label style={labelStyle}>Quality Profile</label>
              {!profiles?.length ? (
                <div style={{ fontSize: 12, color: "var(--color-warning)" }}>
                  No quality profiles configured. <a href="/settings/quality-profiles" style={{ color: "var(--color-accent)" }}>Create one first</a>
                </div>
              ) : (
                <select style={inputStyle} value={profileId} onChange={(e) => setProfileId(e.target.value)}>
                  {profiles.map((p) => (
                    <option key={p.id} value={p.id}>{p.name}</option>
                  ))}
                </select>
              )}
            </div>

            {/* Monitor Type */}
            <div>
              <label style={labelStyle}>Monitor</label>
              <select style={inputStyle} value={monitorType} onChange={(e) => setMonitorType(e.target.value)}>
                {MONITOR_TYPES.map((m) => (
                  <option key={m.value} value={m.value}>{m.label}</option>
                ))}
              </select>
            </div>

            {/* Series Type */}
            <div>
              <label style={labelStyle}>Series Type</label>
              <select style={inputStyle} value={seriesType} onChange={(e) => setSeriesType(e.target.value)}>
                {SERIES_TYPES.map((t) => (
                  <option key={t.value} value={t.value}>{t.label}</option>
                ))}
              </select>
            </div>

            {/* Monitored */}
            <label style={{ display: "flex", alignItems: "center", gap: 8, cursor: "pointer", fontSize: 13, color: "var(--color-text-secondary)" }}>
              <input type="checkbox" checked={monitored} onChange={(e) => setMonitored(e.target.checked)} style={{ accentColor: "var(--color-accent)" }} />
              Start monitoring immediately
            </label>
          </div>
        )}
      </div>

      {/* Footer */}
      {!isAdded && (
        <div style={{ padding: "12px 20px", borderTop: "1px solid var(--color-border-subtle)", display: "flex", justifyContent: "flex-end" }}>
          <button
            onClick={handleAdd}
            disabled={addSeries.isPending || !libraryId || !profileId}
            style={{
              padding: "8px 20px", borderRadius: 6, border: "none",
              background: "var(--color-accent)", color: "var(--color-accent-fg)",
              fontSize: 13, fontWeight: 500, cursor: "pointer",
              opacity: !libraryId || !profileId ? 0.5 : 1,
              display: "flex", alignItems: "center", gap: 6,
            }}
          >
            {addSeries.isPending ? <><Loader2 size={14} style={{ animation: "spin 1s linear infinite" }} /> Adding...</> : "Add Series"}
          </button>
        </div>
      )}
    </Drawer>
  );
}
