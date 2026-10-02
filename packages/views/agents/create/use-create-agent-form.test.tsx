import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { useAuthStore } from "@multica/core/auth";
import { EMPTY_USER } from "@multica/core/api/schemas";
import { useCreateAgentForm } from "./use-create-agent-form";

vi.mock("@multica/core/auth", async () => {
  const { create } = await import("zustand");
  return { useAuthStore: create(() => ({ user: null })) };
});
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "workspace" }));
vi.mock("@tanstack/react-query", async (original) => ({
  ...await original<object>(),
  useQuery: () => ({ data: [], isLoading: false, isSuccess: true, isError: false }),
}));
beforeEach(() => {
  useAuthStore.setState({ user: { ...EMPTY_USER, permissions: { register_runtimes: false, manage_permission_access: true, create_instance_agents: true, create_agents: true, edit_agents: true } } });
});
it("defaults new administrator drafts to instance and preserves explicit opt-out", async () => {
  const { result } = renderHook(() => useCreateAgentForm());
  await waitFor(() => expect(result.current.draft.instanceAgent).toBe(true));
  act(() => result.current.setDraft((draft) => ({ ...draft, instanceAgent: false })));
  expect(result.current.draft.instanceAgent).toBe(false);
});
it("keeps ordinary member creation workspace scoped", async () => {
  useAuthStore.setState({ user: EMPTY_USER });
  const { result } = renderHook(() => useCreateAgentForm());
  await waitFor(() => expect(result.current.draft.instanceAgent).toBe(false));
});
