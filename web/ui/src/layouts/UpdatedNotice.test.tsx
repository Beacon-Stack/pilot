import { afterEach, describe, expect, it, vi } from "vitest";
import { act, render, screen, waitFor } from "@testing-library/react";
import UpdatedNotice from "@beacon-shared/UpdatedNotice";

function serverVersions(...answers: (string | Error)[]) {
  let i = 0;
  const fetchMock = vi.fn(async () => {
    const a = answers[Math.min(i++, answers.length - 1)];
    if (a instanceof Error) throw a;
    return new Response(JSON.stringify({ version: a }), { status: 200 });
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function tabComesBack() {
  act(() => {
    document.dispatchEvent(new Event("visibilitychange"));
  });
}

afterEach(() => vi.unstubAllGlobals());

describe("UpdatedNotice", () => {
  it("offers a reload once the server's version changes", async () => {
    const fetchMock = serverVersions("aaaaaaa", "bbbbbbb");
    render(<UpdatedNotice app="Pilot" />);
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(screen.queryByText(/was updated/)).toBeNull();

    tabComesBack();
    await waitFor(() => expect(screen.getByText("Pilot was updated. Reload to get the latest.")).toBeInTheDocument());
    expect(screen.getByRole("button", { name: "Reload" })).toBeInTheDocument();
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/system/status");
  });

  it("stays quiet while the version holds, and through a failed check", async () => {
    const fetchMock = serverVersions("aaaaaaa", new Error("restarting"), "aaaaaaa");
    render(<UpdatedNotice app="Pilot" />);
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    tabComesBack();
    tabComesBack();
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(3));
    expect(screen.queryByText(/was updated/)).toBeNull();
  });

  it("reads the version from another endpoint when told to", async () => {
    const fetchMock = serverVersions("aaaaaaa");
    render(<UpdatedNotice app="Haul" versionPath="/stats" />);
    await waitFor(() => expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/stats"));
  });
});
