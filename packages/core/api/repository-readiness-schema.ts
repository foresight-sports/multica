import { z } from "zod";
export const RepositoryReadinessSchema = z.object({
 workspace_id: z.string(),
 state: z.string().default("preparing"),
 message: z.string().default(""),
 usable: z.boolean().default(false),
 updated_at: z.string().optional(),
}).transform((value)=>({...value,usable:value.usable && value.state === "ready"}));

export type RepositoryReadiness = z.infer<typeof RepositoryReadinessSchema>;
