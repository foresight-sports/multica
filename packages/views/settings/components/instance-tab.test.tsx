import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useAuthStore } from "@multica/core/auth";
import { EMPTY_USER } from "@multica/core/api/schemas";
import { renderWithI18n } from "../../test/i18n";
import { InstanceTab } from "./instance-tab";

const mocks = vi.hoisted(() => ({ config: vi.fn(), save: vi.fn(), agents: vi.fn(), create: vi.fn(), archive: vi.fn(), restore: vi.fn(), role: "owner" }));
vi.mock("@multica/core/api", () => ({ api: { getInstanceConfiguration: mocks.config, updateInstanceConfiguration: mocks.save, listInstanceAgents: mocks.agents, saveInstanceAgent: mocks.create, archiveAgent: mocks.archive, restoreAgent: mocks.restore } }));
vi.mock("@multica/core/auth", async () => {
  const { create } = await import("zustand");
  return { useAuthStore: create(() => ({ user: null })) };
});
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "workspace" }));
vi.mock("@multica/core/paths", () => ({ useWorkspacePaths: () => ({ newAgent: () => "/workspace/agents/new", agentDetail: (id: string) => `/workspace/agents/${id}` }) }));
vi.mock("@multica/core/permissions", async (importOriginal) => ({ ...await importOriginal<object>(), useCurrentMember: () => ({ role: mocks.role }) }));
vi.mock("../../navigation", () => ({ AppLink: ({ href, children, className }: { href: string; children: React.ReactNode; className?: string }) => <a href={href} className={className}>{children}</a> }));
const definition = { id: "global-agent", name: "Reviewer", description: "Shared reviewer", instructions: "Review carefully", revision: 1, agent_id: "local-binding", source_agent_id: "local-binding", source_workspace_id: "workspace", enabled: true, runtime_bound: false };
function renderPage() {
  return renderWithI18n(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><InstanceTab /></QueryClientProvider>);
}
beforeEach(() => {
  vi.clearAllMocks();
  mocks.role = "owner";
  mocks.config.mockResolvedValue({ instructions: "Old policy", revision: 2 });
  mocks.agents.mockResolvedValue([definition]);
  useAuthStore.setState({ user: { ...EMPTY_USER, id: "manager", permissions: { register_runtimes: false, manage_permission_access: true, create_instance_agents: true, create_agents: true, edit_agents: true } } });
});
describe("instance settings", () => {
  it("saves shared instructions with a revision and retains edits on conflicts", async () => {
    mocks.save.mockRejectedValue(new Error("Configuration changed; reload before saving"));
    renderPage();
    fireEvent.change(await screen.findByLabelText("Instance instructions"), { target: { value: "New policy" } });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    await waitFor(() => expect(mocks.save).toHaveBeenCalledWith({ instructions: "New policy", revision: 2 }));
    expect(await screen.findByRole("alert")).toHaveTextContent("reload before saving");
    expect(screen.getByLabelText("Instance instructions")).toHaveValue("New policy");
  });
  it("disables only the current workspace binding and exposes runtime configuration", async () => {
    mocks.archive.mockImplementation(async () => { mocks.agents.mockResolvedValue([{ ...definition, enabled: false }]); return {}; });
    renderPage();
    expect(await screen.findByText("The central runtime needs configuration by an instance administrator.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Configure instance agent" })).toHaveAttribute("href", "/workspace/agents/local-binding");
    fireEvent.click(screen.getByRole("button", { name: "Disable" }));
    await waitFor(() => expect(mocks.archive).toHaveBeenCalledWith("local-binding"));
    expect(await screen.findByText("Disabled in this workspace")).toBeInTheDocument();
    expect(mocks.create).not.toHaveBeenCalled();
  });
  it("keeps global edits and workspace toggles unavailable to ordinary members", async () => {
    mocks.role = "member";
    useAuthStore.setState({ user: { ...EMPTY_USER, id: "member", permissions: { register_runtimes: false, manage_permission_access: false } } });
    renderPage();
    expect(await screen.findByLabelText("Instance instructions")).toHaveAttribute("readonly");
    expect(screen.queryByRole("button", { name: "Save changes" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Create instance agent" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Disable" })).not.toBeInTheDocument();
  });
  it("opens the normal creation flow and explains the instance option", async () => {
    renderPage();
    expect(await screen.findByRole("link", { name: "Create instance agent" })).toHaveAttribute("href", "/workspace/agents/new");
    expect(screen.getByText("Use the normal agent creation flow and select Instance agent in the configuration panel.")).toBeInTheDocument();
    expect(mocks.create).not.toHaveBeenCalled();
  });
});