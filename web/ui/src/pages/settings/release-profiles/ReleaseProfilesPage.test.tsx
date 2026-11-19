import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { ConfirmProvider } from "@beacon-shared/ConfirmDialog";
import ReleaseProfilesPage, { parseTerms } from "./ReleaseProfilesPage";

type Call = { url: string; init?: RequestInit };

function mockApi(profiles: unknown[]) {
  const calls: Call[] = [];
  const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
    calls.push({ url, init });
    if (url.endsWith("/release-profiles") && (!init?.method || init.method === "GET")) {
      return new Response(JSON.stringify(profiles), { status: 200 });
    }
    if (url.endsWith("/indexers")) {
      return new Response(JSON.stringify([{ id: "idx-1", name: "Tracker A" }]), { status: 200 });
    }
    return new Response(JSON.stringify({ id: "new" }), { status: 201 });
  });
  vi.stubGlobal("fetch", fetchMock);
  return calls;
}

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <ConfirmProvider>
        <MemoryRouter>
          <ReleaseProfilesPage />
        </MemoryRouter>
      </ConfirmProvider>
    </QueryClientProvider>,
  );
}

describe("ReleaseProfilesPage", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("parses one term per line and drops blanks", () => {
    expect(parseTerms(" CAM \n\n/\\bHC\\b/\n")).toEqual(["CAM", "/\\bHC\\b/"]);
  });

  it("lists profiles with their terms and indexer", async () => {
    mockApi([
      {
        id: "rp-1",
        name: "No junk",
        enabled: true,
        required: [],
        ignored: ["CAM", "HDCAM"],
        indexer_id: "idx-1",
        created_at: "",
        updated_at: "",
      },
    ]);
    renderPage();
    expect(await screen.findByText("No junk")).toBeInTheDocument();
    expect(screen.getByText("HDCAM")).toBeInTheDocument();
    expect(await screen.findByText("Tracker A")).toBeInTheDocument();
  });

  it("creates a profile with the API's field names", async () => {
    const calls = mockApi([]);
    renderPage();
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: /Add release profile/ }));
    await user.type(screen.getByLabelText("Name"), "No CAM");
    await user.type(screen.getByLabelText(/Must not contain/), "CAM{enter}{enter}TS");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(calls.some((c) => c.init?.method === "POST")).toBe(true));
    const post = calls.find((c) => c.init?.method === "POST")!;
    expect(post.url).toMatch(/\/api\/v1\/release-profiles$/);
    expect(JSON.parse(String(post.init!.body))).toEqual({
      name: "No CAM",
      enabled: true,
      required: [],
      ignored: ["CAM", "TS"],
      indexer_id: "",
    });
  });
});
