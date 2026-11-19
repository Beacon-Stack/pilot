import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import QualityProfileEditor, {
  QUALITY_CATALOG,
  buildProfileRequest,
  formFromProfile,
  qualityKey,
  validateForm,
} from "./QualityProfileEditor";
import type { QualityProfile } from "@/types";

const q = (name: string, resolution: string, source: string) => ({ name, resolution, source, codec: "", hdr: "" });

const imported: QualityProfile = {
  id: "qp-1",
  name: "HD-1080p",
  cutoff: q("Bluray-1080p", "1080p", "bluray"),
  qualities: [q("WEBDL-1080p", "1080p", "webdl"), q("Bluray-1080p", "1080p", "bluray"), q("WEBDL-480p", "480p", "webdl"), q("Odd", "1080p", "telesync")],
  upgrade_allowed: false,
  min_custom_format_score: 5,
  upgrade_until_cf_score: 10,
  managed_by_pulse: true,
};

describe("quality profile editor logic", () => {
  it("keys SD resolutions together", () => {
    expect(qualityKey({ resolution: "480p", source: "webdl" })).toBe("sd|webdl");
    expect(qualityKey({ resolution: "576p", source: "webdl" })).toBe("sd|webdl");
  });

  it("has unique catalog entries, highest first", () => {
    const keys = QUALITY_CATALOG.map(qualityKey);
    expect(new Set(keys).size).toBe(keys.length);
    expect(QUALITY_CATALOG[0].name).toBe("Bluray-2160p Remux");
  });

  it("round-trips an existing profile, keeping unknown qualities and CF scores", () => {
    const form = formFromProfile(imported);
    expect([...form.checked].sort()).toEqual(["1080p|bluray", "1080p|webdl", "sd|webdl"]);
    expect(form.extras.map((e) => e.name)).toEqual(["Odd"]);
    expect(form.cutoff).toBe("1080p|bluray");
    expect(validateForm(form)).toBeNull();

    const body = buildProfileRequest(form, imported);
    expect(body.qualities.map((x) => x.name)).toEqual(["Bluray-1080p", "WEBDL-1080p", "WEBDL-480p", "Odd"]);
    expect(body.cutoff.name).toBe("Bluray-1080p");
    expect(body.upgrade_until).toBeUndefined();
    expect(body.min_custom_format_score).toBe(5);
    expect(body.upgrade_until_cf_score).toBe(10);
  });

  it("rejects incomplete forms", () => {
    const base = formFromProfile(null);
    expect(validateForm({ ...base, name: " " })).toMatch(/name/);
    expect(validateForm({ ...base, name: "x", checked: new Set() })).toMatch(/at least one/);
    expect(validateForm({ ...base, name: "x", cutoff: "720p|hdtv" })).toMatch(/cutoff/);
    expect(validateForm({ ...base, name: "x", upgradeUntil: "2160p|remux" })).toMatch(/upgrade limit/);
  });
});

describe("QualityProfileEditor", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("creates a profile with the checked qualities and cutoff", async () => {
    const calls: { url: string; init?: RequestInit }[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string, init?: RequestInit) => {
        calls.push({ url, init });
        return new Response(JSON.stringify({ id: "new" }), { status: 201 });
      }),
    );
    const onClose = vi.fn();
    render(
      <QueryClientProvider client={new QueryClient()}>
        <QualityProfileEditor profile={null} onClose={onClose} />
      </QueryClientProvider>,
    );
    const user = userEvent.setup();
    await user.type(screen.getByLabelText("Name"), "Bluray only");
    // Defaults: WEBDL/WEBRip/Bluray 1080p. Make it Bluray-only.
    await user.click(screen.getByLabelText("WEBDL-1080p"));
    await user.click(screen.getByLabelText("WEBRip-1080p"));
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    const post = calls.find((c) => c.init?.method === "POST")!;
    expect(post.url).toMatch(/\/api\/v1\/quality-profiles$/);
    const body = JSON.parse(String(post.init!.body));
    expect(body.name).toBe("Bluray only");
    expect(body.qualities.map((x: { name: string }) => x.name)).toEqual(["Bluray-1080p"]);
    expect(body.cutoff).toMatchObject({ resolution: "1080p", source: "bluray" });
  });
});
