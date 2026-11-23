import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConfirmProvider } from "@beacon-shared/ConfirmDialog";
import Notifications from "@beacon-shared/Notifications";

const catalog = {
  app: "Pilot",
  hub: false,
  kinds: [
    {
      id: "ntfy",
      name: "ntfy",
      help: "Subscribe to the topic in the ntfy app.",
      fields: [
        { key: "url", label: "Server", type: "url", required: true, placeholder: "https://ntfy.sh" },
        { key: "topic", label: "Topic", type: "text", required: true },
        { key: "token", label: "Access token", type: "password" },
      ],
    },
    { id: "discord", name: "Discord", fields: [{ key: "webhook_url", label: "Webhook URL", type: "url", required: true }] },
  ],
  types: [
    { id: "imported", label: "Imported", description: "In your library.", apps: ["pilot", "prism"], default: true },
    { id: "grabbed", label: "Release grabbed", description: "Sent to the client.", apps: ["pilot", "prism"], default: false },
    { id: "health_issue", label: "Health problem", description: "Needs attention.", apps: ["pilot", "prism", "haul", "pulse"], default: true },
  ],
};

type Call = { url: string; method: string; body?: unknown };

function serve(opts: { viaPulse?: boolean; channels?: unknown[]; testResult?: object } = {}) {
  const calls: Call[] = [];
  let mode = opts.viaPulse ? "auto" : "local";
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init?: RequestInit) => {
      const method = init?.method ?? "GET";
      const body = init?.body ? JSON.parse(String(init.body)) : undefined;
      calls.push({ url, method, body });
      const json = (b: unknown, status = 200) => new Response(JSON.stringify(b), { status });
      if (url.endsWith("/notifications/catalog")) return json(catalog);
      if (url.endsWith("/notifications/status")) {
        const via = mode === "auto" && !!opts.viaPulse;
        return json({ app: "Pilot", hub: false, channels: 1, enabled: 1, waiting: 0, route: { mode, via_pulse: via, pulse_connected: !!opts.viaPulse, pulse_channels: 2 } });
      }
      if (url.endsWith("/notifications/channels") && method === "GET") return json(opts.channels ?? []);
      if (url.endsWith("/notifications/channels") && method === "POST") return json({ id: "new", ...body }, 201);
      if (url.endsWith("/notifications/recent"))
        return json([{ time: "2026-11-23T12:00:00Z", app: "Pilot", title: "Imported: Andor", level: "success", via: "this app", results: [{ channel_name: "Phone" }] }]);
      if (url.endsWith("/notifications/test")) return json(opts.testResult ?? { ok: true });
      if (url.endsWith("/notifications/mode")) {
        mode = body.mode;
        return json({});
      }
      return json({});
    }),
  );
  render(
    <ConfirmProvider>
      <Notifications />
    </ConfirmProvider>,
  );
  return calls;
}

afterEach(() => vi.unstubAllGlobals());

describe("shared Notifications page", () => {
  it("says when Pulse handles notifications, and can switch to this app", async () => {
    const calls = serve({ viaPulse: true });
    expect(await screen.findByText("Notifications go through Pulse")).toBeInTheDocument();
    expect(screen.getByText(/2 channels/)).toBeInTheDocument();
    expect(screen.getByText("Used only if Pulse can't be reached.")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Send from Pilot instead" }));
    await waitFor(() => expect(calls.some((c) => c.url.endsWith("/notifications/mode") && (c.body as { mode: string }).mode === "local")).toBe(true));
    expect(await screen.findByRole("button", { name: "Send through Pulse instead" })).toBeInTheDocument();
  });

  it("adds an ntfy channel with Pilot's events and the defaults ticked", async () => {
    const user = userEvent.setup();
    const calls = serve();
    expect(await screen.findByText("No channels yet")).toBeInTheDocument();
    expect(screen.getByText("Imported: Andor")).toBeInTheDocument(); // recently sent
    await user.click(screen.getByRole("button", { name: /Add channel/ }));
    await user.click(screen.getByRole("button", { name: "ntfy" }));
    expect(screen.getByText("Subscribe to the topic in the ntfy app.")).toBeInTheDocument();
    // Only Pilot's events are offered; defaults are ticked.
    expect(screen.getByRole("checkbox", { name: /Imported/ })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: /Release grabbed/ })).not.toBeChecked();
    await user.type(screen.getByLabelText(/Server/), "https://ntfy.sh");
    await user.type(screen.getByLabelText(/^Topic/), "beacon-home");
    await user.click(screen.getByRole("checkbox", { name: /Release grabbed/ }));
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(calls.some((c) => c.method === "POST" && c.url.endsWith("/notifications/channels"))).toBe(true));
    const post = calls.find((c) => c.method === "POST" && c.url.endsWith("/notifications/channels"))!;
    expect(post.body).toEqual({
      name: "ntfy",
      kind: "ntfy",
      enabled: true,
      settings: { url: "https://ntfy.sh", topic: "beacon-home" },
      events: ["imported", "health_issue", "grabbed"],
      apps: [],
    });
  });

  it("keeps saved secrets out of the form and reports a failed test plainly", async () => {
    const user = userEvent.setup();
    serve({
      channels: [{ id: "c1", name: "Phone", kind: "ntfy", enabled: true, settings: { url: "https://ntfy.sh", topic: "t", token: "***" }, events: ["imported"], apps: [] }],
      testResult: { ok: false, error: "ntfy.sh answered 401 unauthorized" },
    });
    const channels = await screen.findByRole("region", { name: "Channels" });
    const row = (await within(channels).findByText("Phone")).closest("div")!.parentElement!.parentElement!;
    expect(within(row).getByText("ntfy · Imported")).toBeInTheDocument();
    await user.click(within(row).getByRole("button", { name: /Send test/ }));
    expect(await screen.findByText("Couldn't send: ntfy.sh answered 401 unauthorized")).toBeInTheDocument();
    await user.click(within(row).getByRole("button", { name: "Edit" }));
    const token = screen.getByLabelText(/Access token/) as HTMLInputElement;
    expect(token.value).toBe("");
    expect(token.placeholder).toBe("Saved; type to replace");
  });
});
