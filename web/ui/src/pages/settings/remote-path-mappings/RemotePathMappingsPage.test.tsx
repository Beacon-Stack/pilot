import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { ConfirmProvider } from "@beacon-shared/ConfirmDialog";
import RemotePathMappingsPage from "./RemotePathMappingsPage";

type Call = { url: string; init?: RequestInit };

function mockApi(mappings: unknown[]) {
  const calls: Call[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init?: RequestInit) => {
      calls.push({ url, init });
      if (url.endsWith("/remote-path-mappings") && (!init?.method || init.method === "GET")) {
        return new Response(JSON.stringify(mappings), { status: 200 });
      }
      if (url.endsWith("/download-clients")) {
        return new Response(JSON.stringify([{ id: "dc-1", name: "qBittorrent" }]), { status: 200 });
      }
      return new Response(JSON.stringify({ id: "new" }), { status: 201 });
    }),
  );
  return calls;
}

function renderPage() {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ConfirmProvider>
        <MemoryRouter>
          <RemotePathMappingsPage />
        </MemoryRouter>
      </ConfirmProvider>
    </QueryClientProvider>,
  );
}

describe("RemotePathMappingsPage", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("lists mappings with their client", async () => {
    mockApi([{ id: "m1", download_client_id: "dc-1", remote_path: "/downloads", local_path: "/mnt/media/downloads" }]);
    renderPage();
    expect(await screen.findByText("/downloads")).toBeInTheDocument();
    expect(screen.getByText("/mnt/media/downloads")).toBeInTheDocument();
    expect(await screen.findByText("qBittorrent")).toBeInTheDocument();
  });

  it("adds a mapping", async () => {
    const calls = mockApi([]);
    renderPage();
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: /Add mapping/ }));
    await user.type(screen.getByLabelText(/Remote path/), "/downloads");
    await user.type(screen.getByLabelText(/Local path/), "/mnt/dl");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(calls.some((c) => c.init?.method === "POST")).toBe(true));
    const post = calls.find((c) => c.init?.method === "POST")!;
    expect(post.url).toMatch(/\/api\/v1\/remote-path-mappings$/);
    expect(JSON.parse(String(post.init!.body))).toEqual({ download_client_id: "", remote_path: "/downloads", local_path: "/mnt/dl" });
  });
});
