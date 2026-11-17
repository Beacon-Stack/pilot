import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import ManualImportPage, { initialChoice } from "./ManualImportPage";
import type { ManualCandidate } from "@/api/manual-import";

const q = { resolution: "1080p", source: "webdl", codec: "x264", hdr: "none", name: "WEBDL-1080p" };

const good: ManualCandidate = {
  path: "/in/Show.S01E01.1080p.WEB.mkv",
  size: 1_000_000,
  series_id: "s1",
  series_title: "Show",
  season: 1,
  episodes: [{ id: "e1", season_number: 1, episode_number: 1, title: "Pilot" }],
  quality: q as ManualCandidate["quality"],
  languages: [],
  rejections: [],
};
const unknown: ManualCandidate = { ...good, path: "/in/Mystery.mkv", series_id: "", series_title: "", season: 0, episodes: [], rejections: ["unknown_series"] };

describe("initialChoice", () => {
  it("includes clean matches and leaves out rejected or unmatched files", () => {
    expect(initialChoice(good)).toEqual({ include: true, seriesId: "s1", season: 1, episodeIds: ["e1"] });
    expect(initialChoice(unknown).include).toBe(false);
    expect(initialChoice({ ...good, rejections: ["sample"] }).include).toBe(false);
    expect(initialChoice({ ...good, rejections: ["existing_file_is_equal_or_better"] }).include).toBe(false);
  });
});

describe("ManualImportPage", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("scans a folder, lets a row be fixed, and imports the chosen files", async () => {
    const calls: { url: string; body?: string }[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string, init?: RequestInit) => {
        calls.push({ url, body: init?.body as string | undefined });
        if (url.endsWith("/manual-import/scan")) return new Response(JSON.stringify({ candidates: [good, unknown] }), { status: 200 });
        if (url.endsWith("/manual-import")) {
          const body = JSON.parse(String(init!.body));
          return new Response(JSON.stringify({ results: body.items.map((it: { path: string }) => ({ path: it.path, ok: true })) }), { status: 200 });
        }
        if (url.includes("/series?page=")) {
          return new Response(JSON.stringify({ series: [{ id: "s1", title: "Show", sort_title: "show" }, { id: "s2", title: "Mystery Show", sort_title: "mystery show" }], total: 2 }), { status: 200 });
        }
        if (url.includes("/series/s2/seasons/1/episodes")) {
          return new Response(JSON.stringify([{ id: "m1", season_number: 1, episode_number: 1, title: "Clue" }]), { status: 200 });
        }
        if (url.includes("/seasons")) return new Response(JSON.stringify([{ id: "x", season_number: 1 }]), { status: 200 });
        return new Response(JSON.stringify([]), { status: 200 });
      }),
    );
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <MemoryRouter>
          <ManualImportPage />
        </MemoryRouter>
      </QueryClientProvider>,
    );
    const user = userEvent.setup();
    await user.type(screen.getByLabelText("Folder"), "/in");
    await user.click(screen.getByRole("button", { name: "Scan" }));

    expect(await screen.findByText("Series not recognized")).toBeInTheDocument();
    expect(screen.getByLabelText("Import Show.S01E01.1080p.WEB.mkv")).toBeChecked();
    expect(screen.getByLabelText("Import Mystery.mkv")).not.toBeChecked();
    expect(screen.getByRole("button", { name: "Import 1 file" })).toBeEnabled();

    // Fix the unmatched row: pick its series and episode.
    await user.selectOptions(await screen.findByLabelText("Series for Mystery.mkv"), "s2");
    const episodeSelects = await screen.findAllByLabelText("Episode");
    await waitFor(() => expect(screen.getAllByRole("option", { name: /Clue/ }).length).toBeGreaterThan(0));
    await user.selectOptions(episodeSelects[1], "m1");

    await user.selectOptions(screen.getByLabelText("Import mode"), "move");
    await user.click(screen.getByRole("button", { name: "Import 2 files" }));

    await waitFor(() => expect(calls.some((c) => c.url.endsWith("/api/v1/manual-import"))).toBe(true));
    const body = JSON.parse(calls.find((c) => c.url.endsWith("/api/v1/manual-import"))!.body!);
    expect(body.mode).toBe("move");
    expect(body.items.map((i: { path: string; series_id: string; episode_ids: string[] }) => [i.path, i.series_id, i.episode_ids])).toEqual([
      ["/in/Show.S01E01.1080p.WEB.mkv", "s1", ["e1"]],
      ["/in/Mystery.mkv", "s2", ["m1"]],
    ]);
    expect(await screen.findAllByText("Imported")).toHaveLength(2);
  });
});
