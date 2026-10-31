import { describe, expect, it } from "vitest";
import { CodeDeliverySchema } from "./code-delivery-schema";
describe("code delivery response", () => {
  it("defaults to normal PRs and denies editing when omitted", () => {
    const value = CodeDeliverySchema.parse({ default: {}, revision: 0 });
    expect(value.default.mode).toBe("pull_request");
    expect(value.can_edit).toBe(false);
    expect(value.automation_available).toBe(false);
    expect(value.projects).toEqual({});
  });
  it("rejects malformed permissions and delivery modes", () => {
    expect(
      CodeDeliverySchema.safeParse({
        default: { mode: "anything" },
        revision: 1,
      }).success,
    ).toBe(false);
    expect(
      CodeDeliverySchema.safeParse({
        default: {},
        revision: 1,
        can_edit: "true",
      }).success,
    ).toBe(false);
  });
});
