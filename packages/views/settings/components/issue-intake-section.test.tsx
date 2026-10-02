import userEvent from "@testing-library/user-event";
// @vitest-environment jsdom
import {
  render,
  screen,

  waitFor,
  cleanup,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { I18nProvider } from "@multica/core/i18n/react";
import { api } from "@multica/core/api";
import { afterEach, beforeEach, it, expect, vi } from "vitest";
import en from "../../locales/en/settings.json";
import common from "../../locales/en/common.json";
import { IssueIntakeSection } from "./issue-intake-section";
vi.mock("@multica/core/api", () => ({
  api: {
    getIssueIntake: vi.fn(),
    updateIssueIntake: vi.fn(),
    listSquads: vi.fn(),
    listProjects: vi.fn(),
  },
}));
beforeEach(() => {
  vi.mocked(api.getIssueIntake).mockResolvedValue({
    revision: 0,
    defaultSquadId: "",
    projects: {},
  });
  vi.mocked(api.listSquads).mockResolvedValue([
    { id: "squad", name: "Intake squad" },
  ] as Awaited<ReturnType<typeof api.listSquads>>);
  vi.mocked(api.listProjects).mockResolvedValue({ projects: [], total:0 } as Awaited<
    ReturnType<typeof api.listProjects>
  >);
  vi.mocked(api.updateIssueIntake).mockResolvedValue({
    revision: 1,
    defaultSquadId: "squad",
    projects: {},
  });
});
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});
function show(canManage = true) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <I18nProvider locale="en" resources={{ en: { settings: en, common } }}>
        <IssueIntakeSection workspaceId="workspace" canManage={canManage} />
      </I18nProvider>
    </QueryClientProvider>,
  );
  return qc;
}
it("saves a chosen squad with the loaded revision", async () => {
  const qc = show();
  const select = await screen.findByRole("combobox", {
    name: "Default intake squad",
  });
  await waitFor(() => expect(select).not.toBeDisabled());
  await userEvent.click(select);
  await userEvent.click(await screen.findByRole("option", { name: "Intake squad" }));
  await userEvent.click(screen.getByRole("button", { name: "Save intake settings" }));
  await waitFor(() =>
    expect(api.updateIssueIntake).toHaveBeenCalledWith("workspace", {
      revision: 0,
      defaultSquadId: "squad",
      projects: {},
    }),
  );
  expect(await screen.findByText("Intake settings saved.")).toBeTruthy();
  qc.clear();
});
it("keeps member settings read-only", async () => {
  const qc = show(false);
  expect(
    await screen.findByRole("combobox", { name: "Default intake squad" }),
  ).toBeDisabled();
  expect(
    screen.queryByRole("button", { name: "Save intake settings" }),
  ).toBeNull();
  qc.clear();
});
