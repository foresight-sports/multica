import { beforeEach, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import en from "../../locales/en/settings.json";
import { WorkspaceLinksSection } from "./workspace-links-section";
const mock = vi.hoisted(() => ({ get: vi.fn(), save: vi.fn() }));
vi.mock("@multica/core/api", () => ({
  api: { getWorkspaceLinks: mock.get, saveWorkspaceLink: mock.save },
}));
vi.mock("../../i18n", () => ({
  useT: () => ({ t: (select: (value: typeof en) => string) => select(en) }),
}));
function mount() {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({
          defaultOptions: {
            queries: { retry: false },
            mutations: { retry: false },
          },
        })
      }
    >
      <WorkspaceLinksSection wsId="app" />
    </QueryClientProvider>,
  );
}
const sdk = {
  id: "sdk",
  name: "SDK - iOS",
  slug: "sdk-ios",
  description: "",
  linked: false,
  can_edit: true,
};
beforeEach(() => {
  vi.clearAllMocks();
  mock.get.mockResolvedValue([sdk]);
});
it("links a workspace and reflects the server response", async () => {
  mock.save.mockResolvedValue([{ ...sdk, linked: true }]);
  mount();
  fireEvent.click(
    await screen.findByRole("button", { name: "Link SDK - iOS" }),
  );
  await waitFor(() =>
    expect(mock.save).toHaveBeenCalledWith("app", "sdk", true),
  );
  expect(
    await screen.findByRole("button", { name: "Unlink SDK - iOS" }),
  ).toBeEnabled();
});
it("does not expose link controls without administration rights", async () => {
  mock.get.mockResolvedValue([{ ...sdk, can_edit: false }]);
  mount();
  expect(await screen.findByText("SDK - iOS")).toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "Link SDK - iOS" }),
  ).not.toBeInTheDocument();
});
it("preserves the current link on failure and explains recovery", async () => {
  mock.save.mockRejectedValue(new Error("Access changed"));
  mount();
  fireEvent.click(
    await screen.findByRole("button", { name: "Link SDK - iOS" }),
  );
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Could not change the link",
  );
  expect(screen.getByRole("button", { name: "Link SDK - iOS" })).toBeEnabled();
});
it("offers a retry when loading fails", async () => {
  mock.get.mockRejectedValue(new Error("Unavailable"));
  mount();
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Could not load workspace links",
  );
  expect(screen.getByRole("button", { name: "Try again" })).toBeEnabled();
});
