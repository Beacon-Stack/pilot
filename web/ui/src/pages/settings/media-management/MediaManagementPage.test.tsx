import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { ConfirmProvider } from "@beacon-shared/ConfirmDialog";
import MediaManagementPage from "./MediaManagementPage";

const settings = {
  rename_episodes: true,
  standard_episode_format: "{Series Title} - S{Season:00}E{Episode:00}",
  daily_episode_format: "{Series Title} - {Air-Date}",
  anime_episode_format: "{Series Title} - S{Season:00}E{Episode:00}",
  series_folder_format: "{Series Title}",
  season_folder_format: "Season {Season:00}",
  colon_replacement: "space-dash",
  import_extra_files: false,
  extra_file_extensions: "srt",
  unmonitor_deleted_episodes: false,
  recycle_bin_path: "",
  recycle_bin_cleanup_days: 7,
};

type Call = { url: string; method?: string; body?: string };

function renderPage(overrides: Partial<typeof settings> = {}) {
  const calls: Call[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init?: RequestInit) => {
      calls.push({ url, method: init?.method, body: init?.body as string | undefined });
      if (url.endsWith("/recycle-bin/empty")) return new Response(JSON.stringify({ deleted: 2 }), { status: 200 });
      return new Response(init?.body ?? JSON.stringify({ ...settings, ...overrides }), { status: 200 });
    }),
  );
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ConfirmProvider>
        <MemoryRouter>
          <MediaManagementPage />
        </MemoryRouter>
      </ConfirmProvider>
    </QueryClientProvider>,
  );
  return calls;
}

describe("MediaManagementPage recycle bin", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("saves the recycle bin folder and cleanup period", async () => {
    const calls = renderPage();
    const user = userEvent.setup();
    await user.type(await screen.findByLabelText("Recycle Bin"), "/data/recycle");
    const days = screen.getByLabelText("Recycle Bin Cleanup (days)");
    await user.clear(days);
    await user.type(days, "30");
    await user.click(screen.getByRole("button", { name: "Save Changes" }));

    await waitFor(() => expect(calls.some((c) => c.method === "PUT")).toBe(true));
    const body = JSON.parse(calls.find((c) => c.method === "PUT")!.body!);
    expect(body.recycle_bin_path).toBe("/data/recycle");
    expect(body.recycle_bin_cleanup_days).toBe(30);
  });

  it("empties the recycle bin only after confirming", async () => {
    const calls = renderPage({ recycle_bin_path: "/data/recycle" });
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Empty recycle bin" }));
    expect(calls.some((c) => c.url.endsWith("/recycle-bin/empty"))).toBe(false);
    await user.click(await screen.findByRole("button", { name: "Empty" }));
    await waitFor(() => expect(calls.some((c) => c.method === "POST" && c.url.endsWith("/api/v1/recycle-bin/empty"))).toBe(true));
  });
});
