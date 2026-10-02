import { z } from "zod";
import { parseWithFallback } from "../api/schema";

export const MachineCapabilitiesSchema = z.object({
  os: z.string().default(""),
  tools: z.array(z.string()).default([]),
  execution_models: z.array(z.object({ id: z.string() })).default([]),
  execution_observed_at: z.string().optional(),
});
export function parseMachineCapabilities(
  metadata: unknown,
): z.infer<typeof MachineCapabilitiesSchema> {
  return parseWithFallback(
    metadata,
    MachineCapabilitiesSchema,
    { os: "", tools: [], execution_models: [] },
    { endpoint: "runtime capabilities", sensitive: true },
  );
}
