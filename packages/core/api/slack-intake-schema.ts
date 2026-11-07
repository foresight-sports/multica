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

export const SlackRelayStatusSchema = z.object({
  installations: z.array(
    z
      .object({
        id: z.string(),
        app_id: z.string().nullable(),
        team_id: z.string().nullable(),
        mode: z.string(),
        active: z.boolean(),
        credential_configured: z.boolean(),
        last_seen_at: z.string().datetime({ offset: true }).nullable(),
        contact_recent: z.boolean(),
      })
      .transform((v) => ({
        id: v.id,
        appId: v.app_id,
        teamId: v.team_id,
        mode: v.mode,
        active: v.active,
        credentialConfigured: v.credential_configured,
        lastSeenAt: v.last_seen_at,
        contactRecent: v.contact_recent,
      })),
  ),
  counts: z.record(z.string(), z.number().int().nonnegative()),
  recent: z.array(
    z
      .object({
        id: z.string(),
        installation_id: z.string(),
        channel_id: z.string(),
        message_ts: z.string(),
        owner: z.string(),
        state: z.string(),
        attempts: z.number().int().nonnegative(),
        last_error: z.string(),
        received_at: z.string().datetime({ offset: true }),
        updated_at: z.string().datetime({ offset: true }),
      })
      .transform((v) => ({
        id: v.id,
        installationId: v.installation_id,
        channelId: v.channel_id,
        messageTs: v.message_ts,
        owner: v.owner,
        state: v.state,
        attempts: v.attempts,
        lastError: v.last_error,
        receivedAt: v.received_at,
        updatedAt: v.updated_at,
      })),
  ),
});
export type SlackRelayStatus = z.infer<typeof SlackRelayStatusSchema>;
export const SlackRelayReplaySchema = z.object({ state: z.literal("pending") });
