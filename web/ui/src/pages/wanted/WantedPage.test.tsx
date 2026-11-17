import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { ConfirmProvider } from "@beacon-shared/ConfirmDialog";
import WantedPage from "./WantedPage";

// Shape returned by GET /api/v1/wanted/missing (wantedEpisodeBody in
// internal/api/v1/wanted.go). The page used to read `title`/`id`, which the
// API never sends, so the episode title column was always blank.
const missing = {
  episodes: [
    {
      episode_id: "ep-1",
      series_id: "s-1",
      series_title: "Severance",
      season_number: 2,
      episode_number: 3,
      episode_title: "Who Is Alive?",
      air_date: "2025-01-31",
      has_file: false,
      monitored: true,
    },
  ],
  total: 1,
  page: 1,
  per_page: 50,
};

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <ConfirmProvider>
        <MemoryRouter>
          <WantedPage />
        </MemoryRouter>
      </ConfirmProvider>
    </QueryClientProvider>,
  );
}

describe("WantedPage", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("shows the episode title from the API response", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(new Response(JSON.stringify(missing), { status: 200 })),
    );
    renderPage();
    expect(await screen.findByText("Who Is Alive?")).toBeInTheDocument();
    expect(screen.getByText("Severance")).toBeInTheDocument();
  });

  it("offers a search-all action", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(new Response(JSON.stringify(missing), { status: 200 })),
    );
    renderPage();
    expect(await screen.findByRole("button", { name: "Search all missing" })).toBeInTheDocument();
  });
});
