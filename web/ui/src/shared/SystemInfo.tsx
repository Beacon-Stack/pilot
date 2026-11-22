import { useEffect, useState } from "react";
import type { ReactNode } from "react";
import { apiFetch } from "./api";
import SystemCard, { SystemRow } from "./SystemCard";
import UpdateCheck from "./UpdateCheck";

/** GET /api/v1/system/status, the same shape in every Beacon app. */
export interface SystemStatus {
  app_name: string;
  version: string;
  build_time: string;
  go_version: string;
  db_type: string;
  db_path?: string;
  uptime_seconds: number;
  start_time: string;
}

interface SystemInfoProps {
  /** Display name, e.g. "Pilot". */
  app: string;
  /** Compose service name, for the update instructions. */
  service: string;
  /** Image name, e.g. "ghcr.io/beacon-stack/pilot". */
  image: string;
  /** App-specific rows shown after the standard ones. */
  extraRows?: { label: string; value: ReactNode }[];
}

function formatUptime(seconds: number): string {
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

function formatTime(iso: string): string {
  const t = new Date(iso);
  return Number.isNaN(t.getTime()) ? iso || "—" : t.toLocaleString();
}

const mono = { fontFamily: "var(--font-family-mono)", fontSize: 12 } as const;

/**
 * The About card at the top of every Beacon app's System page: which
 * build is running, since when, where its data lives, and the update
 * check. Reads /system/status and refreshes it every 30 seconds.
 */
export default function SystemInfo({ app, service, image, extraRows = [] }: SystemInfoProps) {
  const [status, setStatus] = useState<SystemStatus | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    let live = true;
    async function load() {
      try {
        const s = await apiFetch<SystemStatus>("/system/status");
        if (live) {
          setStatus(s);
          setFailed(false);
        }
      } catch {
        if (live) setFailed(true);
      }
    }
    void load();
    const timer = setInterval(load, 30_000);
    return () => {
      live = false;
      clearInterval(timer);
    };
  }, []);

  return (
    <SystemCard title="About">
      {!status && !failed && (
        <div style={{ padding: 16 }}>
          {[60, 40, 50, 45].map((w, i) => (
            <div key={i} className="skeleton" style={{ height: 14, borderRadius: 4, marginBottom: 8, width: `${w}%` }} />
          ))}
        </div>
      )}
      {!status && failed && (
        <div style={{ padding: 16, fontSize: 13, color: "var(--color-text-secondary)" }}>Couldn't load system information.</div>
      )}
      {status && (
        <>
          <SystemRow label="Version"><span style={mono}>{status.version}</span></SystemRow>
          <SystemRow label="Built"><span style={mono}>{formatTime(status.build_time)}</span></SystemRow>
          <SystemRow label="Started"><span style={mono}>{formatTime(status.start_time)}</span></SystemRow>
          <SystemRow label="Uptime">{formatUptime(status.uptime_seconds)}</SystemRow>
          <SystemRow label="Database">
            {status.db_type}
            {status.db_path && <span style={{ ...mono, display: "block", color: "var(--color-text-muted)", marginTop: 2 }}>{status.db_path}</span>}
          </SystemRow>
          <SystemRow label="Go"><span style={mono}>{status.go_version}</span></SystemRow>
          {extraRows.map((r) => (
            <SystemRow key={r.label} label={r.label}>{r.value}</SystemRow>
          ))}
          <SystemRow label="API docs">
            <a href="/api/docs" target="_blank" rel="noopener" style={{ color: "var(--color-accent)", textDecoration: "none" }}>/api/docs</a>
          </SystemRow>
          <SystemRow label="Updates" last>
            <UpdateCheck app={app} service={service} image={image} />
          </SystemRow>
        </>
      )}
    </SystemCard>
  );
}
