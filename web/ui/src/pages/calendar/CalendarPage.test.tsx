import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import CalendarPage from "./CalendarPage";

// What GET /api/v1/calendar returns: episode_id and episode_title, not id
// and title.
const episode = (id: string, n: number, title: string, day: number) => {
  const now = new Date();
  const month = String(now.getMonth() + 1).padStart(2, "0");
  return {
    episode_id: id,
    series_id: "series-1",
    series_title: "The Lord of the Rings: The Rings of Power",
    season_number: 3,
    episode_number: n,
    episode_title: title,
    air_date: `${now.getFullYear()}-${month}-${String(day).padStart(2, "0")}`,
    has_file: false,
    monitored: true,
  };
};

function renderPage() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string) => {
      if (url.includes("/calendar?")) {
        return new Response(
          JSON.stringify([episode("ep-5", 5, "The Lady of Lórien", 15), episode("ep-6", 6, "From Valinor", 15)]),
          { status: 200 },
        );
      }
      return new Response(JSON.stringify({}), { status: 200 });
    }),
  );
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter>
        <CalendarPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

afterEach(() => vi.unstubAllGlobals());

describe("CalendarPage", () => {
  // Two episodes of one series on one day used to look like the same
  // entry twice: the episode code sat at the end of the title and was cut
  // off with it.
  it("keeps two same-day episodes of a series distinguishable", async () => {
    renderPage();
    const five = await screen.findByText("S03E05");
    const six = screen.getByText("S03E06");
    // Each code is its own element that doesn't shrink, beside a title
    // that can be cut short.
    for (const code of [five, six]) {
      expect(code.style.flexShrink).toBe("0");
      expect(code.textContent).not.toContain("Rings");
    }
    await waitFor(() => expect(screen.getAllByText("The Lord of the Rings: The Rings of Power")).toHaveLength(2));
  });

  it("names the episode in the tooltip", async () => {
    renderPage();
    const five = await screen.findByText("S03E05");
    expect(five.closest("button")?.title).toBe(
      "The Lord of the Rings: The Rings of Power — S03E05 The Lady of Lórien",
    );
    expect(screen.getByText("S03E06").closest("button")?.title).toContain("From Valinor");
  });
});
