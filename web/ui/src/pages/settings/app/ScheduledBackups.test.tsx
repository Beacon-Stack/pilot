import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ConfirmProvider } from "@beacon-shared/ConfirmDialog";
import ScheduledBackups, { validateBackupSettings } from "./ScheduledBackups";
import { formatBytes } from "@/api/backups";

type Call = { url: string; method?: string; body?: string };

function mockApi() {
  const calls: Call[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init?: RequestInit) => {
      calls.push({ url, method: init?.method, body: init?.body as string | undefined });
      if (url.endsWith("/system/backup-settings")) {
        return new Response(init?.body ?? JSON.stringify({ interval_days: 7, retention_days: 28 }), { status: 200 });
      }
      if (url.endsWith("/system/backups") && (!init?.method || init.method === "GET")) {
        return new Response(
          JSON.stringify({
            folder: "/config/backups",
            backups: [{ name: "pilot-scheduled-20261110-120000.db", kind: "scheduled", size: 2_500_000, created_at: "2026-11-10T12:00:00Z" }],
          }),
          { status: 200 },
        );
      }
      if (url.endsWith("/restore")) return new Response(JSON.stringify({ message: "ok" }), { status: 200 });
      return new Response(JSON.stringify({ name: "pilot-manual-x.db", kind: "manual", size: 1, created_at: "" }), { status: 201 });
    }),
  );
  return calls;
}

function renderIt() {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ConfirmProvider>
        <ScheduledBackups />
      </ConfirmProvider>
    </QueryClientProvider>,
  );
}

describe("backup helpers", () => {
  it("validates the schedule", () => {
    expect(validateBackupSettings({ interval_days: 0, retention_days: 1 })).toBeNull();
    expect(validateBackupSettings({ interval_days: -1, retention_days: 7 })).toMatch(/interval/);
    expect(validateBackupSettings({ interval_days: 7, retention_days: 0 })).toMatch(/Keep/);
  });
  it("formats sizes", () => {
    expect(formatBytes(512)).toBe("512 B");
    expect(formatBytes(2_500_000)).toBe("2.4 MB");
  });
});

describe("ScheduledBackups", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("lists backups, saves the schedule, backs up now, and restores after confirming", async () => {
    const calls = mockApi();
    renderIt();
    const user = userEvent.setup();

    expect(await screen.findByText(/Scheduled · 2.4 MB/)).toBeInTheDocument();
    expect(screen.getByText("/config/backups")).toBeInTheDocument();
    expect(screen.getByTitle(/Download backup/)).toHaveAttribute("href", "/api/v1/system/backups/pilot-scheduled-20261110-120000.db/download");

    const interval = await screen.findByLabelText("Every (days)");
    await user.clear(interval);
    await user.type(interval, "3");
    await user.click(screen.getByRole("button", { name: "Save schedule" }));
    await waitFor(() => expect(calls.some((c) => c.method === "PUT")).toBe(true));
    expect(JSON.parse(calls.find((c) => c.method === "PUT")!.body!)).toEqual({ interval_days: 3, retention_days: 28 });

    await user.click(screen.getByRole("button", { name: "Back up now" }));
    await waitFor(() => expect(calls.some((c) => c.method === "POST" && c.url.endsWith("/system/backups"))).toBe(true));

    await user.click(screen.getByTitle(/Restore backup/));
    // The confirm dialog's button; the row's button is named "Restore backup from …".
    await user.click(await screen.findByRole("button", { name: "Restore" }));
    await waitFor(() => expect(calls.some((c) => c.url.endsWith("/pilot-scheduled-20261110-120000.db/restore"))).toBe(true));
  });
});
