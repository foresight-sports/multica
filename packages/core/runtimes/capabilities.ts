import { z } from "zod";
import { parseWithFallback } from "../api/schema";

export const RuntimeCapabilityInventorySchema = z.object({
  version: z.literal(1),
  provider: z.string(),
  scope: z.literal("user"),
  status: z.enum(["reported", "unknown", "unsupported"]),
  observed_at: z.string(),
  truncated: z.boolean().default(false),
  entries: z.array(z.object({
    kind: z.enum(["plugin", "skill", "mcp", "app"]),
    name: z.string().max(160),
    plugin: z.string().max(160).optional(),
    description: z.string().max(240).optional(),
    enabled: z.boolean().nullable(),
    callable: z.boolean().nullable(),
    auth: z.literal("unknown"),
  })).max(64),
});
export const MachineCapabilitiesSchema = z.object({
  runtime_capabilities: RuntimeCapabilityInventorySchema.nullable().optional().catch(null),
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
