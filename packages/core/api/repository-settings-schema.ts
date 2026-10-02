import { z } from "zod";
export const RepositorySettingsSchema = z.object({
  repository: z.string().default(""),
  folder: z.string().default(""),
  root: z.string().default(""),
  mode: z.enum(["", "worktree", "in_place"]).default(""),
  revision: z.number().int().nonnegative(),
  can_edit: z.boolean().default(false),
  supported: z.boolean().default(false),
});
export type RepositorySettings = z.infer<typeof RepositorySettingsSchema>;
