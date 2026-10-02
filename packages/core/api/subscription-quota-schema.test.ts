// @vitest-environment node
import { describe, it, expect } from "vitest";
import {
  SubscriptionQuotaSchema,
  quotaWindowState,
  type SubscriptionQuota,
} from "./subscription-quota-schema";
describe("subscription observations", () => {
  it("distinguishes missing percentages, exhausted status and expired observations", () => {
    const now = Date.now();
    const r: SubscriptionQuota = {
      source: undefined,
      status: "reported",
      observedAt: new Date(now).toISOString(),
      windows: [],
    };
    const w = {
      model: undefined,
      id: "weekly",
      usedPercent: null,
      resetsAt: 0,
      durationMinutes: 10080,
      exhausted: false,
      appliesAll: true,
    };
    expect(quotaWindowState(r, w, now).remaining).toBeNull();
    expect(quotaWindowState(r, { ...w, exhausted: true }, now).status).toBe(
      "exhausted",
    );
    expect(quotaWindowState(r, { ...w, usedPercent: 0 }, now).remaining).toBe(
      100,
    );
    expect(quotaWindowState(r, w, now + 600001).status).toBe("unknown");
    expect(
      quotaWindowState(r, { ...w, resetsAt: Math.floor(now / 1000) }, now)
        .status,
    ).toBe("unknown");
  });
});

it("rejects malformed wire values and maps valid reports", () => {
  expect(
    SubscriptionQuotaSchema.safeParse({ status: "reported" }).success,
  ).toBe(false);
  expect(
    SubscriptionQuotaSchema.safeParse({ status: "unknown", windows: [] })
      .success,
  ).toBe(true);
  const parsed = SubscriptionQuotaSchema.parse({
    status: "reported",
    observed_at: "now",
    windows: [
      {
        id: "w",
        used_percent: 0,
        resets_at: 0,
        duration_minutes: 300,
        exhausted: false,
        applies_all: true,
      },
    ],
  });
  expect(parsed.windows[0]?.usedPercent).toBe(0);
});
