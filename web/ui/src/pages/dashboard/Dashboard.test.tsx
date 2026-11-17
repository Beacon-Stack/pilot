import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import Dashboard from "./Dashboard";

const mk = (id: string, title: string) => ({
  id,
  tmdb_id: 1,
  title,
  sort_title: title,
  year: 2020,
  overview: "",
  genres: [],
  status: "continuing",
  series_type: "standard",
  monitor_type: "all",
  monitored: true,
  library_id: "lib",
  quality_profile_id: "qp",
  added_at: "",
  updated_at: "",
  episode_count: 0,
  episode_file_count: 0,
});

type Call = { url: string; method?: string; body?: string };

function renderPage() {
  const calls: Call[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init?: RequestInit) => {
      calls.push({ url, method: init?.method, body: init?.body as string | undefined });
      if (url.includes("/series?page=")) {
        return new Response(JSON.stringify({ series: [mk("a", "Alpha"), mk("b", "Bravo"), mk("c", "Charlie")], total: 3, page: 1, per_page: 200 }), { status: 200 });
      }
      if (url.endsWith("/quality-profiles")) return new Response(JSON.stringify([{ id: "qp2", name: "UHD" }]), { status: 200 });
      if (url.endsWith("/libraries")) return new Response(JSON.stringify([]), { status: 200 });
      if (url.includes("/series/bulk")) return new Response(JSON.stringify({ count: 2 }), { status: 200 });
      if (url.includes("/stats/collection")) return new Response(JSON.stringify({ total_series: 3, with_file: 0, missing: 0 }), { status: 200 });
      if (url.includes("/stats/storage")) return new Response(JSON.stringify({ total_bytes: 0 }), { status: 200 });
      return new Response(JSON.stringify({}), { status: 200 });
    }),
  );
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter>
        <Dashboard />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return calls;
}

describe("Series page bulk editing", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("selects series and applies a change to exactly those", async () => {
    const calls = renderPage();
    const user = userEvent.setup();
    await screen.findAllByText("Alpha");
    expect(screen.queryByRole("toolbar", { name: "Selected series" })).toBeNull();

    await user.click(screen.getByRole("button", { name: "Select" }));
    await user.click(screen.getByRole("checkbox", { name: "Select Alpha" }));
    await user.click(screen.getByRole("checkbox", { name: "Select Charlie" }));
    expect(screen.getByRole("toolbar", { name: "Selected series" })).toHaveTextContent("2 series");

    await user.click(screen.getByRole("button", { name: /Unmonitor/ }));
    await waitFor(() => expect(calls.some((c) => c.method === "PUT")).toBe(true));
    const put = calls.find((c) => c.method === "PUT")!;
    expect(put.url).toMatch(/\/api\/v1\/series\/bulk$/);
    expect(JSON.parse(put.body!)).toEqual({ ids: ["a", "c"], monitored: false });

    await user.selectOptions(screen.getByLabelText("Set quality profile"), "qp2");
    await waitFor(() => expect(calls.filter((c) => c.method === "PUT")).toHaveLength(2));
    expect(JSON.parse(calls.filter((c) => c.method === "PUT")[1].body!)).toEqual({ ids: ["a", "c"], quality_profile_id: "qp2" });
  });

  it("deletes the selection, optionally with files, after confirming", async () => {
    const calls = renderPage();
    const user = userEvent.setup();
    await screen.findAllByText("Alpha");
    await user.click(screen.getByRole("button", { name: "Select" }));
    await user.click(screen.getByRole("button", { name: "Select all" }));
    await user.click(screen.getByRole("button", { name: "Delete…" }));
    await user.click(screen.getByLabelText(/Also delete their episode files/));
    expect(calls.some((c) => c.url.includes("/bulk/delete"))).toBe(false);
    await user.click(screen.getByRole("button", { name: "Delete" }));

    await waitFor(() => expect(calls.some((c) => c.url.endsWith("/series/bulk/delete"))).toBe(true));
    const del = calls.find((c) => c.url.endsWith("/series/bulk/delete"))!;
    expect(JSON.parse(del.body!)).toEqual({ ids: ["a", "b", "c"], delete_files: true });
  });
});
