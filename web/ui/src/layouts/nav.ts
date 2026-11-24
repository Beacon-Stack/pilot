// The sidebar's pages, shared by the sidebar (Shell.tsx) and the command
// palette (commands.ts), so the two always list the same pages in the
// same order with the same icons.
import {
  Activity,
  ArrowDownToLine,
  Ban,
  BarChart2,
  Bell,
  Bookmark,
  CalendarDays,
  Compass,
  Download,
  FileText,
  Film,
  Filter,
  FolderInput,
  FolderSync,
  Gauge,
  History,
  KeyRound,
  Languages,
  LayoutDashboard,
  Library,
  ListPlus,
  MonitorPlay,
  Paintbrush,
  Search,
  Server,
  Settings2,
  SlidersHorizontal,
  Timer,
} from "lucide-react";
import type { NavItem } from "@beacon-shared/Shell";

// Shared with Pilot/Prism: the same items in the same order, so the two
// apps feel alike; each app's own item goes last.
export const mainNav: NavItem[] = [
  { to: "/", icon: LayoutDashboard, label: "Dashboard" },
  { to: "/discover", icon: Compass, label: "Discover" },
  { to: "/activity", icon: Activity, label: "Activity" },
  { to: "/calendar", icon: CalendarDays, label: "Calendar" },
  { to: "/wanted", icon: Bookmark, label: "Wanted" },
  { to: "/stats", icon: BarChart2, label: "Statistics" },
  { to: "/queue", icon: Download, label: "Queue" },
  { to: "/history", icon: History, label: "History" },
  { to: "/manual-import", icon: FolderInput, label: "Manual Import" },
];

// Shared with Pilot/Prism: the same items in the same order; each app's
// own items sit right after the closest shared one.
export const settingsNav: NavItem[] = [
  { to: "/settings/libraries", icon: Library, label: "Libraries" },
  { to: "/settings/media-management", icon: Film, label: "Media Management" },
  { to: "/settings/quality-profiles", icon: SlidersHorizontal, label: "Quality Profiles" },
  { to: "/settings/quality-definitions", icon: Gauge, label: "Quality Definitions" },
  { to: "/settings/release-profiles", icon: Filter, label: "Release Profiles" },
  { to: "/settings/delay-profile", icon: Timer, label: "Delay Profile" },
  { to: "/settings/languages", icon: Languages, label: "Languages" },
  { to: "/settings/indexers", icon: Search, label: "Indexers" },
  { to: "/settings/download-clients", icon: Settings2, label: "Download Clients" },
  { to: "/settings/remote-path-mappings", icon: FolderSync, label: "Remote Path Mappings" },
  { to: "/settings/notifications", icon: Bell, label: "Notifications" },
  { to: "/settings/media-servers", icon: MonitorPlay, label: "Media Servers" },
  { to: "/settings/import-lists", icon: ListPlus, label: "Import Lists" },
  { to: "/settings/blocklist", icon: Ban, label: "Blocklist" },
  { to: "/settings/import", icon: ArrowDownToLine, label: "Import" },
  { to: "/settings/providers", icon: KeyRound, label: "Providers" },
  { to: "/settings/system", icon: Server, label: "System" },
  { to: "/settings/system/logs", icon: FileText, label: "Logs" },
  { to: "/settings/app", icon: Paintbrush, label: "App Settings" },
];
