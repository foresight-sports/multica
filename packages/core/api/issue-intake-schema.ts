import { z } from "zod";
export const IssueIntakeSchema = z
  .object({
    revision: z.number().int().nonnegative().default(0),
    default_squad_id: z.string().default(""),
    projects: z.record(z.string(), z.string()).default({}),
  })
  .transform((v) => ({
    revision: v.revision,
    defaultSquadId: v.default_squad_id,
    projects: v.projects,
  }));
export type IssueIntake = z.infer<typeof IssueIntakeSchema>;
