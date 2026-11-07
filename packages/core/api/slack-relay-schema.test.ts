// @vitest-environment node
import { describe, expect, it } from "vitest";
import {
  SlackRelayStatusSchema,
  SlackRelayReplaySchema,
} from "./slack-intake-schema";

describe("Slack relay status boundary", () => {
  const installation = {
    id: "i",
    app_id: "a",
    team_id: "t",
    mode: "relay",
    active: true,
    credential_configured: true,
    last_seen_at: null,
    contact_recent: false,
  };
  it("rejects missing health and malformed authorization flags", () => {
    expect(SlackRelayStatusSchema.safeParse({}).success).toBe(false);
    expect(
      SlackRelayStatusSchema.safeParse({
        installations: [{ ...installation, credential_configured: "true" }],
        counts: {},
        recent: [],
      }).success,
    ).toBe(false);
    expect(
      SlackRelayStatusSchema.safeParse({
        installations: [installation],
        counts: { pending: -1 },
        recent: [],
      }).success,
    ).toBe(false);
  });
  it("maps server keys and preserves unknown modes without granting actions", () => {
    const parsed = SlackRelayStatusSchema.parse({
      installations: [
        { ...installation, mode: "future", secret: "never surface" },
      ],
      counts: {},
      recent: [],
    });
    expect(parsed.installations[0]).toEqual({
      id: "i",
      appId: "a",
      teamId: "t",
      mode: "future",
      active: true,
      credentialConfigured: true,
      lastSeenAt: null,
      contactRecent: false,
    });
    expect(
      SlackRelayReplaySchema.safeParse({ state: "delivered" }).success,
    ).toBe(false);
  });
});
