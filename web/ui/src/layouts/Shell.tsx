import { useEffect } from "react";
import {
  Activity,
  ArrowDownToLine,
  Ban,
  BarChart2,
  Bell,
  BookOpen,
  Bookmark,
  Calendar as CalendarDays,
  Compass,
  Download,
  Film,
  Gauge,
  History,
  KeyRound,
  Layers,
  Library,
  ListPlus,
  MonitorPlay,
  Paintbrush,
  Search,
  Server,
  Settings2,
  SlidersHorizontal,
  LayoutDashboard,
  Tv,
} from "lucide-react";
import Shell, { type NavItem } from "@beacon-shared/Shell";
import { useSystemStatus } from "@/api/system";
import { applyTheme } from "@/theme";
import { useCommandPalette } from "@/components/command-palette/useCommandPalette";
import { CommandPalette } from "@/components/command-palette/CommandPalette";

const mainNav: NavItem[] = [
  { to: "/", icon: LayoutDashboard, label: "Dashboard" },
  { to: "/discover", icon: Compass, label: "Discover" },
  { to: "/activity", icon: Activity, label: "Activity" },
  { to: "/calendar", icon: CalendarDays, label: "Calendar" },
  { to: "/wanted", icon: Bookmark, label: "Wanted" },
  { to: "/stats", icon: BarChart2, label: "Statistics" },
  { to: "/queue", icon: Download, label: "Queue" },
  { to: "/history", icon: History, label: "History" },
];

const settingsNav: NavItem[] = [
  { to: "/settings/libraries", icon: Library, label: "Libraries" },
  { to: "/settings/media-management", icon: Film, label: "Media Management" },
  { to: "/settings/quality-profiles", icon: SlidersHorizontal, label: "Quality Profiles" },
  { to: "/settings/quality-definitions", icon: Gauge, label: "Quality Definitions" },
  { to: "/settings/custom-formats", icon: Layers, label: "Custom Formats" },
  { to: "/settings/indexers", icon: Search, label: "Indexers" },
  { to: "/settings/download-clients", icon: Settings2, label: "Download Clients" },
  { to: "/settings/notifications", icon: Bell, label: "Notifications" },
  { to: "/settings/media-servers", icon: MonitorPlay, label: "Media Servers" },
  { to: "/settings/import-lists", icon: ListPlus, label: "Import Lists" },
  { to: "/settings/blocklist", icon: Ban, label: "Blocklist" },
  { to: "/settings/import", icon: ArrowDownToLine, label: "Import" },
  { to: "/settings/providers", icon: KeyRound, label: "Providers" },
  { to: "/settings/system", icon: Server, label: "System" },
  { to: "/settings/app", icon: Paintbrush, label: "App Settings" },
];

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
      sidebarFooterExtras={<DocsLink />}
      overlay={
        commandPalette.isOpen && (
          <CommandPalette onClose={commandPalette.close} />
        )
      }
    />
  );
}
