import { useState, useEffect } from "react";
import { useMediaManagement, useUpdateMediaManagement } from "@/api/media-management";
import PageHeader from "@/components/PageHeader";
import { DOCS_URLS } from "@/lib/docsUrls";
import type { MediaManagement } from "@/types";

// ── Shared styles ─────────────────────────────────────────────────────────────

const inputStyle: React.CSSProperties = {
  width: "100%",
  background: "var(--color-bg-elevated)",
  border: "1px solid var(--color-border-default)",
  borderRadius: 6,
  padding: "8px 12px",
  fontSize: 13,
  color: "var(--color-text-primary)",
  outline: "none",
  boxSizing: "border-box",
  fontFamily: "var(--font-family-mono)",
};

const labelStyle: React.CSSProperties = {
  display: "block",
  fontSize: 12,
  fontWeight: 500,
  color: "var(--color-text-secondary)",
  marginBottom: 6,
};

// ── Preview helper ────────────────────────────────────────────────────────────

function previewEpisodeFormat(format: string): string {
  return format
    .replace("{Series Title}", "Breaking Bad")
    .replace("{Series CleanTitle}", "Breaking Bad")
    .replace("{Season}", "2")
    .replace("{Season:00}", "02")
    .replace("{Episode}", "5")
    .replace("{Episode:00}", "05")
    .replace("{Air-Date}", "2009-06-07")
    .replace("{Episode Title}", "Breakage")
    .replace("{Quality Full}", "HDTV-720p")
    .replace("{MediaInfo VideoCodec}", "x264");
}

function previewFolderFormat(format: string): string {
  return format
    .replace("{Series Title}", "Breaking Bad")
    .replace("{Series CleanTitle}", "Breaking Bad")
    .replace("{Series Year}", "2008")
    .replace("{Season}", "2")
    .replace("{Season:00}", "02");
}

// ── Toggle row ────────────────────────────────────────────────────────────────

interface ToggleRowProps {
  label: string;
  description: string;
  checked: boolean;
  onChange: (v: boolean) => void;
}

function ToggleRow({ label, description, checked, onChange }: ToggleRowProps) {
  return (
    <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 16 }}>
      <div>
        <div style={{ fontSize: 13, fontWeight: 500, color: "var(--color-text-primary)" }}>{label}</div>
        <div style={{ fontSize: 12, color: "var(--color-text-muted)", marginTop: 2 }}>{description}</div>
      </div>
      <button
        type="button"
        onClick={() => onChange(!checked)}
        style={{
          flexShrink: 0,
          width: 44,
          height: 24,
          borderRadius: 12,
          border: "none",
          cursor: "pointer",
          background: checked ? "var(--color-accent)" : "var(--color-border-default)",
          position: "relative",
          transition: "background 0.15s",
        }}
      >
        <span style={{
          position: "absolute",
          top: 3,
          left: checked ? 23 : 3,
          width: 18,
          height: 18,
          borderRadius: "50%",
          background: "white",
          transition: "left 0.15s",
        }} />
      </button>
    </div>
  );
}

// ── Section card ─────────────────────────────────────────────────────────────

function SectionCard({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div style={{
      background: "var(--color-bg-surface)",
      border: "1px solid var(--color-border-subtle)",
      borderRadius: 10,
      overflow: "hidden",
    }}>
      <div style={{
        padding: "14px 20px",
        borderBottom: "1px solid var(--color-border-subtle)",
        fontSize: 13,
        fontWeight: 600,
        color: "var(--color-text-primary)",
        letterSpacing: "0.01em",
      }}>
        {title}
      </div>
      <div style={{ padding: "20px", display: "flex", flexDirection: "column", gap: 20 }}>
        {children}
      </div>
    </div>
  );
}

// ── Format field ──────────────────────────────────────────────────────────────

function FormatField({
  label,
  value,
  onChange,
  onFocus,
  onBlur,
  placeholder,
  previewFn,
  dimmed,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  onFocus: (e: React.FocusEvent<HTMLInputElement>) => void;
  onBlur: (e: React.FocusEvent<HTMLInputElement>) => void;
  placeholder: string;
  previewFn: (v: string) => string;
  dimmed?: boolean;
}) {
  return (
    <div style={dimmed ? { opacity: 0.4, pointerEvents: "none" } : {}}>
      <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
        <label style={labelStyle}>{label}</label>
        <input
          style={inputStyle}
          value={value}
          onChange={(e) => onChange(e.currentTarget.value)}
          onFocus={onFocus}
          onBlur={onBlur}
          placeholder={placeholder}
        />
        <div style={{ fontSize: 11, color: "var(--color-text-muted)", fontFamily: "var(--font-family-mono)" }}>
          Preview: {previewFn(value)}
        </div>
      </div>
    </div>
  );
}

// ── Main page ─────────────────────────────────────────────────────────────────

export default function MediaManagementPage() {
  const { data, isLoading } = useMediaManagement();
  const update = useUpdateMediaManagement();

  const [form, setForm] = useState<MediaManagement | null>(null);
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    if (data && !dirty) {
      setForm(data);
    }
  }, [data, dirty]);

  function set<K extends keyof MediaManagement>(key: K, value: MediaManagement[K]) {
    setForm((f) => f ? { ...f, [key]: value } : f);
    setDirty(true);
  }

  function handleSave() {
    if (!form) return;
    update.mutate(form, {
      onSuccess: () => setDirty(false),
    });
  }

  function onInputFocus(e: React.FocusEvent<HTMLInputElement | HTMLSelectElement>) {
    (e.currentTarget as HTMLElement).style.borderColor = "var(--color-accent)";
  }
  function onInputBlur(e: React.FocusEvent<HTMLInputElement | HTMLSelectElement>) {
    (e.currentTarget as HTMLElement).style.borderColor = "var(--color-border-default)";
  }

  if (isLoading) {
    return (
      <div style={{ padding: "24px 32px", display: "flex", flexDirection: "column", gap: 16 }}>
        {[1, 2, 3].map((i) => (
          <div key={i} className="skeleton" style={{ height: 120, borderRadius: 10 }} />
        ))}
      </div>
    );
  }

  if (!form) return null;

  const dimmed = !form.rename_episodes;

  return (
    <div style={{ padding: "24px 32px", display: "flex", flexDirection: "column", gap: 20, maxWidth: 720 }}>
      <PageHeader
        title="Media Management"
        description="Control how episodes are named and organized on disk."
        docsUrl={DOCS_URLS.mediaManagement}
        action={
          <button
            type="button"
            onClick={handleSave}
            disabled={!dirty || update.isPending}
            style={{
              padding: "8px 18px",
              background: dirty ? "var(--color-accent)" : "var(--color-bg-elevated)",
              color: dirty ? "white" : "var(--color-text-muted)",
              border: "none",
              borderRadius: 6,
              fontSize: 13,
              fontWeight: 500,
              cursor: dirty ? "pointer" : "default",
              transition: "all 0.15s",
            }}
          >
            {update.isPending ? "Saving…" : "Save Changes"}
          </button>
        }
      />

      {/* Episode Naming */}
      <SectionCard title="Episode Naming">
        <ToggleRow
          label="Rename Episodes"
          description="Rename imported episode files using the format templates below"
          checked={form.rename_episodes}
          onChange={(v) => set("rename_episodes", v)}
        />

        <FormatField
          label="Standard Episode Format"
          value={form.standard_episode_format}
          onChange={(v) => set("standard_episode_format", v)}
          onFocus={onInputFocus}
          onBlur={onInputBlur}
          placeholder="{Series Title} - S{Season:00}E{Episode:00} - {Episode Title} {Quality Full}"
          previewFn={previewEpisodeFormat}
          dimmed={dimmed}
        />

        <FormatField
          label="Daily Episode Format"
          value={form.daily_episode_format}
          onChange={(v) => set("daily_episode_format", v)}
          onFocus={onInputFocus}
          onBlur={onInputBlur}
          placeholder="{Series Title} - {Air-Date} - {Episode Title} {Quality Full}"
          previewFn={previewEpisodeFormat}
          dimmed={dimmed}
        />

        <FormatField
          label="Anime Episode Format"
          value={form.anime_episode_format}
          onChange={(v) => set("anime_episode_format", v)}
          onFocus={onInputFocus}
          onBlur={onInputBlur}
          placeholder="{Series Title} - S{Season:00}E{Episode:00} - {Episode Title} {Quality Full}"
          previewFn={previewEpisodeFormat}
          dimmed={dimmed}
        />

        <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
          <label style={labelStyle}>Colon Replacement</label>
          <select
            style={{ ...inputStyle, fontFamily: "inherit" }}
            value={form.colon_replacement}
            onChange={(e) => set("colon_replacement", e.currentTarget.value as MediaManagement["colon_replacement"])}
            onFocus={onInputFocus}
            onBlur={onInputBlur}
          >
            <option value="delete">Delete — "Show: Title" → "Show Title"</option>
            <option value="dash">Dash — "Show: Title" → "Show- Title"</option>
            <option value="space-dash">Space Dash — "Show: Title" → "Show - Title"</option>
            <option value="smart">Smart — context-aware space dash</option>
          </select>
        </div>
      </SectionCard>

      {/* Folder Naming */}
      <SectionCard title="Folder Naming">
        <FormatField
          label="Series Folder Format"
          value={form.series_folder_format}
          onChange={(v) => set("series_folder_format", v)}
          onFocus={onInputFocus}
          onBlur={onInputBlur}
          placeholder="{Series Title} ({Series Year})"
          previewFn={previewFolderFormat}
        />

        <FormatField
          label="Season Folder Format"
          value={form.season_folder_format}
          onChange={(v) => set("season_folder_format", v)}
          onFocus={onInputFocus}
          onBlur={onInputBlur}
          placeholder="Season {Season:00}"
          previewFn={previewFolderFormat}
        />
      </SectionCard>

      {/* Importing */}
      <SectionCard title="Importing">
        <ToggleRow
          label="Import Extra Files"
          description="Copy subtitle and metadata files alongside the video when importing"
          checked={form.import_extra_files}
          onChange={(v) => set("import_extra_files", v)}
        />

        <div style={!form.import_extra_files ? { opacity: 0.4, pointerEvents: "none" } : {}}>
          <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
            <label style={labelStyle}>Extra File Extensions</label>
            <input
              style={inputStyle}
              value={form.extra_file_extensions}
              onChange={(e) => set("extra_file_extensions", e.currentTarget.value)}
              onFocus={onInputFocus}
              onBlur={onInputBlur}
              placeholder="srt,nfo,sub,idx"
            />
            <div style={{ fontSize: 11, color: "var(--color-text-muted)" }}>
              Comma-separated list of extensions to import alongside the video file
            </div>
          </div>
        </div>
      </SectionCard>

      {/* File Management */}
      <SectionCard title="File Management">
        <ToggleRow
          label="Unmonitor Deleted Episodes"
          description="When an episode file is deleted from disk, stop monitoring that episode"
          checked={form.unmonitor_deleted_episodes}
          onChange={(v) => set("unmonitor_deleted_episodes", v)}
        />
      </SectionCard>

      {/* Token reference */}
      <div style={{
        background: "var(--color-bg-surface)",
        border: "1px solid var(--color-border-subtle)",
        borderRadius: 10,
        padding: "16px 20px",
      }}>
        <div style={{ fontSize: 12, fontWeight: 600, color: "var(--color-text-secondary)", marginBottom: 10 }}>
          Available Tokens
        </div>
        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "4px 24px" }}>
          {[
            ["{Series Title}", "Series title as-is"],
            ["{Series CleanTitle}", "Filesystem-safe title"],
            ["{Series Year}", "Year the series premiered"],
            ["{Season}", "Season number"],
            ["{Season:00}", "Season number zero-padded"],
            ["{Episode}", "Episode number"],
            ["{Episode:00}", "Episode number zero-padded"],
            ["{Air-Date}", "Episode air date"],
            ["{Episode Title}", "Episode title"],
            ["{Quality Full}", "e.g. HDTV-720p"],
            ["{MediaInfo VideoCodec}", "e.g. x264, x265"],
          ].map(([token, desc]) => (
            <div key={token} style={{ display: "flex", gap: 8, alignItems: "baseline" }}>
              <code style={{
                fontSize: 11,
                fontFamily: "var(--font-family-mono)",
                color: "var(--color-accent)",
                background: "color-mix(in srgb, var(--color-accent) 10%, transparent)",
                borderRadius: 3,
                padding: "1px 4px",
                whiteSpace: "nowrap",
              }}>
                {token}
              </code>
              <span style={{ fontSize: 11, color: "var(--color-text-muted)" }}>{desc}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
