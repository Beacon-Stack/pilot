import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import HealthPanel from "./HealthPanel";
import { summarizeHealth } from "@/api/health";
import type { HealthReport } from "@/types";

const sick: HealthReport = {
  issues: [
    { source: "indexer", level: "error", message: 'Can\'t reach indexer "Only": 401' },
    { source: "library", level: "warning", message: 'Library "TV" has 2.0 GB free, below its minimum of 5 GB.' },
  ],
  connectivity_checked_at: "2026-11-17T12:00:00Z",
};

function wrap() {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <HealthPanel />
    </QueryClientProvider>,
  );
}

describe("summarizeHealth", () => {
  it("counts errors and warnings", () => {
    expect(summarizeHealth(sick)).toBe("1 error, 1 warning");
    expect(summarizeHealth({ issues: [], connectivity_checked_at: "" })).toBe("");
    expect(summarizeHealth(undefined)).toBe("");
    const two = { ...sick, issues: [sick.issues[1], sick.issues[1]] };
    expect(summarizeHealth(two)).toBe("2 warnings");
  });
});

describe("HealthPanel", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("lists issues and re-checks on demand", async () => {
    const calls: { url: string; method?: string }[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string, init?: RequestInit) => {
        calls.push({ url, method: init?.method });
        const body = init?.method === "POST" ? { issues: [], connectivity_checked_at: "2026-11-17T13:00:00Z" } : sick;
        return new Response(JSON.stringify(body), { status: 200 });
      }),
    );
    wrap();

    expect(await screen.findByText(/Can't reach indexer "Only": 401/)).toBeInTheDocument();
    expect(screen.getByText(/below its minimum of 5 GB/)).toBeInTheDocument();
    expect(screen.getByText("Unhealthy")).toBeInTheDocument();
    expect(screen.getByText("Indexer")).toBeInTheDocument();

    await userEvent.setup().click(screen.getByRole("button", { name: "Check now" }));
    await waitFor(() => expect(screen.getByText("All systems healthy")).toBeInTheDocument());
    const post = calls.find((c) => c.method === "POST")!;
    expect(post.url).toMatch(/\/api\/v1\/system\/health\/check$/);
  });
});
