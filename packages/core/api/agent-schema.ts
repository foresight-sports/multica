import { z } from "zod";
import type { Agent } from "../types";
import { parseWithFallback } from "./schema";

// Validate shared identity before permission decisions. Keep existing agent
// fields intact for clients talking to newer servers.
export const AgentResponseSchema = z.object({
  id: z.string(),
  portable_execution: z.boolean().default(false),
  instance_agent_id: z.string().uuid().optional(),
  instance_source_agent_id: z.string().uuid().optional(),
}).loose();

export function parseAgentResponse(raw: unknown): Agent {
  const agent = parseWithFallback<Agent | null>(raw, AgentResponseSchema, null, { endpoint: "agent", sensitive: true });
  if (!agent) throw new Error("Could not load agent configuration. Reload before retrying.");
  return agent;
}

export function parseAgentListResponse(raw: unknown): Agent[] {
  const agents = parseWithFallback<Agent[] | null>(raw, z.array(AgentResponseSchema), null, { endpoint: "agents", sensitive: true });
  if (!agents) throw new Error("Could not load agent configurations.");
  return agents;
}
