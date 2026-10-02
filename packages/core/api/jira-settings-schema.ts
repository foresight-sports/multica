import { z } from "zod";
export const JiraSettingsSchema = z.object({
  property_id: z.string(), required: z.boolean(), revision: z.number(), can_edit: z.boolean(),
});
export type JiraSettings = z.infer<typeof JiraSettingsSchema>;
