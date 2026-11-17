import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import LanguagePage, { orderedSelection } from "./LanguagePage";
import { filterReasonLabel } from "@/api/language";

type Call = { url: string; init?: RequestInit };

describe("language helpers", () => {
  it("keeps the selection in display order", () => {
    expect(orderedSelection(["english", "french", "german"], new Set(["german", "english"]))).toEqual(["english", "german"]);
  });

  it("labels filter reasons", () => {
    expect(filterReasonLabel("unwanted_language", { languages: ["french", "german"] })).toBe("unwanted language: French, German");
    expect(filterReasonLabel("unwanted_language", {})).toBe("unwanted language: English (untagged)");
    expect(filterReasonLabel("release_profile", {})).toBe("rejected by a release profile");
    expect(filterReasonLabel("below minimum seeders (2 < 5)", {})).toBe("below minimum seeders (2 < 5)");
  });
});

describe("LanguagePage", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("loads the preference and saves the checked languages", async () => {
    const calls: Call[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string, init?: RequestInit) => {
        calls.push({ url, init });
        const languages = init?.method === "PUT" ? JSON.parse(String(init.body)).languages : ["english"];
        return new Response(JSON.stringify({ languages, available: ["english", "french", "german"] }), { status: 200 });
      }),
    );
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <LanguagePage />
      </QueryClientProvider>,
    );

    const english = await screen.findByLabelText("English");
    expect(english).toBeChecked();
    expect(screen.getByLabelText("German")).not.toBeChecked();

    const user = userEvent.setup();
    await user.click(screen.getByLabelText("German"));
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(calls.some((c) => c.init?.method === "PUT")).toBe(true));
    const put = calls.find((c) => c.init?.method === "PUT")!;
    expect(put.url).toMatch(/\/api\/v1\/language-preference$/);
    expect(JSON.parse(String(put.init!.body))).toEqual({ languages: ["english", "german"] });
  });
});
