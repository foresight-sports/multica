import { z } from "zod";

export const WorkspaceDirectorySchema = z.object({
  can_create: z.boolean().catch(false),
  truncated: z.boolean().default(false),
  workspaces: z.array(z.object({
    id: z.string(), name: z.string(), slug: z.string(),
    role: z.enum(["", "member", "admin", "owner"]).catch(""),
    request_status: z.enum(["", "pending", "approved", "declined", "cancelled"]).catch(""),
  })),
});
export type WorkspaceDirectoryData = z.infer<typeof WorkspaceDirectorySchema>;
export const WorkspaceAccessRequestsSchema = z.object({
  truncated: z.boolean().default(false),
  requests: z.array(z.object({
    workspace_id: z.string(), workspace_name: z.string(), user_id: z.string(),
    name: z.string(), email: z.string(), requested_at: z.string(),
  })),
});
export type WorkspaceAccessRequests = z.infer<typeof WorkspaceAccessRequestsSchema>;
