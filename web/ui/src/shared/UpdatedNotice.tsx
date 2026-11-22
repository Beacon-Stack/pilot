import { useEffect, useRef, useState } from "react";
import { apiFetch } from "./api";

interface UpdatedNoticeProps {
  /** Display name, e.g. "Pilot". */
  app: string;
  /** An endpoint whose JSON has a `version` field. Default "/system/status". */
  versionPath?: string;
  /** How often to check while the tab is open. Default one minute. */
  intervalMs?: number;
}

/**
 * Tells someone their open page is older than the server behind it.
 *
 * A single-page app keeps running the code it loaded until the page is
 * reloaded, so a tab left open across an update quietly runs the old UI
 * against the new server. This remembers the server version the page
 * first saw and checks again every minute and whenever the tab comes
 * back into view; when the version changes it offers a reload. Failed
 * checks (the server restarting, say) are ignored.
 */
export default function UpdatedNotice({ app, versionPath = "/system/status", intervalMs = 60_000 }: UpdatedNoticeProps) {
  const first = useRef<string | null>(null);
  const [updated, setUpdated] = useState(false);
  const [dismissed, setDismissed] = useState(false);

  useEffect(() => {
    let live = true;
    async function check() {
      try {
        const res = await apiFetch<{ version?: string }>(versionPath);
        const v = res?.version;
        if (!live || !v) return;
        if (first.current === null) {
          first.current = v;
        } else if (v !== first.current) {
          setUpdated(true);
        }
      } catch {
        // Unreachable for a moment (restarting): try again next time.
      }
    }
    void check();
    const timer = setInterval(check, intervalMs);
    const onVisible = () => {
      if (document.visibilityState === "visible") void check();
    };
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      live = false;
      clearInterval(timer);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, [versionPath, intervalMs]);

  if (!updated || dismissed) return null;
  return (
    <div
      role="status"
      style={{
        position: "fixed",
        bottom: 20,
        left: "50%",
        transform: "translateX(-50%)",
        zIndex: 400,
        display: "flex",
        alignItems: "center",
        gap: 12,
        padding: "10px 14px",
        borderRadius: 8,
        background: "var(--color-bg-elevated)",
        border: "1px solid var(--color-border-default)",
        boxShadow: "var(--shadow-modal, 0 8px 24px rgba(0,0,0,0.3))",
        fontSize: 13,
        color: "var(--color-text-primary)",
        maxWidth: "calc(100vw - 32px)",
      }}
    >
      <span>{app} was updated. Reload to get the latest.</span>
      <button
        onClick={() => window.location.reload()}
        style={{
          padding: "5px 12px",
          borderRadius: 6,
          border: "none",
          background: "var(--color-accent)",
          color: "var(--color-accent-fg)",
          fontSize: 12,
          fontWeight: 500,
          cursor: "pointer",
        }}
      >
        Reload
      </button>
      <button
        onClick={() => setDismissed(true)}
        aria-label="Dismiss"
        style={{ background: "none", border: "none", color: "var(--color-text-muted)", cursor: "pointer", fontSize: 14 }}
      >
        ✕
      </button>
    </div>
  );
}
