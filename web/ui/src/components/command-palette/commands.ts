import type { ElementType } from "react";
import type { NavigateFunction } from "react-router-dom";
import { mainNav, settingsNav } from "@/layouts/nav";
import {
  Rss,
  HardDrive,
  RotateCw,
} from "lucide-react";

// Types

type CommandCategory = "navigation" | "series" | "action";

export interface Command {
  id: string;
  category: CommandCategory;
  label: string;
  keywords: string[];
  icon: ElementType;
  onSelect: (navigate: NavigateFunction) => void;
}

// Navigation commands

// Search words and stable IDs for the sidebar's pages, by path. The pages
// themselves (label, icon, order) come from the sidebar's list in
// layouts/nav.ts, so the palette can't drift from the sidebar.
const NAV_EXTRA: Record<string, { id: string; keywords: string[] }> = {
  "/settings/providers": { id: "nav:providers", keywords: ["tmdb", "trakt", "api key", "metadata"] },
  "/settings/system/logs": { id: "nav:logs", keywords: ["log", "debug", "errors"] },
  "/": { id: "nav:dashboard", keywords: ["home", "overview"] },
  "/discover": { id: "nav:discover", keywords: ["search", "add", "find", "new"] },
  "/activity": { id: "nav:activity", keywords: ["events", "timeline", "log"] },
  "/calendar": { id: "nav:calendar", keywords: ["schedule", "upcoming"] },
  "/wanted": { id: "nav:wanted", keywords: ["missing", "cutoff"] },
  "/stats": { id: "nav:stats", keywords: ["graphs", "charts"] },
  "/queue": { id: "nav:queue", keywords: ["downloads", "progress"] },
  "/history": { id: "nav:history", keywords: ["grabs", "past"] },
  "/manual-import": { id: "nav:manual-import", keywords: ["import", "files", "folder", "existing", "disk"] },
  "/settings/libraries": { id: "nav:libraries", keywords: ["settings", "root", "path"] },
  "/settings/media-management": { id: "nav:media-management", keywords: ["settings", "rename", "format"] },
  "/settings/quality-profiles": { id: "nav:quality-profiles", keywords: ["settings", "quality"] },
  "/settings/quality-definitions": { id: "nav:quality-defs", keywords: ["settings", "size", "limits"] },
  "/settings/release-profiles": { id: "nav:release-profiles", keywords: ["settings", "must contain", "ignore", "terms"] },
  "/settings/delay-profile": { id: "nav:delay-profile", keywords: ["settings", "usenet", "torrent", "wait", "protocol"] },
  "/settings/languages": { id: "nav:languages", keywords: ["settings", "language", "audio", "dub", "multi"] },
  "/settings/indexers": { id: "nav:indexers", keywords: ["settings", "torznab", "newznab"] },
  "/settings/download-clients": { id: "nav:download-clients", keywords: ["settings", "qbittorrent", "deluge"] },
  "/settings/remote-path-mappings": { id: "nav:remote-path-mappings", keywords: ["settings", "docker", "container", "path", "download", "mapping"] },
  "/settings/notifications": { id: "nav:notifications", keywords: ["settings", "discord", "webhook"] },
  "/settings/media-servers": { id: "nav:media-servers", keywords: ["settings", "plex", "jellyfin"] },
  "/settings/import-lists": { id: "nav:import-lists", keywords: ["settings", "trakt", "tmdb"] },
  "/settings/blocklist": { id: "nav:blocklist", keywords: ["settings", "block", "ban"] },
  "/settings/import": { id: "nav:import", keywords: ["settings", "sonarr"] },
  "/settings/system": { id: "nav:system", keywords: ["settings", "health", "tasks"] },
  "/settings/app": { id: "nav:app-settings", keywords: ["settings", "theme", "api key"] },
};

// Pages reachable from the palette but not the sidebar.
const PALETTE_ONLY: Command[] = [

];

export const NAV_COMMANDS: Command[] = [
  ...[...mainNav, ...settingsNav].map((item): Command => ({
    id: NAV_EXTRA[item.to]?.id ?? `nav:${item.to.replace(/^\/(settings\/)?/, "").replace(/\//g, "-") || "dashboard"}`,
    category: "navigation",
    label: item.label,
    keywords: NAV_EXTRA[item.to]?.keywords ?? [],
    icon: item.icon,
    onSelect: (n) => n(item.to),
  })),
  ...PALETTE_ONLY,
];

// Action commands

export interface ActionCommand extends Command {
  taskName: string;
}

export const ACTION_COMMANDS: ActionCommand[] = [
  { id: "action:rss-sync",         category: "action", label: "Run RSS Sync",           keywords: ["rss", "feed", "check"],    icon: Rss,        taskName: "rss_sync",         onSelect: () => {} },
  { id: "action:library-scan",     category: "action", label: "Scan All Libraries",     keywords: ["scan", "disk", "files"],   icon: HardDrive,  taskName: "library_scan",     onSelect: () => {} },
  { id: "action:refresh-metadata", category: "action", label: "Refresh All Metadata",   keywords: ["tmdb", "refresh", "sync"], icon: RotateCw,   taskName: "refresh_metadata", onSelect: () => {} },
];

// Fuzzy matching

export function fuzzyScore(query: string, text: string): number {
  const q = query.toLowerCase();
  const t = text.toLowerCase();

  if (!q) return 0;
  if (q.length > t.length) return -1;

  let qi = 0;
  let score = 0;
  let consecutive = 0;

  for (let ti = 0; ti < t.length && qi < q.length; ti++) {
    if (t[ti] === q[qi]) {
      qi++;
      consecutive++;
      score += consecutive;
      if (ti === 0 || /[\s\-_]/.test(t[ti - 1])) score += 10;
    } else {
      consecutive = 0;
    }
  }

  return qi === q.length ? score : -1;
}

// Filter helper

export function filterCommands<T extends Command>(commands: T[], query: string): T[] {
  if (!query) return commands;

  const scored: Array<{ cmd: T; score: number }> = [];

  for (const cmd of commands) {
    const labelScore = fuzzyScore(query, cmd.label);
    const best = labelScore >= 0 ? labelScore + 50 : -1;

    let kwBest = -1;
    for (const kw of cmd.keywords) {
      const s = fuzzyScore(query, kw);
      if (s > kwBest) kwBest = s;
    }

    const score = Math.max(best, kwBest);
    if (score >= 0) scored.push({ cmd, score });
  }

  scored.sort((a, b) => b.score - a.score);
  return scored.map((s) => s.cmd);
}
