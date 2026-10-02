import { render, screen, waitFor, cleanup } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { I18nProvider } from "@multica/core/i18n/react";
import { api } from "@multica/core/api";
import { afterEach, beforeEach, it, expect, vi } from "vitest";
import en from "../../locales/en/settings.json";
import { RepositorySettingsSection } from "./repository-settings-section";
vi.mock("@multica/core/api", () => ({
  api: { getRepositorySettings: vi.fn(), saveRepositorySettings: vi.fn() },
}));
const value = {
  revision: 2,
  repository: "org/repo",
  folder: "repo",
  root: "",
  mode: "worktree" as const,
  can_edit: true,
  supported: true,
};
beforeEach(() => {
  vi.mocked(api.getRepositorySettings).mockResolvedValue(value);
  vi.mocked(api.saveRepositorySettings).mockResolvedValue({
    ...value,
    revision: 3,
  });
});
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});
function show(runtimeId?: string) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <I18nProvider locale="en" resources={{ en: { settings: en } }}>
      <QueryClientProvider client={qc}>
        <RepositorySettingsSection wsId="ws" runtimeId={runtimeId} />
      </QueryClientProvider>
    </I18nProvider>,
  );
  return qc;
}
it("saves the workspace repository with the loaded revision", async () => {
  const qc = show();
  const input = await screen.findByLabelText("GitHub repository");
  await userEvent.clear(input);
  await userEvent.type(input, "org/new-repo");
  await userEvent.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() =>
    expect(api.saveRepositorySettings).toHaveBeenCalledWith(
      "ws",
      expect.objectContaining({ repository: "org/new-repo", revision: 2 }),
      undefined,
    ),
  );
  qc.clear();
});
it("shows a machine root and upgrade requirement without workspace fields", async () => {
  vi.mocked(api.getRepositorySettings).mockResolvedValue({
    ...value,
    repository: "",
    folder: "",
    root: "D:/GitHub",
    supported: false,
    can_edit: false,
  });
  const qc = show("runtime");
  expect(
    await screen.findByRole("textbox", { name: "Repositories folder" }),
  ).toBeDisabled();
  expect(screen.queryByLabelText("GitHub repository")).toBeNull();
  expect(screen.getByText(/Update this machine/)).toBeTruthy();
  qc.clear();
});
it("retains a draft on a stale save and offers reload", async () => {
  vi.mocked(api.saveRepositorySettings).mockRejectedValue(
    new Error("settings changed; reload before saving"),
  );
  const qc = show();
  await userEvent.type(
    await screen.findByLabelText("Local folder name"),
    "-new",
  );
  await userEvent.click(screen.getByRole("button", { name: "Save" }));
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "settings changed",
  );
  expect(screen.getByLabelText("Local folder name")).toHaveValue("repo-new");
  qc.clear();
});
