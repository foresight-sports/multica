import { z } from "zod";

const text = z.string().catch("").default("");
export const WorkRecordSchema = z.object({
  id: z.string().uuid(),
  kind: z.enum(["installation", "checkpoint", "decision", "blocker"]).catch("blocker"),
  state: text,
  revision: z.number().int().positive(),
  runtime_id: z.string().nullable().optional(),
  machine_owner_id: z.string().nullable().optional(),
  can_approve: z.boolean().catch(false).default(false),
  data: z.object({
    tool: text, version: text, source: text, scope: text, effects: text, reason: text,
    machine: text, verification: text, repository: text, branch: text, commit: text,
    pr_url: text, validation: text, next_step: text, owner: text, text, author_type: text,
    dependencies: z.array(z.object({ repository: z.string(), commit: z.string() })).catch([]).default([]),
  }),
});
export const WorkRecordsSchema = z.object({ records: z.array(WorkRecordSchema), truncated: z.boolean().default(false) });
export type WorkRecord = z.infer<typeof WorkRecordSchema>;

export type WorkRecords = z.infer<typeof WorkRecordsSchema>;
