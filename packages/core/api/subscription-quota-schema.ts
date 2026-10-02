import { z } from "zod";
const windowSchema = z
  .object({
    id: z.string(),
    used_percent: z.number().finite().nonnegative().nullable(),
    resets_at: z.number().nonnegative(),
    duration_minutes: z.number().nonnegative(),
    model: z.string().optional(),
    exhausted: z.boolean(),
    applies_all: z.boolean(),
  })
  .transform((w) => ({
    id: w.id,
    usedPercent: w.used_percent,
    resetsAt: w.resets_at,
    durationMinutes: w.duration_minutes,
    model: w.model,
    exhausted: w.exhausted,
    appliesAll: w.applies_all,
  }));
export const SubscriptionQuotaSchema = z
  .object({
    status: z.enum(["unknown", "reported", "stale", "not_applicable"]),
    observed_at: z.string().optional(),
    source: z.string().optional(),
    windows: z.array(windowSchema),
  })
  .transform((r) => ({
    status: r.status,
    observedAt: r.observed_at,
    source: r.source,
    windows: r.windows,
  }));
export type SubscriptionQuota = z.infer<typeof SubscriptionQuotaSchema>;
export const unknownSubscriptionQuota: SubscriptionQuota = {
  status: "unknown",
  observedAt: undefined,
  source: undefined,
  windows: [],
};
export function quotaWindowState(
  report: SubscriptionQuota,
  w: SubscriptionQuota["windows"][number],
  now = Date.now(),
) {
  const observed = Date.parse(report.observedAt ?? "");
  if (
    report.status !== "reported" ||
    !Number.isFinite(observed) ||
    now - observed > 600000 ||
    observed - now > 60000 ||
    (w.resetsAt > 0 && w.resetsAt * 1000 <= now)
  )
    return { status: "unknown" as const, remaining: null };
  return {
    status:
      w.exhausted || (w.usedPercent !== null && w.usedPercent >= 100)
        ? ("exhausted" as const)
        : ("reported" as const),
    remaining: w.usedPercent === null ? null : Math.max(0, 100 - w.usedPercent),
  };
}
