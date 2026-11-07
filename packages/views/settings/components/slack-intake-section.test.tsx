// @vitest-environment jsdom
import { describe, it, expect, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { I18nProvider } from "@multica/core/i18n/react";
import enSettings from "../../locales/en/settings.json";
import enCommon from "../../locales/en/common.json";
const directory = vi.hoisted(() =>
  vi.fn(async () => ({
    channels: [{ id: "C1", name: "#support" }],
    people: [{ id: "U1", name: "Kody" }],
  })),
);
const save = vi.hoisted(() => vi.fn(async () => []));
vi.mock("@multica/core/api", () => ({
  api: {
    getSlackIntake: async () => [],
    getSlackSupportActivity: async () => [],
    getSlackSupportDirectory: directory,
    saveSlackIntake: save,
  },
}));
vi.mock("@multica/core/workspace/queries", () => ({
  agentListOptions: () => ({
    queryKey: ["agents"],
    queryFn: async () => [
      { id: "c", name: "Coordinator" },
      { id: "s", name: "Sammy" },
      { id: "w", name: "Writer" },
    ],
  }),
}));
vi.mock("@multica/core/slack", () => ({
  slackInstallationsOptions: () => ({
    queryKey: ["apps"],
    queryFn: async () => ({
      installations: [{ id: "app", agent_id: "c", status: "active" }],
    }),
  }),
}));
import { SlackIntakeSection } from "./slack-intake-section";
describe("support audience configuration", () => {
  it("saves a channel without agent or audience selections and defaults to disabled", async () => {
    const u = userEvent.setup();
    const q = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <I18nProvider
        locale="en"
        resources={{ en: { settings: enSettings, common: enCommon } }}
      >
        <QueryClientProvider client={q}>
          <SlackIntakeSection wsId="ws" />
        </QueryClientProvider>
      </I18nProvider>,
    );
    await waitFor(() =>
      expect(
        screen.getByLabelText("Slack app").querySelectorAll("option"),
      ).toHaveLength(2),
    );
    await u.selectOptions(screen.getByLabelText("Slack app"), "app");
    await screen.findByRole("option", { name: "#support" });
    await u.selectOptions(screen.getByLabelText("Channel to monitor"), "C1");
    await u.selectOptions(
      screen.getByLabelText("Mention when further investigation is needed"),
      "U1",
    );
    expect(screen.queryByLabelText("Review coordinator")).toBeNull();
    expect(screen.queryByLabelText("Support analyst (Sammy)")).toBeNull();
    expect(screen.queryByLabelText("Communications agent")).toBeNull();
    expect(screen.queryByLabelText("Audience")).toBeNull();
    await u.click(screen.getByRole("button", { name: "Save workflow" }));
    await waitFor(() =>
      expect(save).toHaveBeenCalledWith(
        "ws",
        expect.objectContaining({
          coordinator_id: undefined,
          analyst_id: undefined,
          composer_id: undefined,
          audience: undefined,
          style_instructions: undefined,
          enabled: false,
        }),
      ),
    );
    q.clear();
  });
});

it("shows the directory failure instead of blaming permissions for every error", async () => {
  directory.mockRejectedValueOnce(
    new Error("Joined-channel directory is temporarily unavailable."),
  );
  const u = userEvent.setup();
  const q = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <I18nProvider
      locale="en"
      resources={{ en: { settings: enSettings, common: enCommon } }}
    >
      <QueryClientProvider client={q}>
        <SlackIntakeSection wsId="ws-error" />
      </QueryClientProvider>
    </I18nProvider>,
  );
  await waitFor(() =>
    expect(
      screen.getByLabelText("Slack app").querySelectorAll("option"),
    ).toHaveLength(2),
  );
  await u.selectOptions(screen.getByLabelText("Slack app"), "app");
  expect(
    await screen.findByText(
      "Joined-channel directory is temporarily unavailable.",
    ),
  ).toBeTruthy();
  q.clear();
});
