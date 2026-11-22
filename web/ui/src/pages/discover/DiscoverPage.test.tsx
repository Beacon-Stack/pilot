import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router-dom";
import DiscoverPage from "./DiscoverPage";

const list = {
  results: [
    { tmdb_id: 1, title: "Severance", year: 2022, overview: "", poster_path: "/s.jpg", rating: 8.4 },
    { tmdb_id: 2, title: "Andor", year: 2022, overview: "", poster_path: "/a.jpg", rating: 8.3 },
  ],
  page: 1,
  total_pages: 3,
};

let calls: URL[] = [];

function renderPage() {
  calls = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: string) => {
      const url = new URL(input, "http://pilot.test");
      calls.push(url);
      const json = (b: unknown) => new Response(JSON.stringify(b), { status: 200 });
      if (url.pathname.endsWith("/discover/browse")) return json(list);
      if (url.pathname.endsWith("/discover/search")) return json({ results: [{ ...list.results[0], tmdb_id: 9, title: "The Bear" }], page: 1, total_pages: 1 });
      if (url.pathname.endsWith("/discover/providers")) return json([{ id: 8, name: "Netflix", logo_path: "" }]);
      if (url.pathname.endsWith("/discover/genres")) return json([{ id: 18, name: "Drama" }]);
      if (url.pathname.endsWith("/series/tmdb-ids")) return json([2]);
      return json([]);
    }),
  );
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <BrowserRouter>
        <DiscoverPage />
      </BrowserRouter>
    </QueryClientProvider>,
  );
}

const browseCalls = () => calls.filter((u) => u.pathname.endsWith("/discover/browse"));

beforeEach(() => window.history.replaceState({}, "", "/discover"));
afterEach(() => vi.unstubAllGlobals());

describe("Pilot Discover", () => {
  it("has the same search, lists and filters as Prism", async () => {
    renderPage();
    expect(screen.getByRole("searchbox", { name: "Search TV series" })).toBeInTheDocument();
    for (const name of ["Trending", "Popular", "Top rated", "New on streaming", "Coming soon"]) {
      expect(screen.getByRole("button", { name })).toBeInTheDocument();
    }
    expect(await screen.findByText("Severance")).toBeInTheDocument();
    expect(await screen.findByRole("option", { name: "Netflix" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "Drama" })).toBeInTheDocument();
  });

  it("asks for New on streaming with filters", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByText("Severance");
    await screen.findByRole("option", { name: "Netflix" });
    await user.click(screen.getByRole("button", { name: "New on streaming" }));
    await user.selectOptions(screen.getByRole("combobox", { name: "Streaming service" }), "Netflix");
    await user.selectOptions(screen.getByRole("combobox", { name: "Genre" }), "Drama");
    await waitFor(() => {
      const last = browseCalls().at(-1)!.searchParams;
      expect([last.get("category"), last.get("provider"), last.get("genre")]).toEqual(["new", "8", "18"]);
    });
  });

  it("searches by title with the Discover search endpoint", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByText("Severance");
    await user.type(screen.getByRole("searchbox", { name: "Search TV series" }), "the bear");
    expect(await screen.findByText("The Bear")).toBeInTheDocument();
    expect(calls.some((u) => u.pathname.endsWith("/discover/search") && u.searchParams.get("q") === "the bear")).toBe(true);
  });
});
