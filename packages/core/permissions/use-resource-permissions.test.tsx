// @vitest-environment jsdom
import { renderHook } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import type { Agent } from "../types";
import { useAgentPermissions } from "./use-resource-permissions";

const state = vi.hoisted(() => ({ edit: false, role: "member", userId: "owner" }));
vi.mock("../auth", () => ({ useAuthStore: (selector: (s: unknown) => unknown) => selector({ user: { permissions: { edit_agents: state.edit, manage_permission_access: true } } }) }));
vi.mock("./use-current-member", () => ({ useCurrentMember: () => ({ userId: state.userId, role: state.role, isLoading: false }) }));
const agent = { id: "agent", owner_id: "owner", visibility: "workspace", permission_mode: "public_to", invocation_targets: [{ target_type: "workspace", target_id: "workspace" }] } as Agent;
beforeEach(() => { state.edit = false; state.role = "member"; state.userId = "owner"; });
it("requires an explicit edit grant even for permission managers", () => {
 const { result } = renderHook(() => useAgentPermissions(agent, "workspace"));
 expect(result.current.canEdit.allowed).toBe(false);
});
it("keeps resource ownership checks after a grant", () => {
 state.edit = true;
 const { result, rerender } = renderHook(() => useAgentPermissions(agent, "workspace"));
 expect(result.current.canEdit.allowed).toBe(true);
 state.userId = "someone-else";
 rerender();
 expect(result.current.canEdit.allowed).toBe(false);
});
it("allows editing the source but never a workspace copy", () => {
 state.edit = true;
 const { result, rerender } = renderHook(({ source }) => useAgentPermissions({ ...agent, instance_agent_id: "instance", instance_source_agent_id: source }, "workspace"), { initialProps: { source: "agent" } });
 expect(result.current.canEdit.allowed).toBe(true);
 rerender({ source: "other-agent" });
 expect(result.current.canEdit.allowed).toBe(false);
});
