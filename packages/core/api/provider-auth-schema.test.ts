// @vitest-environment node
import { describe, expect, it } from "vitest";
import { ProviderAuthSchema } from "./provider-auth-schema";
describe("provider authentication responses", () => {
  it("rejects unknown statuses and providers", () => {
    expect(ProviderAuthSchema.safeParse({ status: "success" }).success).toBe(false);
    expect(ProviderAuthSchema.safeParse({ status: "running", provider: "shell" }).success).toBe(false);
    expect(ProviderAuthSchema.safeParse({ status: "idle" }).success).toBe(true);
    expect(ProviderAuthSchema.safeParse({ status: "running" }).success).toBe(false);
  });
});
