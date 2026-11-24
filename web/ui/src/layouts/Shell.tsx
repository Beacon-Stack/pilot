import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { summarizeHealth, useHealth } from "@/api/health";
import {
  AlertTriangle,
  BookOpen,
  LogOut,
  Tv,
} from "lucide-react";
import Shell from "@beacon-shared/Shell";
import { mainNav, settingsNav } from "./nav";
import UpdatedNotice from "@beacon-shared/UpdatedNotice";
import { useSystemStatus } from "@/api/system";
import { applyTheme } from "@/theme";
import { getAuthStatus, logout } from "@/lib/auth";
import { useCommandPalette } from "@/components/command-palette/useCommandPalette";
import { CommandPalette } from "@/components/command-palette/CommandPalette";

// Pilot fetches its display name from /api/system/status (so the
// branding can be re-skinned without a frontend rebuild). The shared
// Shell takes appName as a string, so we resolve it here and pass it
// down — useSystemStatus's re-render flows through naturally.
function useAppName(): string {
  const { data: status } = useSystemStatus();
  return status?.app_name ?? "Pilot";
}

// AppIcon wraps the Tv glyph in Pilot's accent-color tile.
function AppIcon() {
  return (
    <div
      style={{
        width: 32,
        height: 32,
        borderRadius: "8px",
        background: "var(--color-accent)",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        flexShrink: 0,
      }}
    >
      <Tv size={18} color="white" strokeWidth={2} />
    </div>
  );
}

// Shows "N errors, M warnings" in the sidebar footer, linking to the
// System page, when health checks found something. Nothing when healthy.
function HealthIndicator() {
  const { data } = useHealth();
  const summary = summarizeHealth(data);
  if (!summary) return null;
  const hasError = data?.issues.some((i) => i.level === "error");
  const color = hasError ? "var(--color-danger)" : "var(--color-warning)";
  return (
    <Link
      to="/settings/system"
      title="Health issues"
      style={{
        display: "flex",
        alignItems: "center",
        gap: "8px",
        padding: "0 12px",
        height: "36px",
        color,
        fontSize: "12px",
        textDecoration: "none",
        borderRadius: "6px",
      }}
    >
      <AlertTriangle size={16} strokeWidth={1.5} style={{ flexShrink: 0 }} />
      <span>{summary}</span>
    </Link>
  );
}

// Pilot's docs link sits in the sidebar footer. Rendered as a slot
// rather than via Shell's docsUrl so we keep parity with the previous
// inline styling.
function DocsLink() {
  return (
    <a
      href="https://pilot.tv/docs"
      target="_blank"
      rel="noopener noreferrer"
      style={{
        display: "flex",
        alignItems: "center",
        gap: "8px",
        padding: "0 12px",
        height: "36px",
        color: "var(--color-text-muted)",
        fontSize: "12px",
        textDecoration: "none",
        borderRadius: "6px",
      }}
    >
      <BookOpen size={16} strokeWidth={1.5} style={{ flexShrink: 0 }} />
      <span>Docs</span>
    </a>
  );
}

// Only a login session can be logged out of. On the local network there
// is nothing to log out of, so the button stays hidden.
function LogoutButton() {
  const [hasSession, setHasSession] = useState(false);
  useEffect(() => {
    getAuthStatus()
      .then((s) => setHasSession(s.method === "session"))
      .catch(() => setHasSession(false));
  }, []);
  if (!hasSession) return null;
  return (
    <button
      type="button"
      onClick={() => void logout()}
      style={{
        display: "flex",
        alignItems: "center",
        gap: "8px",
        padding: "0 12px",
        height: "36px",
        width: "100%",
        color: "var(--color-text-muted)",
        fontSize: "12px",
        background: "none",
        border: "none",
        borderRadius: "6px",
        cursor: "pointer",
        textAlign: "left",
      }}
    >
      <LogOut size={16} strokeWidth={1.5} style={{ flexShrink: 0 }} />
      <span>Log out</span>
    </button>
  );
}

export default function PilotShell() {
  useEffect(() => {
    applyTheme();
  }, []);

  const appName = useAppName();
  const commandPalette = useCommandPalette();

  return (
    <Shell
      appName={appName}
      appIcon={<AppIcon />}
      mainNav={mainNav}
      settingsNav={settingsNav}
      collapsedStorageKey="pilot-sidebar-collapsed"
      sidebarFooterExtras={
        <>
          <HealthIndicator />
          <DocsLink />
          <LogoutButton />
        </>
      }
      overlay={
        <>
          {commandPalette.isOpen && <CommandPalette onClose={commandPalette.close} />}
          <UpdatedNotice app="Pilot" />
        </>
      }
    />
  );
}
