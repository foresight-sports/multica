import { z } from "zod";
export const ExecutionProfileSchema = z.object({
  id: z.string(),
  name: z.string(),
  runtime_id: z.string().default(""),
  provider: z.string().default(""),
  model: z.string(),
  thinking_level: z.string().default(""),
  service_tier: z.string().default(""),
  purpose: z.string().default(""),
  keywords: z
    .array(z.string())
    .nullable()
    .transform((v) => v ?? [])
    .default([]),
  required_os: z.string().default(""),
  required_tools: z
    .array(z.string())
    .nullable()
    .transform((v) => v ?? [])
    .default([]),
  quality: z.number().default(3),
  speed: z.number().default(3),
  cost: z.number().default(3),
});
export const ExecutionPolicySchema = z.object({
  revision: z.number().default(0),
  mode: z.enum(["default", "automatic"]).catch("default"),
  default_profile: z.string().default(""),
  router_profile: z.string().default(""),
  preference: z
    .enum(["balanced", "quality", "speed", "cost"])
    .catch("balanced"),
  allow_fallback: z.boolean().default(false),
  profiles: z.array(ExecutionProfileSchema).default([]),
});
export const ExecutionRequestSchema = z.object({
  mode: z.enum(["default", "automatic"]).optional(),
  profile_id: z.string().optional(),
  runtime_id: z.string().optional(),
  model: z.string().optional(),
  fresh_session: z.boolean().optional(),
  instruction: z.string().max(8000).optional(),
});
export const ExecutionSelectionSchema = z.object({
  state: z.string(),
  profile: ExecutionProfileSchema.optional().catch(undefined),
  reason: z.string().default(""),
  policy_revision: z.number().default(0),
  explicit: z.boolean().default(false),
  history: z
    .array(
      z.object({
        profile_id: z.string(),
        runtime_id: z.string().default(""),
  provider: z.string().default(""),
        model: z.string(),
        reason: z.string(),
        at: z.string(),
      }),
    )
    .nullable()
    .transform((v) => v ?? [])
    .default([]),
});
export const ExecutionPreviewSchema = z.object({
  candidates: z.array(
    z.object({
      profile: ExecutionProfileSchema,
      eligible: z.boolean(),
      reason: z.string().default(""),
    }),
  ),
  profile: ExecutionProfileSchema,
  reason: z.string().default(""),
  blocked: z.string().default(""),
  uses_router: z.boolean().default(false),
});
export type ExecutionProfile = z.infer<typeof ExecutionProfileSchema>;
export type ExecutionPolicy = z.infer<typeof ExecutionPolicySchema>;
export type ExecutionRequest = z.infer<typeof ExecutionRequestSchema>;
export type ExecutionSelection = z.infer<typeof ExecutionSelectionSchema>;
export type ExecutionPreview = z.infer<typeof ExecutionPreviewSchema>;
