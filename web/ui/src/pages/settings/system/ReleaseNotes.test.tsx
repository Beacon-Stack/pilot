import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import UpdateCheck from "@beacon-shared/UpdateCheck";
import Markdown from "@beacon-shared/Markdown";

const notes014 = [
  "Faster search.",
  "",
  "## New features",
  "",
  "- **Search:** Stream results ([#88](https://github.com/beacon-stack/pilot/pull/88))",
  "",
  "<details>",
  "<summary>Under the hood (1)</summary>",
  "",
  "- **CI:** Newest build wins",
  "",
  "</details>",
].join("\n");

const releases = [
  { tag: "v0.14.0", name: "v0.14.0", published_at: "2026-12-01T00:00:00Z", url: "https://github.com/beacon-stack/pilot/releases/tag/v0.14.0", notes: notes014 },
  { tag: "v0.13.0", name: "v0.13.0", published_at: "2026-11-21T00:00:00Z", url: "https://github.com/beacon-stack/pilot/releases/tag/v0.13.0", notes: "- Older change" },
];

function stubFetch(routes: Record<string, unknown>) {
  const fetchMock = vi.fn(async (url: string) => {
    const hit = Object.entries(routes).find(([path]) => String(url).includes(path));
    return new Response(JSON.stringify(hit ? hit[1] : {}), { status: 200 });
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

afterEach(() => vi.unstubAllGlobals());

describe("Release notes button", () => {
  it("lists recent releases with the newest open and its notes rendered", async () => {
    const fetchMock = stubFetch({ "/system/releases": { releases } });
    render(<UpdateCheck app="Pilot" service="pilot" image="ghcr.io/beacon-stack/pilot" />);
    await userEvent.setup().click(screen.getByText("Release notes"));

    await waitFor(() => expect(screen.getByText("Pilot release notes")).toBeInTheDocument());
    expect(fetchMock.mock.calls.some(([u]) => String(u) === "/api/v1/system/releases?limit=10")).toBe(true);
    expect(screen.getByText("Faster search.")).toBeInTheDocument();
    expect(screen.getByText("New features")).toBeInTheDocument();
    expect(screen.getByText("Search:")).toBeInTheDocument(); // bold scope
    expect(screen.getByText("#88")).toHaveAttribute("href", "https://github.com/beacon-stack/pilot/pull/88");
    // Newest open, older folded.
    const folds = screen.getAllByText(/^v0\.1[34]\.0$/).map((el) => el.closest("details"));
    expect(folds[0]).toHaveAttribute("open");
    expect(folds[1]).not.toHaveAttribute("open");
  });

  it("says so when nothing is published", async () => {
    stubFetch({ "/system/releases": { releases: [] } });
    render(<UpdateCheck app="Pilot" service="pilot" image="ghcr.io/beacon-stack/pilot" />);
    await userEvent.setup().click(screen.getByText("Release notes"));
    await waitFor(() => expect(screen.getByText("No releases have been published yet.")).toBeInTheDocument());
  });
});

describe("Update window", () => {
  it("shows the notes of every release the update brings", async () => {
    stubFetch({
      "/system/updates": {
        update_available: true,
        current_version: "0.12.0",
        latest_version: "v0.14.0",
        release_notes: notes014,
        releases,
      },
    });
    render(<UpdateCheck app="Pilot" service="pilot" image="ghcr.io/beacon-stack/pilot" />);
    await userEvent.setup().click(screen.getByText("Check for updates"));

    await waitFor(() => expect(screen.getByText("Pilot update available")).toBeInTheDocument());
    expect(screen.getByText(/Release notes \(2 releases to catch up on\)/)).toBeInTheDocument();
    expect(screen.getByText("Older change")).toBeInTheDocument();
    expect(screen.getByText(/docker compose pull pilot/)).toBeInTheDocument();
  });
});

describe("Markdown", () => {
  it("never renders markup from the notes, and only links https", () => {
    const { container } = render(
      <Markdown text={'<img src=x onerror="alert(1)"> and [bad](javascript:alert(1)) and [good](https://example.com)'} />,
    );
    expect(container.querySelector("img")).toBeNull();
    expect(screen.getByText(/<img src=x/)).toBeInTheDocument();
    const links = Array.from(container.querySelectorAll("a")).map((a) => a.getAttribute("href"));
    expect(links).toEqual(["https://example.com"]);
  });

  it("renders code blocks and folds", () => {
    const { container } = render(
      <Markdown text={"```sh\ndocker compose pull pilot\n```\n\n<details>\n<summary>More</summary>\n\n- hidden\n\n</details>"} />,
    );
    expect(container.querySelector("pre")?.textContent).toBe("docker compose pull pilot");
    expect(container.querySelector("details summary")?.textContent).toBe("More");
    expect(container.querySelector("details li")?.textContent).toBe("hidden");
  });
});
