import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { I18nProvider } from "@multica/core/i18n/react";
import { useAuthStore } from "@multica/core/auth";
import { EMPTY_USER } from "@multica/core/api/schemas";
import enCommon from "../../locales/en/common.json";
import enRuntimes from "../../locales/en/runtimes.json";
import { RuntimesPage } from "./runtimes-page";

vi.mock("@multica/core/auth", async () => {
  const { create } = await import("zustand");
  return { useAuthStore: create(() => ({ isLoading: false, user: null })) };
});
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "workspace-1" }));
vi.mock("@multica/core/realtime", () => ({ useWSEvent: vi.fn() }));
vi.mock("@tanstack/react-query", async (original) => ({
  ...await original<typeof import("@tanstack/react-query")>(),
  useQuery: () => ({ data: [], isLoading: false }),
}));
vi.mock("./connect-remote-dialog", () => ({
  ConnectRemoteDialog: () => <div role="dialog">Connect computer</div>,
}));

function renderPage(permission: boolean | undefined) {
  useAuthStore.setState({ user: { ...EMPTY_USER, id: "user-1",
    permissions: permission === undefined ? undefined : { register_runtimes: permission } } });
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <I18nProvider locale="en" resources={{ en: { common: enCommon, runtimes: enRuntimes } }}>
        <RuntimesPage />
      </I18nProvider>
    </QueryClientProvider>,
  );
}

describe("runtime enrollment controls", () => {
  it.each([false, undefined])("hides both entry points without a server grant (%s)", (permission) => {
    renderPage(permission);
    expect(screen.queryByRole("button", { name: enRuntimes.page.connect_remote })).not.toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
  it("lets an allowed user open setup and closes it when permission is removed", async () => {
    renderPage(true);
    const buttons = screen.getAllByRole("button", { name: enRuntimes.page.connect_remote });
    expect(buttons).toHaveLength(2);
    await userEvent.click(buttons[0]!);
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    act(() => useAuthStore.setState({ user: { ...EMPTY_USER, id: "user-1", permissions: { register_runtimes: false } } }));
    expect(screen.queryByRole("button", { name: enRuntimes.page.connect_remote })).not.toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
});
