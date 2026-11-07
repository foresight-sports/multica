import { z } from "zod";
export const SlackIntakeRuleSchema = z.object({
  id: z.string(),
  workspace_id: z.string(),
  installation_id: z.string(),
  channel_id: z.string(),
  channel_name: z.string(),
  workflow: z.literal("support_diagnosis"),
  service_user_id: z.string(),
  composer_id: z.string().nullable().default(null),
  audience: z.string().optional(),
  style_instructions: z.string().default(""),
  coordinator_id: z.string().nullable().optional(),
  analyst_id: z.string().nullable().optional(),
  escalation_user_id: z.string(),
  enabled: z.boolean(),
  revision: z.number().int(),
  timeout_minutes: z.number().int(),
});
export const SlackIntakeRulesSchema = z.array(SlackIntakeRuleSchema);
export type SlackIntakeRule = z.infer<typeof SlackIntakeRuleSchema>;
const Choice = z.object({ id: z.string(), name: z.string() });
export const SlackSupportDirectorySchema = z.object({
  channels: z.array(Choice),
  people: z.array(Choice),
});
export type SlackSupportDirectory = z.infer<typeof SlackSupportDirectorySchema>;
export const SlackSupportActivitySchema = z.array(
  z.object({
    id: z.string(),
    issue_id: z.string(),
    channel_name: z.string(),
    state: z.string(),
    error: z.string(),
    updated_at: z.string(),
  }),
);
export type SlackSupportActivity = z.infer<typeof SlackSupportActivitySchema>;
