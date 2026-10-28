// @vitest-environment node
import { describe, expect, it } from "vitest";
import { InstanceMembersSchema } from "./instance-schema";
describe("instance members response", () => {
  it("accepts accounts without workspace memberships", () => {
    expect(InstanceMembersSchema.safeParse({ members: [{ id: "e696515d-d4d0-4f9c-947e-b63ecdf214d5", name: "New user", email: "new@example.com", workspaces: [] }], truncated: false }).success).toBe(true);
  });
  it("rejects malformed inventory instead of presenting an empty directory", () => {
    expect(InstanceMembersSchema.safeParse({ members: [{ id: "invalid" }], truncated: false }).success).toBe(false);
  });
});
