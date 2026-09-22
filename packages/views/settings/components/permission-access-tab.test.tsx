import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useAuthStore } from "@multica/core/auth";
import { EMPTY_USER } from "@multica/core/api/schemas";
import { renderWithI18n } from "../../test/i18n";
import { PermissionAccessTab } from "./permission-access-tab";

const mocks = vi.hoisted(() => ({ get: vi.fn(), save: vi.fn(), refresh: vi.fn() }));
vi.mock("@multica/core/api", () => ({ api: { getRuntimePermissionPolicy: mocks.get, updateRuntimePermissionPolicy: mocks.save } }));
vi.mock("@multica/core/auth", async () => {
  const { create } = await import("zustand");
  return { useAuthStore: create(() => ({ user: null, refreshMe: mocks.refresh })) };
});
const policy = { action: "runtime.register", allowed_emails: ["old@foresightsports.com"], restricted: true, revision: 2 };
function renderPage() {
  return renderWithI18n(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><PermissionAccessTab /></QueryClientProvider>);
}
beforeEach(() => {
  vi.clearAllMocks();
  mocks.get.mockResolvedValue(policy);
  mocks.refresh.mockResolvedValue(undefined);
  useAuthStore.setState({ user: { ...EMPTY_USER, id: "manager", permissions: { register_runtimes: false, manage_permission_access: true } } });
});
describe("permission access settings", () => {
  it("saves the edited list with its revision and refreshes the user's capabilities", async () => {
    mocks.save.mockResolvedValue({ ...policy, revision: 3, allowed_emails: ["new@foresightsports.com"] });
    renderPage();
    const field = await screen.findByLabelText("Allowed users");
    fireEvent.change(field, { target: { value: "new@foresightsports.com" } });
    fireEvent.click(screen.getByRole("button", { name: "Save access" }));
    await waitFor(() => expect(mocks.save).toHaveBeenCalledWith({ allowed_emails: ["new@foresightsports.com"], revision: 2 }));
    await waitFor(() => expect(mocks.refresh).toHaveBeenCalled());
    expect(screen.getByRole("button", { name: "Save access" })).toBeDisabled();
  });
  it("keeps edits and shows a failed save instead of claiming success", async () => {
    mocks.save.mockRejectedValue(new Error("permission access changed; reload the latest list before saving"));
    renderPage();
    fireEvent.change(await screen.findByLabelText("Allowed users"), { target: { value: "new@foresightsports.com" } });
    fireEvent.click(screen.getByRole("button", { name: "Save access" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("reload the latest list");
    expect(screen.getByLabelText("Allowed users")).toHaveValue("new@foresightsports.com");
    expect(mocks.refresh).not.toHaveBeenCalled();
  });
  it("does not fetch the policy for a user who cannot manage access", () => {
    useAuthStore.setState({ user: { ...EMPTY_USER, id: "member", permissions: { register_runtimes: true, manage_permission_access: false } } });
    renderPage();
    expect(screen.getByRole("status")).toHaveTextContent("Only configured permission managers");
    expect(mocks.get).not.toHaveBeenCalled();
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
  });
});
