import { z } from "zod";
export const WorkspaceLinksSchema = z.array(
  z.object({
    id: z.string(),
    name: z.string(),
    slug: z.string(),
    description: z.string().default(""),
    linked: z.boolean(),
    can_edit: z.boolean().default(false),
  }),
);
export const WorkspaceRequestsSchema = z.array(
  z.object({
    id: z.string(),
    source_issue_id: z.string(),
    target_issue_id: z.string(),
    workspace_id: z.string(),
    workspace_slug: z.string(),
    identifier: z.string(),
    title: z.string(),
    status: z.string(),
    description: z.string().default(""),
    latest_reply: z.string().default(""),
    notification_error: z.string().default(""),
  }),
);
export type WorkspaceLinks = z.infer<typeof WorkspaceLinksSchema>;
export type WorkspaceRequests = z.infer<typeof WorkspaceRequestsSchema>;
