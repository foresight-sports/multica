import { render, screen, waitFor, cleanup } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { I18nProvider } from "@multica/core/i18n/react";
import { api } from "@multica/core/api";
import { afterEach, beforeEach, it, expect, vi } from "vitest";
import en from "../../locales/en/settings.json";
import { CodeDeliverySection } from "./code-delivery-section";
vi.mock("@multica/core/api", () => ({
  api: {
    getCodeDelivery: vi.fn(),
    saveCodeDelivery: vi.fn(),
    listProjects: vi.fn(),
  },
}));
const value = {
  revision: 2,
  can_edit: true,
  automation_available: true,
  automation_errors: [],
  default: {
    mode: "pull_request" as const,
    target_branch: "",
    ai_reviewers: [],
    human_reviewers: [],
    human_teams: [],
  },
  projects: {},
};
beforeEach(() => {
  vi.mocked(api.getCodeDelivery).mockResolvedValue(value);
  vi.mocked(api.saveCodeDelivery).mockResolvedValue({ ...value, revision: 3 });
  vi.mocked(api.listProjects).mockResolvedValue({
    projects: [{ id: "project", title: "Multica" }],
  } as Awaited<ReturnType<typeof api.listProjects>>);
});
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});
function show() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <I18nProvider locale="en" resources={{ en: { settings: en } }}>
      <QueryClientProvider client={qc}>
        <CodeDeliverySection wsId="ws" />
      </QueryClientProvider>
    </I18nProvider>,
  );
  return qc;
}
it("saves a project override without changing the workspace default", async () => {
  const qc = show();
  await screen.findByRole("option", { name: "Multica" });
  await userEvent.selectOptions(
    screen.getByLabelText("Delivery scope"),
    "project",
  );
  await userEvent.selectOptions(
    screen.getByLabelText("Delivery method"),
    "direct",
  );
  await userEvent.type(
    screen.getByLabelText("Delivery target branch"),
    "develop",
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Save delivery settings" }),
  );
  await waitFor(() =>
    expect(api.saveCodeDelivery).toHaveBeenCalledWith(
      "ws",
      expect.objectContaining({
        revision: 2,
        default: value.default,
        projects: {
          project: {
            ...value.default,
            mode: "direct",
            target_branch: "develop",
          },
        },
      }),
    ),
  );
  qc.clear();
});
it("supports multiple reviewer identities", async () => {
  const qc = show();
  const input = await screen.findByLabelText("AI reviewer accounts");
  await userEvent.type(input, "one[[bot],two[[bot]");
  expect(input).toHaveValue("one[bot],two[bot]");
  qc.clear();
});
it("does not permit members to save policy", async () => {
  vi.mocked(api.getCodeDelivery).mockResolvedValue({
    ...value,
    can_edit: false,
  });
  const qc = show();
  expect(await screen.findByLabelText("Delivery method")).toBeDisabled();
  expect(
    screen.getByRole("button", { name: "Save delivery settings" }),
  ).toBeDisabled();
  qc.clear();
});
