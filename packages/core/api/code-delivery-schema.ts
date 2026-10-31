import { z } from "zod";
export const DeliverySchema = z.object({
  mode: z.enum(["pull_request", "direct"]).default("pull_request"),
  target_branch: z.string().default(""),
  ai_reviewers: z.array(z.string()).default([]),
  human_reviewers: z.array(z.string()).default([]),
  human_teams: z.array(z.string()).default([]),
});
export const CodeDeliverySchema = z.object({
  default: DeliverySchema,
  projects: z.record(z.string(), DeliverySchema).default({}),
  revision: z.number().int().nonnegative(),
  automation_available: z.boolean().default(false),
  automation_errors: z.array(z.string()).default([]),
  can_edit: z.boolean().default(false),
});
export type CodeDelivery = z.infer<typeof CodeDeliverySchema>;
export type Delivery = z.infer<typeof DeliverySchema>;
