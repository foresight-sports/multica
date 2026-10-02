import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, it, expect, vi } from "vitest";
import { renderWithI18n } from "../../../test/i18n";
import type { Agent, AgentRuntime } from "@multica/core/types";
import { ExecutionTab, ExecutionPolicyEditor } from "./execution-tab";
const mocks = vi.hoisted(() => ({
  get: vi.fn(),
  save: vi.fn(),
  models: vi.fn(),
  preview: vi.fn(),
}));
vi.mock("@multica/core/api", () => ({
  api: {
    getAgentExecution: mocks.get,
    saveAgentExecution: mocks.save,
    initiateListModels: mocks.models,
    previewAgentExecution: mocks.preview,
  },
}));
vi.mock("@multica/core/auth", () => ({
  useAuthStore: (selector: (state: { user: { id: string } }) => unknown) =>
    selector({ user: { id: "user" } }),
}));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws" }));
const profile = {
  id: "p",
  name: "Careful",
  runtime_id: "",
  provider: "codex",
  model: "m",
  thinking_level: "",
  service_tier: "",
  purpose: "",
  keywords: [],
  required_os: "",
  required_tools: [],
  quality: 3,
  speed: 3,
  cost: 3,
};
const policy = {
  revision: 2,
  mode: "default",
  default_profile: "p",
  router_profile: "",
  preference: "balanced",
  allow_fallback: true,
  profiles: [profile],
};
function page() {
  renderWithI18n(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <ExecutionTab
        agent={{ id: "a", runtime_id: "rt" } as Agent}
        runtimes={[
          { id: "rt", name: "Workstation", provider: "codex" } as AgentRuntime,
        ]}
      />
    </QueryClientProvider>,
  );
}
beforeEach(() => {
  vi.clearAllMocks();
  mocks.get.mockResolvedValue(policy);
  mocks.save.mockImplementation(async (_id, p) => ({ ...p, revision: 3 }));
  mocks.models.mockResolvedValue({
    status: "completed",
    models: [{ id: "m", label: "Model" }],
    supported: true,
  });
});
describe("execution settings", () => {
  it("configures a new agent without fetching or saving an existing agent", () => {
    const changed = vi.fn();
    renderWithI18n(
      <QueryClientProvider client={new QueryClient()}>
        <ExecutionPolicyEditor
          policy={{
            ...policy,
            mode: "default",
            preference: "balanced",
            profiles: [],
            default_profile: "",
          }}
          onChange={changed}
          runtimeId="rt"
          seed={{ model: "m" }}
          runtimes={[
            {
              id: "rt",
              name: "Workstation",
              provider: "codex",
            } as AgentRuntime,
          ]}
        />
      </QueryClientProvider>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Add profile" }));
    expect(changed).toHaveBeenCalledWith(
      expect.objectContaining({
        profiles: [expect.objectContaining({ runtime_id: "", provider: "codex", model: "m" })],
      }),
    );
    expect(mocks.get).not.toHaveBeenCalled();
    expect(mocks.save).not.toHaveBeenCalled();
  });
  it("shows the saved fallback checkbox and saves edited routing policy", async () => {
    const user = userEvent.setup();
    page();
    expect(await screen.findByRole("checkbox")).toBeChecked();
    await user.click(screen.getByRole("combobox", { name: "Selection" }));
    await user.click(
      await screen.findByRole("option", { name: "Choose for each task" }),
    );
    await user.click(
      await screen.findByRole("combobox", { name: "Task understanding" }),
    );
    await user.click(await screen.findByRole("option", { name: "Careful" }));
    fireEvent.click(
      screen.getByRole("button", { name: "Save execution settings" }),
    );
    await waitFor(() =>
      expect(mocks.save).toHaveBeenCalledWith(
        "a",
        expect.objectContaining({
          revision: 2,
          mode: "automatic",
          router_profile: "p",
          allow_fallback: true,
        }),
      ),
    );
    expect(await screen.findByRole("status")).toHaveTextContent("Saved");
  });
  it("preserves a failed save and exposes its error", async () => {
    mocks.save.mockRejectedValue(new Error("Reload before saving"));
    page();
    fireEvent.change(await screen.findByLabelText("Profile name"), {
      target: { value: "Deep" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: "Save execution settings" }),
    );
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Reload before saving",
    );
    expect(screen.getByLabelText("Profile name")).toHaveValue("Deep");
  });
});
