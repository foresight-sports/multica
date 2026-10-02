import { z } from "zod";
export const InstanceUpdateSchema = z
  .object({
    channel: z.string(),
    current_version: z.string().default(""),
    latest_version: z.string().default(""),
    update_available: z.boolean().default(false),
    remote_supported: z.boolean().default(false),
    can_update: z.boolean().default(false),
    online: z.boolean().default(false),
    reason: z.string().default(""),
  })
  .transform((v) => ({
    channel: v.channel,
    currentVersion: v.current_version,
    latestVersion: v.latest_version,
    updateAvailable: v.update_available,
    remoteSupported: v.remote_supported,
    canUpdate: v.can_update,
    online: v.online,
    reason: v.reason,
  }));
export type InstanceUpdateInfo = z.infer<typeof InstanceUpdateSchema>;
export const RuntimeUpdateResponseSchema = z.object({
  id: z.string().min(1),
  runtime_id: z.string(),
  status: z.enum(["pending", "running", "completed", "failed", "timeout"]),
  target_version: z.string(),
  output: z
    .string()
    .nullish()
    .transform((v) => v ?? undefined),
  error: z
    .string()
    .nullish()
    .transform((v) => v ?? undefined),
  created_at: z.string(),
  updated_at: z.string().default(""),
  completed_at: z.string().nullish(),
});
