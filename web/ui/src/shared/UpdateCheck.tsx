import { useState } from "react";
import type { ReactNode } from "react";
import Modal from "./Modal";
import { apiFetch } from "./api";

// Shape of GET /api/v1/system/updates (internal/updatecheck in each app).
export interface UpdateCommit {
  sha: string;
  subject: string;
  url: string;
}

export interface UpdateStatus {
  update_available: boolean;
  current_version: string;
  latest_version: string;
  release_url?: string;
  release_notes?: string;
  published_at?: string;
  behind?: number;
  commits?: UpdateCommit[];
  message?: string;
}

interface UpdateCheckProps {
  /** Display name, e.g. "Prism". */
  app: string;
  /** Compose service and container name, e.g. "prism". */
  service: string;
  /** Image without tag, e.g. "ghcr.io/beacon-stack/prism". */
  image: string;
}

/**
 * "Check for updates" button for a Beacon app's System page. Asks the
 * app's own /system/updates endpoint, which compares the running build
 * with the newest image published from beacon-stack/<app>, and shows
 * what changed and how to update.
 */
export default function UpdateCheck({ app, service, image }: UpdateCheckProps) {
  const [status, setStatus] = useState<UpdateStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [checking, setChecking] = useState(false);
  const [open, setOpen] = useState(false);

  async function check() {
    setChecking(true);
    setError(null);
    try {
      const s = await apiFetch<UpdateStatus>("/system/updates");
      setStatus(s);
      if (s.update_available) setOpen(true);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Couldn't check for updates");
    } finally {
      setChecking(false);
    }
  }

  return (
    <div style={{ display: "flex", alignItems: "center", gap: 10, flexWrap: "wrap" }}>
      <button
        onClick={check}
        disabled={checking}
        style={{
          background: "var(--color-bg-elevated)",
          border: "1px solid var(--color-border-default)",
          borderRadius: 6,
          padding: "3px 10px",
          fontSize: 12,
          color: checking ? "var(--color-text-muted)" : "var(--color-text-secondary)",
          cursor: checking ? "not-allowed" : "pointer",
        }}
      >
        {checking ? "Checking…" : "Check for updates"}
      </button>
      {status && !status.update_available && !status.message && (
        <span style={{ fontSize: 12, color: "var(--color-success)" }}>Up to date ✓</span>
      )}
      {status?.update_available && (
        <button
          onClick={() => setOpen(true)}
          style={{
            background: "none",
            border: "none",
            padding: 0,
            fontSize: 12,
            color: "var(--color-accent)",
            cursor: "pointer",
          }}
        >
          {status.behind ? `${status.behind} new change${status.behind === 1 ? "" : "s"} available` : "Update available"} →
        </button>
      )}
      {status?.message && !status.update_available && (
        <span style={{ fontSize: 12, color: "var(--color-text-muted)" }}>{status.message}</span>
      )}
      {error && <span style={{ fontSize: 12, color: "var(--color-danger, #e5484d)" }}>{error}</span>}
      {open && status?.update_available && (
        <UpdateModal status={status} app={app} service={service} image={image} onClose={() => setOpen(false)} />
      )}
    </div>
  );
}

function UpdateModal({
  status,
  app,
  service,
  image,
  onClose,
}: { status: UpdateStatus; onClose: () => void } & UpdateCheckProps) {
  const [method, setMethod] = useState<"compose" | "docker">("compose");
  // A build whose version is a commit came from :main (every merge to
  // main); a release build (0.13.0) from :latest (releases only).
  const onMain = /^[0-9a-f]{7,40}$/i.test(status.current_version);
  const tag = onMain ? "main" : "latest";
  const compose = `docker compose pull ${service}\ndocker compose up -d ${service}`;
  const docker = `docker pull ${image}:${tag}\n# then recreate the ${service} container with the same settings as before`;
  const cmd = method === "compose" ? compose : docker;
  const safeURL = (u?: string) => (u && /^https:\/\//i.test(u) ? u : undefined);

  return (
    <Modal onClose={onClose} width={600} maxHeight="85vh" innerStyle={{ padding: 24, overflowY: "auto" }}>
      <h2 style={{ margin: "0 0 14px", fontSize: 17, fontWeight: 600, color: "var(--color-text-primary)" }}>
        {app} update available
      </h2>

      <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 16, flexWrap: "wrap" }}>
        <Badge>{status.current_version}</Badge>
        <span style={{ color: "var(--color-text-muted)", fontSize: 13 }}>→</span>
        <Badge fresh>{status.latest_version}</Badge>
        {status.published_at && (
          <span style={{ fontSize: 12, color: "var(--color-text-muted)", marginLeft: "auto" }}>
            published {new Date(status.published_at).toLocaleString()}
          </span>
        )}
      </div>

      <p style={{ margin: "0 0 14px", fontSize: 12, color: "var(--color-text-muted)" }}>
        {onMain
          ? `You're on ${image}:main, which gets every change merged to main.`
          : `You're on ${image}:latest, which gets each new release.`}
        {status.message ? ` ${status.message}` : ""}
      </p>

      {status.commits && status.commits.length > 0 && (
        <Section title={`What's new (${status.behind ?? status.commits.length})`}>
          <ul style={{ margin: 0, padding: 0, listStyle: "none", maxHeight: 220, overflowY: "auto" }}>
            {status.commits.map((c) => (
              <li key={c.sha} style={{ display: "flex", gap: 8, padding: "4px 0", fontSize: 13 }}>
                <a
                  href={safeURL(c.url)}
                  target="_blank"
                  rel="noreferrer"
                  style={{ fontFamily: "var(--font-family-mono)", fontSize: 12, color: "var(--color-accent)", flexShrink: 0 }}
                >
                  {c.sha}
                </a>
                <span style={{ color: "var(--color-text-secondary)" }}>{c.subject}</span>
              </li>
            ))}
          </ul>
          {status.behind !== undefined && status.behind > status.commits.length && (
            <p style={{ margin: "6px 0 0", fontSize: 12, color: "var(--color-text-muted)" }}>
              …and {status.behind - status.commits.length} more.
            </p>
          )}
        </Section>
      )}

      {status.release_notes && (
        <Section title="Release notes">
          <div
            style={{
              whiteSpace: "pre-wrap",
              fontSize: 13,
              color: "var(--color-text-secondary)",
              maxHeight: 220,
              overflowY: "auto",
            }}
          >
            {status.release_notes}
          </div>
        </Section>
      )}

      <Section title="How to update">
        <div style={{ display: "flex", marginBottom: 10, borderBottom: "1px solid var(--color-border-subtle)" }}>
          {(["compose", "docker"] as const).map((m) => (
            <button
              key={m}
              onClick={() => setMethod(m)}
              style={{
                background: "none",
                border: "none",
                borderBottom: method === m ? "2px solid var(--color-accent)" : "2px solid transparent",
                padding: "6px 14px",
                fontSize: 12,
                fontWeight: 500,
                color: method === m ? "var(--color-text-primary)" : "var(--color-text-muted)",
                cursor: "pointer",
                marginBottom: -1,
              }}
            >
              {m === "compose" ? "Docker Compose" : "Docker"}
            </button>
          ))}
        </div>
        <div style={{ position: "relative" }}>
          <pre
            style={{
              margin: 0,
              padding: 12,
              borderRadius: 6,
              background: "var(--color-bg-elevated)",
              border: "1px solid var(--color-border-subtle)",
              fontFamily: "var(--font-family-mono)",
              fontSize: 12,
              color: "var(--color-text-primary)",
              whiteSpace: "pre-wrap",
            }}
          >
            {cmd}
          </pre>
          <CopyButton text={cmd} />
        </div>
        {method === "compose" && (
          <p style={{ margin: "6px 0 0", fontSize: 11, color: "var(--color-text-muted)" }}>
            Run in the folder with your compose file. Use your service name if it isn't "{service}".
          </p>
        )}
      </Section>

      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
        {safeURL(status.release_url) ? (
          <a
            href={safeURL(status.release_url)}
            target="_blank"
            rel="noreferrer"
            style={{ fontSize: 13, color: "var(--color-accent)", textDecoration: "none" }}
          >
            View on GitHub →
          </a>
        ) : (
          <span />
        )}
        <button
          onClick={onClose}
          style={{
            background: "var(--color-bg-elevated)",
            border: "1px solid var(--color-border-default)",
            borderRadius: 6,
            padding: "6px 16px",
            fontSize: 13,
            color: "var(--color-text-secondary)",
            cursor: "pointer",
          }}
        >
          Close
        </button>
      </div>
    </Modal>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div style={{ marginBottom: 18 }}>
      <p
        style={{
          margin: "0 0 8px",
          fontSize: 11,
          fontWeight: 600,
          letterSpacing: "0.06em",
          textTransform: "uppercase",
          color: "var(--color-text-muted)",
        }}
      >
        {title}
      </p>
      {children}
    </div>
  );
}

function Badge({ children, fresh }: { children: ReactNode; fresh?: boolean }) {
  return (
    <span
      style={{
        padding: "3px 9px",
        borderRadius: 6,
        fontSize: 13,
        fontFamily: "var(--font-family-mono)",
        background: fresh ? "color-mix(in srgb, var(--color-success) 12%, transparent)" : "var(--color-bg-subtle)",
        color: fresh ? "var(--color-success)" : "var(--color-text-secondary)",
      }}
    >
      {children}
    </span>
  );
}

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <button
      onClick={() => {
        navigator.clipboard?.writeText(text).then(() => {
          setCopied(true);
          setTimeout(() => setCopied(false), 2000);
        });
      }}
      style={{
        position: "absolute",
        top: 6,
        right: 6,
        background: "none",
        border: "none",
        padding: "2px 6px",
        fontSize: 11,
        color: copied ? "var(--color-success)" : "var(--color-text-muted)",
        cursor: "pointer",
      }}
    >
      {copied ? "Copied" : "Copy"}
    </button>
  );
}
