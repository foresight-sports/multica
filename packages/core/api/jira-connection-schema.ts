import { z } from "zod";

export const JiraConnectionSchema = z.object({
  configured: z.boolean(),
  status: z.enum(["disconnected", "connected", "reconnect_required"]),
  pending: z.boolean().default(false),
});
export type JiraConnection = z.infer<typeof JiraConnectionSchema>;
export const JiraConnectSchema = z.object({ redirect_url: z.string().url() });
export const JiraBlockersSchema = z.array(z.object({
  task_id: z.string().uuid(), agent_name: z.string(), can_resume: z.boolean(),
}));
export type JiraBlockers = z.infer<typeof JiraBlockersSchema>;
export const JiraResumeSchema = z.object({ task_id: z.string().uuid() });
