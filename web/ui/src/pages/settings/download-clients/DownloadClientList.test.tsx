import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { ConfirmProvider } from "@beacon-shared/ConfirmDialog";
import DownloadClientList from "./DownloadClientList";

describe("DownloadClientList", () => {
  afterEach(() => vi.unstubAllGlobals());

  // The page used to call /download-handling endpoints that never existed.
  it("calls no missing endpoints and links to remote path mappings", async () => {
    const urls: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string) => {
        urls.push(url);
        return new Response(JSON.stringify([]), { status: 200 });
      }),
    );
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <ConfirmProvider>
          <MemoryRouter>
            <DownloadClientList />
          </MemoryRouter>
        </ConfirmProvider>
      </QueryClientProvider>,
    );
    expect(await screen.findByText("How downloads are handled")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "remote path mapping" })).toHaveAttribute("href", "/settings/remote-path-mappings");
    expect(urls.length).toBeGreaterThan(0);
    expect(urls.filter((u) => u.includes("/download-handling"))).toEqual([]);
  });
});
