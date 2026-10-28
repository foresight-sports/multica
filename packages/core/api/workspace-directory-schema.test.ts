// @vitest-environment node
import { expect, it } from "vitest";
import { WorkspaceDirectorySchema, WorkspaceAccessRequestsSchema } from "./workspace-directory-schema";

it("fails closed on unknown permission and membership fields", () => {
  const data = WorkspaceDirectorySchema.parse({ can_create: "true", workspaces: [{ id: "ws", name: "Team", slug: "team", role: "unexpected", request_status: "future" }] });
  expect(data.can_create).toBe(false);
  expect(data.workspaces[0]?.role).toBe("");
  expect(data.workspaces[0]?.request_status).toBe("");
});
it("rejects malformed directories and review identities", () => {
  expect(WorkspaceDirectorySchema.safeParse({ workspaces: [{ name: "Team" }] }).success).toBe(false);
  expect(WorkspaceAccessRequestsSchema.safeParse({ requests: [{ user_id: null }] }).success).toBe(false);
});
