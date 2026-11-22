import { beforeAll, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { FileText, Server, Settings } from "lucide-react";
import Shell from "@beacon-shared/Shell";

beforeAll(() => {
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: false,
    media: query,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
  }));
});

function renderAt(path: string) {
  render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route
          element={
            <Shell
              appName="Test"
              appIcon={null}
              collapsedStorageKey="test-sidebar"
              mainNav={[{ to: "/settings", icon: Settings, label: "Settings" }]}
              settingsNav={[
                { to: "/settings/system", icon: Server, label: "System" },
                { to: "/settings/system/logs", icon: FileText, label: "Logs" },
              ]}
            />
          }
        >
          <Route path="*" element={<div />} />
        </Route>
      </Routes>
    </MemoryRouter>,
  );
}

function current() {
  const nav = screen.getAllByRole("navigation")[0];
  return within(nav)
    .queryAllByRole("link")
    .filter((a) => a.getAttribute("aria-current") === "page")
    .map((a) => a.getAttribute("title"));
}

describe("sidebar highlight", () => {
  it("lights only Logs on the Logs page, not System as well", () => {
    renderAt("/settings/system/logs");
    expect(current()).toEqual(["Logs"]);
  });

  it("lights only System on the System page, not Settings as well", () => {
    renderAt("/settings/system");
    expect(current()).toEqual(["System"]);
  });
});
