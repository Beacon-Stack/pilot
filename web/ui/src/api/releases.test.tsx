import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useSearchReleases } from "./releases";

const release = (title: string) => ({
  guid: title,
  title,
  indexer: "x",
  indexer_id: "x",
  protocol: "torrent",
  download_url: "",
  size: 0,
  quality: { resolution: "1080p", source: "WEB-DL", codec: "", hdr: "", name: "" },
  quality_score: 0,
});

// A response whose body sends each chunk only when the test says so.
function streamedResponse() {
  let push!: (text: string) => void;
  let end!: () => void;
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      const enc = new TextEncoder();
      push = (text) => controller.enqueue(enc.encode(text));
      end = () => controller.close();
    },
  });
  return { response: new Response(body, { status: 200 }), push, end };
}

function wrapper({ children }: { children: ReactNode }) {
  return <QueryClientProvider client={new QueryClient()}>{children}</QueryClientProvider>;
}

afterEach(() => vi.unstubAllGlobals());

describe("useSearchReleases", () => {
  it("shows partial results while indexers are still answering, then the final list", async () => {
    const s = streamedResponse();
    const fetchMock = vi.fn(async () => s.response);
    vi.stubGlobal("fetch", fetchMock);

    const { result } = renderHook(() => useSearchReleases({ seriesId: "s1", season: 1 }, true), { wrapper });

    // One line split across two chunks: parsing must wait for the newline.
    const partial = JSON.stringify({
      releases: [release("Fast.S01")],
      indexers: [{ indexer_id: "slow", indexer_name: "slow", results: 0, searching: true }],
      done: false,
    });
    s.push(partial.slice(0, 20));
    s.push(partial.slice(20) + "\n");
    await waitFor(() => expect(result.current.data?.releases).toHaveLength(1));
    expect(result.current.data?.done).toBe(false);
    expect(result.current.isFetching).toBe(true);

    s.push(JSON.stringify({ releases: [release("Fast.S01"), release("Slow.S01")], done: true }) + "\n");
    s.end();
    await waitFor(() => expect(result.current.isFetching).toBe(false));
    expect(result.current.data?.releases).toHaveLength(2);
    expect(result.current.data?.done).toBe(true);
    expect(fetchMock).toHaveBeenCalledWith("/api/v1/series/s1/releases/stream?season=1", expect.anything());
  });

  it("does not retry a failed search", async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({ title: "boom" }), { status: 500 }));
    vi.stubGlobal("fetch", fetchMock);

    const { result } = renderHook(() => useSearchReleases({ seriesId: "s1", season: 1 }, true), { wrapper });
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(result.current.error?.message).toBe("boom");
  });
});
