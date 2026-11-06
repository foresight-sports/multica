import { describe, it, expect } from "vitest";
import {
  SlackIntakeRulesSchema,
  SlackSupportDirectorySchema,
} from "./slack-intake-schema";
describe("Slack support API contract", () => {
  it("rejects malformed authorization and revision fields", () => {
    const rule = {
      id: "r",
      workspace_id: "w",
      installation_id: "i",
      channel_id: "C",
      channel_name: "support",
      workflow: "support_diagnosis",
      service_user_id: "s",
      coordinator_id: "c",
      analyst_id: "a",
      escalation_user_id: "U",
      enabled: false,
      revision: 1,
      timeout_minutes: 60,
    };
    expect(SlackIntakeRulesSchema.parse([rule])[0]?.composer_id).toBeNull();
    expect(
      SlackIntakeRulesSchema.safeParse([{ ...rule, enabled: "false" }]).success,
    ).toBe(false);
    expect(
      SlackIntakeRulesSchema.safeParse([{ ...rule, revision: 1.5 }]).success,
    ).toBe(false);
    expect(
      SlackIntakeRulesSchema.safeParse([{ ...rule, audience: "unknown" }])
        .success,
    ).toBe(false);
  });
  it("does not accept an unusable directory", () => {
    expect(
      SlackSupportDirectorySchema.safeParse({
        channels: [{ name: "support" }],
        people: [],
      }).success,
    ).toBe(false);
  });
});
