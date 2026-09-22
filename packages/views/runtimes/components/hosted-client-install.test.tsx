import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { I18nProvider } from "@multica/core/i18n/react";
import enRuntimes from "../../locales/en/runtimes.json";
import { HostedClientInstall } from "./hosted-client-install";

const state = vi.hoisted(() => ({ allowed: true, getInstaller: vi.fn() }));
vi.mock("@multica/core/api", () => ({ api: { getRuntimeInstallation: state.getInstaller } }));
vi.mock("@multica/core/auth", () => ({
  useAuthStore: (selector: (s: unknown) => unknown) => selector({ user: { id: "user-1", permissions: { register_runtimes: state.allowed } } }),
}));
beforeEach(() => { state.allowed = true; state.getInstaller.mockReset(); });
function show() {
  return render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    <I18nProvider locale="en" resources={{ en: { runtimes: enRuntimes } }}><HostedClientInstall /></I18nProvider>
  </QueryClientProvider>);
}
it("shows the server command with no download link", async () => {
  state.getInstaller.mockResolvedValue({ command: "test-private-command" });
  show();
  expect(await screen.findByText("test-private-command")).toBeInTheDocument();
  expect(screen.queryByRole("link")).not.toBeInTheDocument();
});
it("does not request credentials for an unlisted user", () => {
  state.allowed = false;
  show();
  expect(state.getInstaller).not.toHaveBeenCalled();
  expect(screen.getByRole("status")).toHaveTextContent("permission");
});
it("does not offer a broken copy command when configuration is missing", async () => {
  state.getInstaller.mockRejectedValue(new Error("unconfigured"));
  show();
  expect(await screen.findByRole("alert")).toHaveTextContent("Installer unavailable");
  expect(screen.queryByText("test-private-command")).not.toBeInTheDocument();
});
