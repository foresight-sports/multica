import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
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
async function openPolicies() {
 renderPage();
 for (const name of ["Register runtimes", "Create instance agents", "Create agents", "Edit agents"]) {
   await waitFor(() => expect(within(screen.getByRole("region", { name })).getByRole("button", { expanded: false })).toBeInTheDocument());
   fireEvent.click(within(screen.getByRole("region", { name })).getByRole("button", { expanded: false }));
 }
}
beforeEach(() => {
  vi.clearAllMocks();
  mocks.get.mockImplementation((action) => Promise.resolve({ ...policy, action: ({ "runtime-register": "runtime.register", "agent-create": "agent.create", "instance-agent-create": "instance-agent.create", "agent-edit": "agent.edit" } as Record<string, string>)[action] }));
  mocks.refresh.mockResolvedValue(undefined);
  useAuthStore.setState({ user: { ...EMPTY_USER, id: "manager", permissions: { register_runtimes: false, manage_permission_access: true } } });
});
describe("permission access settings", () => {
  it("saves the edited list with its revision and refreshes the user's capabilities", async () => {
    mocks.save.mockResolvedValue({ ...policy, revision: 3, allowed_emails: ["new@foresightsports.com"] });
    await openPolicies();
    const field = (await screen.findAllByLabelText("Allowed users"))[0]!;
    fireEvent.change(field, { target: { value: "new@foresightsports.com" } });
    fireEvent.click(screen.getAllByRole("button", { name: "Save access" })[0]!);
    await waitFor(() => expect(mocks.save).toHaveBeenCalledWith({ allowed_emails: ["new@foresightsports.com"], revision: 2 }, "runtime-register"));
    await waitFor(() => expect(mocks.refresh).toHaveBeenCalled());
    expect(within(screen.getByRole("region", { name: "Register runtimes" })).getByRole("button", { expanded: false })).toBeInTheDocument();
  });
  it("keeps edits and shows a failed save instead of claiming success", async () => {
    mocks.save.mockRejectedValue(new Error("permission access changed; reload the latest list before saving"));
    await openPolicies();
    fireEvent.change(screen.getAllByLabelText("Allowed users")[0]!, { target: { value: "new@foresightsports.com" } });
    fireEvent.click(screen.getAllByRole("button", { name: "Save access" })[0]!);
    expect(await screen.findByRole("alert")).toHaveTextContent("reload the latest list");
    expect(screen.getAllByLabelText("Allowed users")[0]!).toHaveValue("new@foresightsports.com");
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

it("saves each agent policy independently", async () => {
 mocks.save.mockImplementation((data) => Promise.resolve({ ...policy, ...data, revision: 3 }));
 await openPolicies();
 await waitFor(() => expect(screen.getAllByLabelText("Allowed users")).toHaveLength(4));
 const fields = screen.getAllByLabelText("Allowed users");
 for (const [index, action] of ["instance-agent-create", "agent-create", "agent-edit"].entries()) {
 fireEvent.change(fields[index + 1]!, { target: { value: "delegate@example.com" } });
 fireEvent.click(within(fields[index + 1]!.closest("form")!).getByRole("button", { name: "Save access" }));
 await waitFor(() => expect(mocks.save).toHaveBeenCalledWith({ allowed_emails: ["delegate@example.com"], revision: 2 }, action));
 }
});
