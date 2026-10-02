import { z } from "zod";
import {
  ExecutionPolicySchema,
  ExecutionProfileSchema,
} from "../api/execution-schema";
import type { RuntimeDevice, RuntimeModel } from "../types";
import type { AgentDraft } from "./draft";

export interface BuilderConfigurationContext {
  runtimes: Array<{ runtime: RuntimeDevice; models: RuntimeModel[] | null }>;
  canCreateAgents: boolean;
  canCreateInstanceAgents: boolean;
}
export interface BuilderConfigurationPayload {
  execution_policy?: unknown;
  thinking_level?: unknown;
  service_tier?: unknown;
  instance_agent?: unknown;
  model?: unknown;
}
const Profile = ExecutionProfileSchema.extend({
  id: z.string().min(1).max(64),
  provider: z.string().min(1).max(100),
  runtime_id: z.literal("").default(""),
  name: z.string().trim().min(1).max(100),
  model: z.string().min(1).max(256),
  purpose: z.string().max(2000).default(""),
  keywords: z.array(z.string().max(100)).max(30).default([]),
  required_tools: z.array(z.string().max(100)).max(20).default([]),
  required_os: z.enum(["", "windows", "linux", "darwin"]).default(""),
  quality: z.number().int().min(1).max(5).default(3),
  speed: z.number().int().min(1).max(5).default(3),
  cost: z.number().int().min(1).max(5).default(3),
});
export const BuilderPolicySchema = ExecutionPolicySchema.extend({
  mode: z.enum(["default", "automatic"]),
  preference: z.enum(["balanced", "quality", "speed", "cost"]),
  profiles: z.array(Profile).max(24),
});
function supported(
  model: RuntimeModel | undefined,
  thinking: string,
  tier: string,
): boolean {
  return (
    !!model &&
    (!thinking ||
      !!model.thinking?.supported_levels.some((l) => l.value === thinking)) &&
    (!tier ||
      !!model.service_tiers?.some((t) => t.id === tier) ||
      (tier === "default" &&
        model.supports_explicit_standard_service_tier === true))
  );
}
/** Reject an invalid configuration as a unit; never silently widen approved choices. */
export function validateBuilderConfiguration(
  current: AgentDraft,
  payload: BuilderConfigurationPayload,
  context: BuilderConfigurationContext,
): string | null {
  if (
    payload.instance_agent !== undefined &&
    (typeof payload.instance_agent !== "boolean" ||
      (payload.instance_agent
        ? !context.canCreateInstanceAgents
        : !context.canCreateAgents))
  )
    return "Agent scope is not permitted for your account.";
  if (payload.execution_policy !== undefined) {
    const parsed = BuilderPolicySchema.safeParse(payload.execution_policy);
    if (!parsed.success) return "Execution profiles contain invalid fields.";
    const policy = parsed.data;
    const ids = new Set(policy.profiles.map((p) => p.id));
    if (
      ids.size !== policy.profiles.length ||
      (policy.profiles.length && !ids.has(policy.default_profile)) ||
      (policy.router_profile && !ids.has(policy.router_profile))
    )
      return "Choose valid default and task-understanding profiles.";
    const primary = policy.profiles[0];
    for (const p of policy.profiles) {
      const catalog = context.runtimes.find(
        (c) =>
          c.runtime.provider === p.provider &&
          supported(
            c.models?.find((m) => m.id === p.model),
            p.thinking_level,
            p.service_tier,
          ),
      );
      if (!catalog) {
        const providerCatalogs = context.runtimes.filter(
          (c) => c.runtime.provider === p.provider,
        );
        if (!providerCatalogs.length)
          return `Profile "${p.name}" has no accessible provider.`;
        if (providerCatalogs.some((c) => c.models === null))
          return `Model discovery is unavailable for profile "${p.name}". Refresh the provider catalogs and reapply the proposal.`;
        const model = providerCatalogs
          .flatMap((c) => c.models ?? [])
          .find((m) => m.id === p.model);
        if (!model)
          return `Profile "${p.name}": model "${p.model}" is not advertised by an accessible machine.`;
        if (
          p.thinking_level &&
          !model.thinking?.supported_levels.some(
            (l) => l.value === p.thinking_level,
          )
        )
          return `Profile "${p.name}": reasoning "${p.thinking_level}" is not advertised for "${p.model}".`;
        return `Profile "${p.name}" has no accessible machine advertising this combination of model, reasoning and speed.`;
      }
      if (
        p.id === policy.router_profile &&
        (catalog.runtime.profile_id ||
          !["codex", "claude"].includes(catalog.runtime.provider))
      )
        return "Task understanding requires a built-in Codex or Claude runtime.";
    }
    if (
      primary &&
      ((payload.model !== undefined && payload.model !== primary.model) ||
        (payload.thinking_level !== undefined &&
          payload.thinking_level !== primary.thinking_level) ||
        (payload.service_tier !== undefined &&
          payload.service_tier !== primary.service_tier))
    )
      return "The primary model settings conflict with the first profile.";
  } else if (
    payload.thinking_level !== undefined ||
    payload.service_tier !== undefined
  ) {
    const model =
      typeof payload.model === "string" ? payload.model : current.model;
    const thinking =
      payload.thinking_level ??
      (model === current.model ? current.thinkingLevel : "");
    const tier =
      payload.service_tier ??
      (model === current.model ? current.serviceTier : "");
    if (typeof thinking !== "string" || typeof tier !== "string")
      return "Invalid reasoning or speed setting.";
    const unchanged =
      model === current.model &&
      thinking === current.thinkingLevel &&
      tier === current.serviceTier;
    if (
      !unchanged &&
      !supported(
        context.runtimes
          .find((c) =>
            current.executionPolicy?.profiles.length
              ? c.runtime.provider ===
                current.executionPolicy.profiles[0]?.provider
              : c.runtime.id === current.runtimeId,
          )
          ?.models?.find((m) => m.id === model),
        thinking,
        tier,
      )
    )
      return "The selected model does not advertise that reasoning or speed setting.";
  }
  return null;
}
