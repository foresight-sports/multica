import { z } from "zod";

export const InstanceMembersSchema = z.object({
  members: z.array(z.object({ id: z.string().uuid(), name: z.string(), email: z.string(), workspaces: z.array(z.string()), memberships: z.array(z.object({ workspace_id: z.string().uuid(), role: z.enum(["owner", "admin", "member"]) })).optional().default([]) })),
  workspaces: z.array(z.object({ id: z.string().uuid(), name: z.string(), slug: z.string() })).optional().default([]),
  truncated: z.boolean(),
});
export type InstanceMembers = z.infer<typeof InstanceMembersSchema>;

export const InstanceConfigurationSchema = z.object({
  instructions: z.string(),
  revision: z.number().int().positive(),
});
export type InstanceConfiguration = z.infer<typeof InstanceConfigurationSchema>;

export const InstanceAgentSchema = z.object({
  id: z.string().uuid(),
  name: z.string(),
  description: z.string(),
  instructions: z.string(),
  revision: z.number().int().positive(),
  agent_id: z.string().uuid(),
  source_owner_id: z.string().uuid().optional().catch(undefined),
  source_agent_id: z.string().uuid().optional().catch(undefined),
  source_workspace_id: z.string().uuid().optional().catch(undefined),
  enabled: z.boolean(),
  runtime_bound: z.boolean(),
});
export const InstanceAgentListSchema = z.array(InstanceAgentSchema);
export type InstanceAgent = z.infer<typeof InstanceAgentSchema>;
export type InstanceAgentInput = Pick<InstanceAgent, "name" | "description" | "instructions" | "revision">;
export const InstanceAgentSavedSchema = z.object({ id: z.string().uuid() });
