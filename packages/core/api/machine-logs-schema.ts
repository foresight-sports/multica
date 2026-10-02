import { z } from "zod";

export const MachineLogsSchema = z.object({
  supported: z.boolean(),
  log: z.string(),
  crash: z.string(),
  message: z.string().default(""),
  truncated: z.boolean().default(false),
  received_at: z.string().datetime({ offset: true }).nullable(),
});
export type MachineLogs = z.infer<typeof MachineLogsSchema>;
