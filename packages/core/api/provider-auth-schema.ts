import { z } from "zod";

export const ProviderAuthSchema = z.object({
  id: z.string().optional(),
  provider: z.enum(["claude", "codex"]).optional(),
  status: z.enum(["idle", "pending", "running", "completed", "failed"]),
}).refine((value) => value.status === "idle" || (!!value.id && !!value.provider), {
  message: "Authentication requests require an id and provider",
});
export type ProviderAuthStatus = z.infer<typeof ProviderAuthSchema>;
