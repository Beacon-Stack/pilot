import { useState, useRef } from "react";
import { Monitor, Moon, Sun, Copy, Check, Download, Upload } from "lucide-react";
import { toast } from "sonner";
import PageHeader from "@/components/PageHeader";
import {
  THEME_PRESETS,
  getStoredMode,
  getStoredPreset,
  resolveMode,
  setThemeMode,
  setThemePreset,
  getTooltipsEnabled,
  setTooltipsEnabled,
} from "@/theme";
import type { ThemeMode } from "@/theme";
import { apiFetch } from "@/api/client";

// ── Shared styles ────────────────────────────────────────────────────────────

const card: React.CSSProperties = {
  background: "var(--color-bg-surface)",
  border: "1px solid var(--color-border-subtle)",
  borderRadius: 8,
  padding: 20,
  boxShadow: "var(--shadow-card)",
};

const sectionHeader: React.CSSProperties = {
  fontSize: 11,
  fontWeight: 600,
  letterSpacing: "0.08em",
  textTransform: "uppercase",
  color: "var(--color-text-muted)",
  margin: "0 0 12px",
};

// ── Mode button helper ───────────────────────────────────────────────────────

function ModeButton({
  mode,
  currentMode,
  icon: Icon,
  label,
  onClick,
}: {
  mode: ThemeMode;
  currentMode: ThemeMode;
  icon: React.ElementType;
  label: string;
  onClick: () => void;
}) {
  const active = currentMode === mode;
  return (
    <button
      onClick={onClick}
      style={{
        display: "flex",
        alignItems: "center",
        gap: 6,
        padding: "6px 14px",
        borderRadius: 6,
        border: active
          ? "1px solid var(--color-accent)"
          : "1px solid var(--color-border-default)",
        background: active ? "var(--color-accent-muted)" : "var(--color-bg-elevated)",
        color: active ? "var(--color-accent-hover)" : "var(--color-text-secondary)",
        fontSize: 13,
        fontWeight: 500,
        cursor: "pointer",
        transition: "background 120ms ease, border-color 120ms ease, color 120ms ease",
      }}
      onMouseEnter={(e) => {
        if (!active) {
          (e.currentTarget as HTMLButtonElement).style.borderColor = "var(--color-border-strong)";
          (e.currentTarget as HTMLButtonElement).style.color = "var(--color-text-primary)";
        }
      }}
      onMouseLeave={(e) => {
        if (!active) {
          (e.currentTarget as HTMLButtonElement).style.borderColor = "var(--color-border-default)";
          (e.currentTarget as HTMLButtonElement).style.color = "var(--color-text-secondary)";
        }
      }}
    >
      <Icon size={14} strokeWidth={2} />
      {label}
    </button>
  );
}

// ── Theme swatch ─────────────────────────────────────────────────────────────

function ThemeSwatch({
  preset,
  isSelected,
  onClick,
}: {
  preset: (typeof THEME_PRESETS)[number];
  isSelected: boolean;
  onClick: () => void;
}) {
  return (
    <button
      onClick={onClick}
      title={preset.label}
      style={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        gap: 6,
        padding: 0,
        background: "none",
        border: "none",
        cursor: "pointer",
      }}
    >
      <div
        style={{
          width: 72,
          height: 48,
          borderRadius: 8,
          background: preset.preview.bg,
          border: isSelected
            ? "2px solid var(--color-accent)"
            : "2px solid transparent",
          outline: isSelected ? "2px solid var(--color-accent)" : "2px solid transparent",
          outlineOffset: 1,
          overflow: "hidden",
          position: "relative",
          boxShadow: isSelected ? "0 0 0 3px var(--color-accent-muted)" : undefined,
          transition: "border-color 150ms ease",
        }}
      >
        {/* Mini preview: sidebar + content area with text/accent bars */}
        <div style={{ position: "absolute", inset: 0, display: "flex" }}>
          <div style={{ width: 16, background: preset.preview.surface, height: "100%" }} />
          <div
            style={{
              flex: 1,
              display: "flex",
              flexDirection: "column",
              justifyContent: "center",
              alignItems: "center",
              gap: 3,
            }}
          >
            <div
              style={{
                width: 24,
                height: 3,
                borderRadius: 2,
                background: preset.preview.text,
                opacity: 0.7,
              }}
            />
            <div
              style={{
                width: 16,
                height: 3,
                borderRadius: 2,
                background: preset.preview.accent,
                opacity: 0.9,
              }}
            />
          </div>
        </div>
        {/* Checkmark overlay */}
        {isSelected && (
          <div
            style={{
              position: "absolute",
              inset: 0,
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              background: "rgba(0,0,0,0.35)",
            }}
          >
            <Check size={18} strokeWidth={2.5} color="#fff" />
          </div>
        )}
      </div>
      <span
        style={{
          fontSize: 11,
          color: isSelected ? "var(--color-accent)" : "var(--color-text-secondary)",
          fontWeight: isSelected ? 600 : 400,
          whiteSpace: "nowrap",
        }}
      >
        {preset.label}
      </span>
    </button>
  );
}

// ── Toggle row ───────────────────────────────────────────────────────────────

function ToggleRow({
  label,
  description,
  checked,
  onChange,
}: {
  label: string;
  description: string;
  checked: boolean;
  onChange: (v: boolean) => void;
}) {
  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        justifyContent: "space-between",
        gap: 16,
      }}
    >
      <div>
        <span
          style={{
            display: "block",
            fontSize: 13,
            fontWeight: 500,
            color: "var(--color-text-primary)",
            marginBottom: 2,
          }}
        >
          {label}
        </span>
        <span style={{ fontSize: 12, color: "var(--color-text-muted)" }}>{description}</span>
      </div>
      <button
        role="switch"
        aria-checked={checked}
        onClick={() => onChange(!checked)}
        style={{
          width: 40,
          height: 22,
          borderRadius: 11,
          border: "none",
          background: checked ? "var(--color-accent)" : "var(--color-bg-subtle)",
          cursor: "pointer",
          position: "relative",
          flexShrink: 0,
          transition: "background 150ms ease",
        }}
      >
        <span
          style={{
            position: "absolute",
            top: 3,
            left: checked ? 21 : 3,
            width: 16,
            height: 16,
            borderRadius: "50%",
            background: "var(--color-bg-base)",
            transition: "left 150ms ease",
          }}
        />
      </button>
    </div>
  );
}

// ── API Key section ──────────────────────────────────────────────────────────

function APIKeySection() {
  const [fullKey, setFullKey] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [copied, setCopied] = useState(false);

  const show = fullKey !== null;

  async function handleReveal() {
    if (fullKey) {
      setFullKey(null);
      return;
    }
    setLoading(true);
    try {
      const data = await apiFetch<{ api_key: string }>("/system/config/apikey");
      setFullKey(data.api_key);
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setLoading(false);
    }
  }

  async function handleCopy() {
    try {
      let key = fullKey;
      if (!key) {
        const data = await apiFetch<{ api_key: string }>("/system/config/apikey");
        key = data.api_key;
      }
      await navigator.clipboard.writeText(key);
      setCopied(true);
      toast.success("API key copied");
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      toast.error((err as Error).message);
    }
  }

  return (
    <div style={card}>
      <p style={sectionHeader}>API Key</p>
      <p style={{ fontSize: 12, color: "var(--color-text-muted)", margin: "0 0 12px" }}>
        Use this key for external integrations (scripts, other *arr apps, Overseerr).
      </p>
      <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
        <code
          style={{
            flex: 1,
            background: "var(--color-bg-elevated)",
            border: "1px solid var(--color-border-default)",
            borderRadius: 6,
            padding: "8px 12px",
            fontSize: 13,
            fontFamily: "var(--font-family-mono, monospace)",
            color: "var(--color-text-primary)",
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
            userSelect: show ? "all" : "none",
          }}
        >
          {show ? fullKey : "••••••••••••••••"}
        </code>
        <button
          onClick={handleReveal}
          disabled={loading}
          style={{
            background: "none",
            border: "none",
            cursor: "pointer",
            fontSize: 12,
            color: "var(--color-text-muted)",
            padding: "4px 6px",
            whiteSpace: "nowrap",
          }}
        >
          {loading ? "…" : show ? "hide" : "show"}
        </button>
        <button
          onClick={handleCopy}
          title="Copy to clipboard"
          style={{
            display: "flex",
            alignItems: "center",
            gap: 4,
            background: "var(--color-bg-elevated)",
            border: "1px solid var(--color-border-default)",
            borderRadius: 6,
            padding: "6px 12px",
            fontSize: 12,
            color: copied ? "var(--color-success)" : "var(--color-text-secondary)",
            cursor: "pointer",
            whiteSpace: "nowrap",
          }}
          onMouseEnter={(e) => {
            if (!copied) (e.currentTarget as HTMLButtonElement).style.background = "var(--color-bg-subtle)";
          }}
          onMouseLeave={(e) => {
            (e.currentTarget as HTMLButtonElement).style.background = "var(--color-bg-elevated)";
          }}
        >
          <Copy size={13} strokeWidth={2} />
          {copied ? "Copied" : "Copy"}
        </button>
      </div>
    </div>
  );
}

// ── Backup & Restore section ─────────────────────────────────────────────

function BackupRestoreSection() {
  const fileRef = useRef<HTMLInputElement>(null);
  const [uploading, setUploading] = useState(false);

  async function handleRestore(file: File) {
    setUploading(true);
    try {
      const res = await fetch("/api/v1/system/restore", {
        method: "POST",
        headers: { "Content-Type": "application/octet-stream" },
        body: file,
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({ title: "Upload failed" }));
        throw new Error(data.title || `HTTP ${res.status}`);
      }
      toast.success("Restore staged — restart Pilot to apply the backup.");
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setUploading(false);
      if (fileRef.current) fileRef.current.value = "";
    }
  }

  return (
    <div style={card}>
      <p style={sectionHeader}>Backup & Restore</p>
      <p style={{ fontSize: 12, color: "var(--color-text-muted)", margin: "0 0 16px" }}>
        Download a copy of your database or restore from a previous backup.
      </p>
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        <a
          href="/api/v1/system/backup"
          style={{
            display: "inline-flex",
            alignItems: "center",
            gap: 6,
            background: "var(--color-bg-elevated)",
            border: "1px solid var(--color-border-default)",
            borderRadius: 6,
            padding: "7px 14px",
            fontSize: 13,
            color: "var(--color-text-secondary)",
            textDecoration: "none",
            cursor: "pointer",
          }}
          onMouseEnter={(e) => { (e.currentTarget as HTMLAnchorElement).style.background = "var(--color-bg-subtle)"; }}
          onMouseLeave={(e) => { (e.currentTarget as HTMLAnchorElement).style.background = "var(--color-bg-elevated)"; }}
        >
          <Download size={14} strokeWidth={2} />
          Download Backup
        </a>
        <button
          onClick={() => fileRef.current?.click()}
          disabled={uploading}
          style={{
            display: "inline-flex",
            alignItems: "center",
            gap: 6,
            background: "var(--color-bg-elevated)",
            border: "1px solid var(--color-border-default)",
            borderRadius: 6,
            padding: "7px 14px",
            fontSize: 13,
            color: "var(--color-text-secondary)",
            cursor: uploading ? "wait" : "pointer",
            opacity: uploading ? 0.6 : 1,
          }}
          onMouseEnter={(e) => {
            if (!uploading) (e.currentTarget as HTMLButtonElement).style.background = "var(--color-bg-subtle)";
          }}
          onMouseLeave={(e) => {
            (e.currentTarget as HTMLButtonElement).style.background = "var(--color-bg-elevated)";
          }}
        >
          <Upload size={14} strokeWidth={2} />
          {uploading ? "Uploading…" : "Restore from File"}
        </button>
        <input
          ref={fileRef}
          type="file"
          accept=".db"
          style={{ display: "none" }}
          onChange={(e) => {
            const file = e.target.files?.[0];
            if (file) handleRestore(file);
          }}
        />
      </div>
    </div>
  );
}

// ── Main page ────────────────────────────────────────────────────────────────

export default function AppSettings() {
  const [mode, setMode] = useState<ThemeMode>(getStoredMode);
  const resolved = resolveMode(mode);
  const [selectedPreset, setSelectedPreset] = useState(() => getStoredPreset(resolved));
  const [tooltips, setTooltips] = useState(getTooltipsEnabled);

  const visiblePresets = THEME_PRESETS.filter((p) => p.mode === resolved);

  function handleMode(m: ThemeMode) {
    setMode(m);
    setThemeMode(m);
    const newResolved = resolveMode(m);
    setSelectedPreset(getStoredPreset(newResolved));
  }

  function handlePreset(id: string) {
    const preset = THEME_PRESETS.find((p) => p.id === id);
    if (!preset) return;
    setSelectedPreset(id);
    setThemePreset(preset.mode, id);
  }

  function handleTooltips(v: boolean) {
    setTooltips(v);
    setTooltipsEnabled(v);
  }

  return (
    <>
      <PageHeader
        title="App Settings"
        description="Customize the appearance and behaviour of Pilot."
      />

      <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
        {/* ── Appearance ──────────────────────────────────────────────────── */}
        <div style={card}>
          <p style={sectionHeader}>Appearance</p>

          {/* Mode toggle */}
          <div style={{ marginBottom: 20 }}>
            <span
              style={{
                display: "block",
                fontSize: 13,
                fontWeight: 500,
                color: "var(--color-text-primary)",
                marginBottom: 10,
              }}
            >
              Color mode
            </span>
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
              <ModeButton mode="dark" currentMode={mode} icon={Moon} label="Dark" onClick={() => handleMode("dark")} />
              <ModeButton mode="light" currentMode={mode} icon={Sun} label="Light" onClick={() => handleMode("light")} />
              <ModeButton mode="system" currentMode={mode} icon={Monitor} label="System" onClick={() => handleMode("system")} />
            </div>
          </div>

          {/* Preset grid — only shows presets matching the active mode */}
          <div>
            <span
              style={{
                display: "block",
                fontSize: 13,
                fontWeight: 500,
                color: "var(--color-text-primary)",
                marginBottom: 10,
              }}
            >
              Theme
            </span>
            <div
              style={{
                display: "grid",
                gridTemplateColumns: "repeat(auto-fill, minmax(80px, 1fr))",
                gap: 12,
              }}
            >
              {visiblePresets.map((p) => (
                <ThemeSwatch
                  key={p.id}
                  preset={p}
                  isSelected={selectedPreset === p.id}
                  onClick={() => handlePreset(p.id)}
                />
              ))}
            </div>
          </div>
        </div>

        {/* ── UI Preferences ──────────────────────────────────────────────── */}
        <div style={card}>
          <p style={sectionHeader}>UI Preferences</p>
          <ToggleRow
            label="Tooltips"
            description="Show informational tooltips when hovering over UI elements."
            checked={tooltips}
            onChange={handleTooltips}
          />
        </div>

        {/* ── API Key ─────────────────────────────────────────────────────── */}
        <APIKeySection />

        {/* ── Backup & Restore ───────────────────────────────────────────── */}
        <BackupRestoreSection />
      </div>
    </>
  );
}
