// LogViewer — shared in-app log panel.
//
// Every Beacon service (Pulse, Pilot, Prism, Haul) exposes the same
// /api/v1/system/{logs,log-level,logs/docker} endpoints (via
// pulse/pkg/log on the backend). This component is the single
// frontend that consumes them — drop it into any service's
// system/diagnostics/settings page and it just works.
//
// Features (all driven by user feedback in the planning round):
//   - Color-coded rows by level (debug=gray, info=blue,
//     warn=amber, error=red).
//   - Free-text search field (debounced; matches the backend's
//     case-insensitive `q` param so search hits message + string
//     fields).
//   - Level filter dropdown (debug → info → warn → error).
//   - Click row → expands to show the full JSON payload.
//   - Runtime level toggle (PUT /api/v1/system/log-level) so the
//     user can flip to debug for troubleshooting and back without
//     restarting the service.
//   - Source switch: ring buffer (default — fast, current
//     session) vs. Docker stdout (full history when the socket
//     is mounted). Falls back gracefully when Docker isn't
//     reachable.
//   - Live-tail toggle: when on, polls every 2s. When off, the
//     Refresh button fetches once.

import { useEffect, useRef, useState } from "react";
import { ChevronDown, ChevronRight, RefreshCw, Search, Pause, Play } from "lucide-react";
import { apiFetch } from "./api";

// ── Types ────────────────────────────────────────────────────────────────────

export type LogLevel = "debug" | "info" | "warn" | "error";

export interface LogEntry {
  time: string;
  level: string; // "DEBUG" | "INFO" | "WARN" | "ERROR" — uppercase from slog
  message: string;
  fields?: Record<string, unknown>;
}

interface LogListResponse {
  entries: LogEntry[];
  total: number;
}

interface LogLevelResponse {
  level: LogLevel;
}

interface DockerLogsResponse {
  available: boolean;
  entries: LogEntry[] | null;
  reason?: string;
}

// ── Component ────────────────────────────────────────────────────────────────

export interface LogViewerProps {
  // serviceName drives the empty-state copy ("Pulse logs", etc).
  serviceName: string;
  // Polling interval when live-tail is on (default 2000ms).
  liveTailIntervalMs?: number;
}

type Source = "buffer" | "docker";

export default function LogViewer({ serviceName, liveTailIntervalMs = 2000 }: LogViewerProps) {
  const [search, setSearch] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const [levelFilter, setLevelFilter] = useState<LogLevel | "all">("all");
  const [source, setSource] = useState<Source>("buffer");
  const [liveTail, setLiveTail] = useState(true);
  const [entries, setEntries] = useState<LogEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [dockerReason, setDockerReason] = useState<string | null>(null);

  // The currently-active runtime level (separate from the filter).
  const [runtimeLevel, setRuntimeLevel] = useState<LogLevel>("info");

  // Debounce search → query: keep typing fluid, network polite.
  useEffect(() => {
    const t = setTimeout(() => setDebouncedSearch(search.trim()), 250);
    return () => clearTimeout(t);
  }, [search]);

  // Fetch the runtime level once on mount and after a successful PUT.
  const refreshRuntimeLevel = async () => {
    try {
      const r = await apiFetch<LogLevelResponse>("/system/log-level");
      setRuntimeLevel(r.level);
    } catch {
      // ignore — the level field just stays at info if this fails
    }
  };
  useEffect(() => {
    void refreshRuntimeLevel();
  }, []);

  // Fetch logs. Memoise the URL building so liveTail's re-fetch is cheap.
  const fetchLogs = async () => {
    setLoading(true);
    setError(null);
    try {
      if (source === "buffer") {
        const qs = new URLSearchParams();
        if (levelFilter !== "all") qs.set("level", levelFilter);
        if (debouncedSearch) qs.set("q", debouncedSearch);
        qs.set("limit", "500");
        const r = await apiFetch<LogListResponse>(`/system/logs?${qs}`);
        setEntries(r.entries ?? []);
        setTotal(r.total);
        setDockerReason(null);
      } else {
        const qs = new URLSearchParams();
        if (debouncedSearch) qs.set("q", debouncedSearch);
        qs.set("tail", "500");
        const r = await apiFetch<DockerLogsResponse>(`/system/logs/docker?${qs}`);
        if (!r.available) {
          setEntries([]);
          setTotal(0);
          setDockerReason(r.reason ?? "Docker stdout not available.");
        } else {
          let filtered = r.entries ?? [];
          // Docker endpoint doesn't filter by level server-side — apply
          // client-side so the UX matches the buffer source.
          if (levelFilter !== "all") {
            const min = levelOrdinal(levelFilter);
            filtered = filtered.filter((e) => levelOrdinal(e.level) >= min);
          }
          setEntries(filtered);
          setTotal(filtered.length);
          setDockerReason(null);
        }
      }
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setLoading(false);
    }
  };

  // Re-fetch whenever filters change.
  useEffect(() => {
    void fetchLogs();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [debouncedSearch, levelFilter, source]);

  // Live tail: poll on interval when enabled.
  const tailRef = useRef<ReturnType<typeof setInterval> | null>(null);
  useEffect(() => {
    if (tailRef.current) {
      clearInterval(tailRef.current);
      tailRef.current = null;
    }
    if (liveTail) {
      tailRef.current = setInterval(() => {
        void fetchLogs();
      }, liveTailIntervalMs);
    }
    return () => {
      if (tailRef.current) clearInterval(tailRef.current);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [liveTail, debouncedSearch, levelFilter, source, liveTailIntervalMs]);

  // Runtime level change.
  const setRuntime = async (next: LogLevel) => {
    try {
      const r = await apiFetch<LogLevelResponse>("/system/log-level", {
        method: "PUT",
        body: JSON.stringify({ level: next }),
      });
      setRuntimeLevel(r.level);
    } catch (err) {
      setError((err as Error).message);
    }
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
      {/* Toolbar */}
      <div
        style={{
          display: "flex",
          gap: 10,
          alignItems: "center",
          flexWrap: "wrap",
          padding: "12px 14px",
          background: "var(--color-bg-surface)",
          border: "1px solid var(--color-border-subtle)",
          borderRadius: 8,
        }}
      >
        {/* Search */}
        <div style={{ position: "relative", flex: "1 1 240px", maxWidth: 400 }}>
          <Search
            size={14}
            style={{
              position: "absolute",
              left: 10,
              top: "50%",
              transform: "translateY(-50%)",
              color: "var(--color-text-muted)",
              pointerEvents: "none",
            }}
          />
          <input
            type="search"
            placeholder="Search messages + fields…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            style={{
              width: "100%",
              padding: "7px 10px 7px 32px",
              borderRadius: 6,
              border: "1px solid var(--color-border-default)",
              background: "var(--color-bg-elevated)",
              color: "var(--color-text-primary)",
              fontSize: 13,
              outline: "none",
            }}
          />
        </div>

        {/* Filter (display level — what entries to show) */}
        <select
          value={levelFilter}
          onChange={(e) => setLevelFilter(e.target.value as LogLevel | "all")}
          style={selectStyle}
          title="Show only entries at this level or higher"
        >
          <option value="all">All levels</option>
          <option value="debug">Debug+</option>
          <option value="info">Info+</option>
          <option value="warn">Warn+</option>
          <option value="error">Error only</option>
        </select>

        {/* Source */}
        <select
          value={source}
          onChange={(e) => setSource(e.target.value as Source)}
          style={selectStyle}
          title="Where to read logs from"
        >
          <option value="buffer">In-memory (this session)</option>
          <option value="docker">Docker stdout (full history)</option>
        </select>

        {/* Runtime level */}
        <span style={{ fontSize: 11, color: "var(--color-text-muted)", display: "flex", alignItems: "center", gap: 6 }}>
          Runtime:
          <select
            value={runtimeLevel}
            onChange={(e) => void setRuntime(e.target.value as LogLevel)}
            style={{ ...selectStyle, fontSize: 11, padding: "3px 6px" }}
            title="Change the minimum level the service emits. No restart required."
          >
            <option value="debug">debug</option>
            <option value="info">info</option>
            <option value="warn">warn</option>
            <option value="error">error</option>
          </select>
        </span>

        {/* Live-tail toggle */}
        <button
          onClick={() => setLiveTail((v) => !v)}
          style={{
            ...buttonStyle,
            background: liveTail ? "color-mix(in srgb, var(--color-accent) 12%, transparent)" : "transparent",
            color: liveTail ? "var(--color-accent)" : "var(--color-text-secondary)",
            borderColor: liveTail ? "var(--color-accent)" : "var(--color-border-default)",
          }}
          title={liveTail ? `Pausing pulls polling. Currently every ${liveTailIntervalMs / 1000}s.` : "Resume live tail"}
        >
          {liveTail ? <Pause size={12} /> : <Play size={12} />}
          {liveTail ? "Live" : "Paused"}
        </button>

        {/* Manual refresh */}
        <button
          onClick={() => void fetchLogs()}
          disabled={loading}
          style={buttonStyle}
          title="Fetch the latest entries now"
        >
          <RefreshCw size={12} />
          Refresh
        </button>
      </div>

      {/* Status line */}
      <div style={{ fontSize: 11, color: "var(--color-text-muted)", display: "flex", justifyContent: "space-between" }}>
        <span>
          {entries.length === 0
            ? `No ${serviceName} log entries match the filter.`
            : `${entries.length} of ${total.toLocaleString()} entries`}
          {error && <span style={{ color: "var(--color-danger)", marginLeft: 8 }}>· {error}</span>}
        </span>
        {dockerReason && (
          <span style={{ color: "var(--color-warning)" }}>{dockerReason}</span>
        )}
      </div>

      {/* Entry list */}
      <div
        style={{
          background: "var(--color-bg-surface)",
          border: "1px solid var(--color-border-subtle)",
          borderRadius: 8,
          overflow: "hidden",
        }}
      >
        {entries.map((e, i) => (
          <LogRow key={`${e.time}-${i}`} entry={e} isLast={i === entries.length - 1} />
        ))}
        {entries.length === 0 && !loading && (
          <div
            style={{
              padding: "32px 14px",
              textAlign: "center",
              fontSize: 13,
              color: "var(--color-text-muted)",
            }}
          >
            {dockerReason
              ? "Switch source back to In-memory to see the ring buffer."
              : `No ${serviceName} log entries match the current filter.`}
          </div>
        )}
      </div>
    </div>
  );
}

// ── Row ──────────────────────────────────────────────────────────────────────

function LogRow({ entry, isLast }: { entry: LogEntry; isLast: boolean }) {
  const [open, setOpen] = useState(false);
  const color = levelColor(entry.level);
  const hasPayload = entry.fields && Object.keys(entry.fields).length > 0;

  return (
    <div
      style={{
        borderBottom: isLast ? "none" : "1px solid var(--color-border-subtle)",
      }}
    >
      <div
        onClick={() => hasPayload && setOpen((v) => !v)}
        style={{
          display: "flex",
          alignItems: "flex-start",
          gap: 8,
          padding: "8px 12px",
          cursor: hasPayload ? "pointer" : "default",
          fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
          fontSize: 12,
          lineHeight: 1.4,
        }}
        onMouseEnter={(ev) => {
          if (hasPayload) ev.currentTarget.style.background = "var(--color-bg-elevated)";
        }}
        onMouseLeave={(ev) => {
          ev.currentTarget.style.background = "transparent";
        }}
      >
        {hasPayload ? (
          open ? (
            <ChevronDown size={11} style={{ color: "var(--color-text-muted)", marginTop: 4, flexShrink: 0 }} />
          ) : (
            <ChevronRight size={11} style={{ color: "var(--color-text-muted)", marginTop: 4, flexShrink: 0 }} />
          )
        ) : (
          <span style={{ width: 11, flexShrink: 0 }} />
        )}
        <span
          style={{
            color: "var(--color-text-muted)",
            flexShrink: 0,
            fontVariantNumeric: "tabular-nums",
          }}
        >
          {formatTime(entry.time)}
        </span>
        <span
          style={{
            color,
            fontWeight: 600,
            width: 56,
            flexShrink: 0,
            textTransform: "uppercase",
            fontSize: 10,
            letterSpacing: 0.6,
            paddingTop: 1,
          }}
        >
          {entry.level}
        </span>
        <span style={{ color: "var(--color-text-primary)", wordBreak: "break-word", flex: 1, minWidth: 0 }}>
          {entry.message}
        </span>
        {/* Inline first-2 fields preview when collapsed */}
        {!open && hasPayload && (
          <span style={{ color: "var(--color-text-muted)", flexShrink: 0, maxWidth: "40%", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
            {previewFields(entry.fields!)}
          </span>
        )}
      </div>

      {open && hasPayload && (
        <div
          style={{
            padding: "8px 12px 12px 32px",
            background: "var(--color-bg-elevated)",
            borderTop: "1px solid var(--color-border-subtle)",
            fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
            fontSize: 11,
            color: "var(--color-text-secondary)",
            lineHeight: 1.5,
            whiteSpace: "pre-wrap",
            wordBreak: "break-word",
          }}
        >
          {JSON.stringify(entry.fields, null, 2)}
        </div>
      )}
    </div>
  );
}

// ── Helpers ──────────────────────────────────────────────────────────────────

function levelColor(level: string): string {
  switch (level.toUpperCase()) {
    case "DEBUG":
      return "var(--color-text-muted)";
    case "INFO":
      return "var(--color-status-downloading)"; // blue accent in every theme
    case "WARN":
    case "WARNING":
      return "var(--color-warning)";
    case "ERROR":
    case "ERR":
      return "var(--color-danger)";
    default:
      return "var(--color-text-secondary)";
  }
}

function levelOrdinal(level: string): number {
  switch (level.toUpperCase()) {
    case "DEBUG":
      return -4;
    case "INFO":
      return 0;
    case "WARN":
    case "WARNING":
      return 4;
    case "ERROR":
    case "ERR":
      return 8;
    default:
      return -100;
  }
}

function formatTime(iso: string): string {
  try {
    const d = new Date(iso);
    return d.toLocaleTimeString(undefined, {
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
      hour12: false,
    });
  } catch {
    return iso;
  }
}

function previewFields(fields: Record<string, unknown>): string {
  // Pick the most useful 2 fields for the inline preview. Skip "service"
  // and "error" — service is always set and noisy; error is shown
  // expanded.
  const keys = Object.keys(fields).filter((k) => k !== "service" && k !== "error");
  const preview = keys.slice(0, 2).map((k) => {
    const v = fields[k];
    const s = typeof v === "string" ? v : JSON.stringify(v);
    return `${k}=${s.length > 40 ? s.slice(0, 40) + "…" : s}`;
  });
  return preview.join(" ");
}

// ── Reusable styles ──────────────────────────────────────────────────────────

const buttonStyle: React.CSSProperties = {
  display: "inline-flex",
  alignItems: "center",
  gap: 5,
  padding: "6px 10px",
  fontSize: 12,
  borderRadius: 6,
  border: "1px solid var(--color-border-default)",
  background: "transparent",
  color: "var(--color-text-secondary)",
  cursor: "pointer",
};

const selectStyle: React.CSSProperties = {
  padding: "6px 10px",
  fontSize: 12,
  borderRadius: 6,
  border: "1px solid var(--color-border-default)",
  background: "var(--color-bg-elevated)",
  color: "var(--color-text-primary)",
  outline: "none",
};
