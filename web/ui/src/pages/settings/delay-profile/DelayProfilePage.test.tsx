import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import DelayProfilePage, { validateDelayProfile } from "./DelayProfilePage";
import PendingReleases from "@/pages/queue/PendingReleases";
import { formatWait } from "@/api/delay";
import type { DelayProfile } from "@/types";

const profile: DelayProfile = {
  usenet_enabled: true,
  torrent_enabled: true,
  preferred_protocol: "usenet",
  usenet_delay_minutes: 0,
  torrent_delay_minutes: 0,
  bypass_if_highest_quality: true,
};

type Call = { url: string; init?: RequestInit };

function stubFetch(handler: (url: string, init?: RequestInit) => unknown) {
  const calls: Call[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init?: RequestInit) => {
      calls.push({ url, init });
      const body = handler(url, init);
      return body === undefined ? new Response(null, { status: 204 }) : new Response(JSON.stringify(body), { status: 200 });
    }),
  );
  return calls;
}

function wrap(ui: React.ReactNode) {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter>{ui}</MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("formatWait", () => {
  const now = new Date("2026-11-17T12:00:00Z");
  it("formats minutes and hours", () => {
    expect(formatWait("2026-11-17T12:45:00Z", now)).toBe("in 45 min");
    expect(formatWait("2026-11-17T14:05:00Z", now)).toBe("in 2 h 5 min");
    expect(formatWait("2026-11-17T14:00:00Z", now)).toBe("in 2 h");
  });
  it("says next RSS sync once the time has passed", () => {
    expect(formatWait("2026-11-17T11:00:00Z", now)).toBe("next RSS sync");
  });
});

describe("validateDelayProfile", () => {
  it("needs a protocol and sane delays", () => {
    expect(validateDelayProfile(profile)).toBeNull();
    expect(validateDelayProfile({ ...profile, usenet_enabled: false, torrent_enabled: false })).toMatch(/protocol/);
    expect(validateDelayProfile({ ...profile, torrent_delay_minutes: -5 })).toMatch(/minutes/);
    expect(validateDelayProfile({ ...profile, usenet_delay_minutes: 1.5 })).toMatch(/minutes/);
  });
});

describe("DelayProfilePage", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("loads the profile and saves edits", async () => {
    const calls = stubFetch((_url, init) => (init?.method === "PUT" ? JSON.parse(String(init.body)) : profile));
    wrap(<DelayProfilePage />);
    const user = userEvent.setup();
    const torrentDelay = await screen.findByLabelText("Torrent delay (minutes)");
    await user.clear(torrentDelay);
    await user.type(torrentDelay, "120");
    await user.selectOptions(screen.getByLabelText(/Preferred protocol/), "torrent");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(calls.some((c) => c.init?.method === "PUT")).toBe(true));
    const put = calls.find((c) => c.init?.method === "PUT")!;
    expect(put.url).toMatch(/\/api\/v1\/delay-profile$/);
    expect(JSON.parse(String(put.init!.body))).toEqual({ ...profile, torrent_delay_minutes: 120, preferred_protocol: "torrent" });
  });
});

describe("PendingReleases", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("renders nothing when nothing is pending", async () => {
    stubFetch(() => []);
    const { container } = wrap(<PendingReleases />);
    await waitFor(() => expect(container).toBeEmptyDOMElement());
  });

  it("lists pending releases and drops one", async () => {
    const calls = stubFetch((_url, init) =>
      init?.method === "DELETE"
        ? undefined
        : [
            {
              id: "p1",
              series_id: "s1",
              series_title: "Severance",
              episode_id: "e1",
              season_number: 2,
              episode_number: 3,
              release_title: "Severance.S02E03.1080p.WEB-DL-GRP",
              protocol: "nzb",
              quality: "WEBDL-1080p",
              eligible_at: "2099-01-01T00:00:00Z",
              added_at: "2026-11-17T00:00:00Z",
            },
          ],
    );
    wrap(<PendingReleases />);
    expect(await screen.findByText("Waiting for delay (1)")).toBeInTheDocument();
    expect(screen.getByText(/WEBDL-1080p · usenet/)).toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole("button", { name: "Drop" }));
    await waitFor(() =>
      expect(calls.some((c) => c.init?.method === "DELETE" && c.url.endsWith("/api/v1/queue/pending/p1"))).toBe(true),
    );
  });
});
