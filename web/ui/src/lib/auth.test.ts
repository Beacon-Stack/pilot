import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { installAuthRedirect, safeNextPath } from "./auth";

describe("safeNextPath", () => {
  it("keeps same-app relative paths", () => {
    expect(safeNextPath("/series/12?tab=files")).toBe("/series/12?tab=files");
  });

  it.each([null, "", "https://evil.example/", "//evil.example/x", "evil", "/login", "/login?next=/x"])(
    "falls back to / for %s",
    (next) => {
      expect(safeNextPath(next)).toBe("/");
    },
  );
});

describe("installAuthRedirect", () => {
  const realFetch = window.fetch;
  let assign: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    assign = vi.fn();
    vi.stubGlobal("location", {
      ...window.location,
      origin: "http://localhost:3000",
      pathname: "/settings/indexers",
      search: "?x=1",
      assign,
    });
  });

  afterEach(() => {
    window.fetch = realFetch;
    vi.unstubAllGlobals();
  });

  function install(status: number) {
    window.fetch = vi.fn().mockResolvedValue(new Response(null, { status }));
    installAuthRedirect();
  }

  it("redirects to /login with next on an API 401", async () => {
    install(401);
    await window.fetch("/api/v1/series");
    expect(assign).toHaveBeenCalledWith("/login?next=" + encodeURIComponent("/settings/indexers?x=1"));
  });

  it("does not redirect on a failed login attempt", async () => {
    install(401);
    await window.fetch("/api/v1/auth/login", { method: "POST" });
    expect(assign).not.toHaveBeenCalled();
  });

  it("ignores non-401 responses and non-API URLs", async () => {
    install(500);
    await window.fetch("/api/v1/series");
    install(401);
    await window.fetch("https://image.tmdb.org/t/p/w500/x.jpg");
    expect(assign).not.toHaveBeenCalled();
  });
});
