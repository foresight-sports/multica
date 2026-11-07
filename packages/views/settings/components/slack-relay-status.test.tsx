// @vitest-environment jsdom
import { describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { I18nProvider } from "@multica/core/i18n/react";
import enSettings from "../../locales/en/settings.json";
import enCommon from "../../locales/en/common.json";
const mocks = vi.hoisted(() => ({ status: vi.fn(), replay: vi.fn() }));
vi.mock("@multica/core/api", () => ({
  api: { getSlackRelayStatus: mocks.status, replaySlackRelay: mocks.replay },
}));
import { SlackRelayStatus } from "./slack-relay-status";

function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <I18nProvider
      locale="en"
      resources={{ en: { settings: enSettings, common: enCommon } }}
    >
      <QueryClientProvider client={client}>
        <SlackRelayStatus wsId="ws" />
      </QueryClientProvider>
    </I18nProvider>,
  );
  return client;
}

describe("Slack relay operator visibility", () => {
  it("shows stale contact and only offers replay for held or failed receipts", async () => {
    const receipt = {
      installationId: "i",
      channelId: "CSUPPORT",
      messageTs: "100.1",
      owner: "multica",
      attempts: 0,
      lastError: "",
      receivedAt: "2026-11-06T00:00:00Z",
      updatedAt: "2026-11-06T00:00:00Z",
    };
    mocks.status.mockResolvedValue({
      installations: [
        {
          id: "i",
          appId: "ASUPPORT",
          teamId: "TSUPPORT",
          mode: "relay",
          active: true,
          credentialConfigured: true,
          contactRecent: false,
          lastSeenAt: null,
        },
      ],
      counts: { held_disabled: 1 },
      recent: [
        { ...receipt, id: "held", state: "held_disabled" },
        { ...receipt, id: "sent", state: "delivered" },
      ],
    });
    mocks.replay.mockResolvedValue(undefined);
    const client = mount();
    expect(
      await screen.findByText(/No authenticated relay contact/),
    ).toBeTruthy();
    expect(
      screen.getAllByRole("button", { name: "Replay receipt" }),
    ).toHaveLength(1);
    await userEvent.click(
      screen.getByRole("button", { name: "Replay receipt" }),
    );
    await waitFor(() =>
      expect(mocks.replay).toHaveBeenCalledWith("ws", "held"),
    );
    expect(await screen.findByRole("status")).toHaveTextContent(
      "Replay queued",
    );
    client.clear();
  });
  it("shows load failure and a retry action", async () => {
    mocks.status.mockRejectedValue(new Error("unavailable"));
    const client = mount();
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Relay status is unavailable",
    );
    expect(screen.getByRole("button", { name: "Retry" })).toBeTruthy();
    client.clear();
  });
});
