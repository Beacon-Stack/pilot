import { describe, expect, it } from "vitest";
import { mainNav, settingsNav } from "@/layouts/nav";
import { NAV_COMMANDS } from "./commands";

describe("command palette pages", () => {
  // The palette lists the sidebar's pages in the sidebar's order (from
  // layouts/nav.ts), so the two can't drift apart.
  it("matches the sidebar", () => {
    const sidebar = [...mainNav, ...settingsNav].map((n) => n.label);
    expect(NAV_COMMANDS.map((c) => c.label)).toEqual(sidebar);
    expect(new Set(NAV_COMMANDS.map((c) => c.id)).size).toBe(NAV_COMMANDS.length);
  });

  it("starts with the order Pilot and Prism share", () => {
    expect(mainNav.slice(0, 8).map((n) => n.label)).toEqual(["Dashboard", "Discover", "Activity", "Calendar", "Wanted", "Statistics", "Queue", "History"]);
  });
});
