import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import SystemSettings from "./SystemSettings";

function renderPage(updates: object) {
  const fetchMock = vi.fn(async (url: string) => {
    if (url.endsWith("/system/updates")) return new Response(JSON.stringify(updates), { status: 200 });
    if (url.endsWith("/system/status"))
      return new Response(
        JSON.stringify({ app_name: "Pilot", version: "1111111", build_time: "", go_version: "go1.25", db_type: "sqlite", uptime_seconds: 60 }),
        { status: 200 },
      );
    if (url.endsWith("/system/health")) return new Response(JSON.stringify({ issues: [] }), { status: 200 });
    return new Response(JSON.stringify({}), { status: 200 });
  });
  vi.stubGlobal("fetch", fetchMock);
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter>
        <SystemSettings />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return fetchMock;
}

afterEach(() => vi.unstubAllGlobals());

describe("SystemSettings updates", () => {
  it("checks Pilot's own endpoint and shows the newer commits", async () => {
    const fetchMock = renderPage({
      update_available: true,
      current_version: "1111111",
      latest_version: "2222222",
      behind: 1,
      commits: [{ sha: "2222222", subject: "fix: something", url: "https://github.com/beacon-stack/pilot/commit/2222222" }],
    });
    const user = userEvent.setup();
    await user.click(await screen.findByText("Check for updates"));
    await waitFor(() => expect(screen.getByText("Pilot update available")).toBeInTheDocument());
    expect(screen.getByText("fix: something")).toBeInTheDocument();
    expect(screen.getByText(/docker compose pull pilot/)).toBeInTheDocument();
    expect(fetchMock.mock.calls.some(([u]) => String(u) === "/api/v1/system/updates")).toBe(true);
  });

  it("says up to date", async () => {
    renderPage({ update_available: false, current_version: "2222222", latest_version: "2222222" });
    const user = userEvent.setup();
    await user.click(await screen.findByText("Check for updates"));
    await waitFor(() => expect(screen.getByText(/Up to date/)).toBeInTheDocument());
  });
});
