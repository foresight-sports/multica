// @vitest-environment node
import { describe, it, expect } from "vitest";
import { EMPTY_AGENT_DRAFT, getDraftExecutionPolicy } from "./draft";
import { encodeBuilderInput, mergeBuilderDraft } from "./builder-protocol";
import {
  validateBuilderConfiguration,
  type BuilderConfigurationContext,
} from "./builder-configuration";
import type { RuntimeDevice } from "../types";
const draft = {
  ...EMPTY_AGENT_DRAFT,
  name: "Agent",
  runtimeId: "rt",
  model: "m",
};
const context: BuilderConfigurationContext = {
  canCreateAgents: true,
  canCreateInstanceAgents: true,
  runtimes: [
    {
      runtime: {
        id: "rt",
        name: "Workstation",
        provider: "codex",
        status: "online",
      } as RuntimeDevice,
      models: [
        {
          id: "m",
          label: "Model",
          thinking: { supported_levels: [{ value: "high", label: "High" }] },
          service_tiers: [{ id: "priority", name: "Fast" }],
        },
      ],
    },
  ],
};
const policy = () => ({
  ...getDraftExecutionPolicy(draft, undefined, true, "codex"),
  mode: "automatic" as const,
  router_profile: "creation-primary",
});
const merge = (p: Parameters<typeof mergeBuilderDraft>[1], c = context) =>
  mergeBuilderDraft(draft, p, new Set(), new Set(), new Set(["m"]), c);
describe("builder configuration", () => {
  it("provides profiles, permissions, and exact model capabilities without runtime metadata", () => {
    const input = encodeBuilderInput(
      "configure",
      draft,
      [],
      [],
      null,
      null,
      context,
    );
    const parsed = JSON.parse(input.slice(input.indexOf("\n") + 1));
    expect(parsed.current_draft.execution_policy.profiles[0]).not.toHaveProperty("runtime_id");
    expect(parsed.configuration_permissions.create_instance_agents).toBe(true);
    expect(
      parsed.available_execution_runtimes[0].models[0].thinking
        .supported_levels[0].value,
    ).toBe("high");
    expect(parsed.available_execution_runtimes[0]).not.toHaveProperty(
      "metadata",
    );
  });
  it("applies profiles, routing, scope, and primary capabilities together", () => {
    const p = policy();
    p.profiles[0]!.thinking_level = "high";
    p.profiles[0]!.service_tier = "priority";
    const result = merge({ execution_policy: p, instance_agent: true });
    expect(result.executionPolicy).toMatchObject({
      mode: "automatic",
      router_profile: "creation-primary",
      revision: 0,
    });
    expect(result).toMatchObject({
      instanceAgent: true,
      thinkingLevel: "high",
      serviceTier: "priority",
    });
  });
  it.each(["model", "runtime_id", "thinking_level", "service_tier"])(
    "rejects invented %s without changing the draft",
    (field) => {
      const p = policy();
      Object.assign(p.profiles[0]!, { [field]: "invented" });
      expect(merge({ execution_policy: p, name: "Should not apply" })).toBe(
        draft,
      );
    },
  );
  it("rejects invalid routing, rating, scope and conflicting primary fields", () => {
    expect(merge({ execution_policy: { ...policy(), mode: "invented" } })).toBe(
      draft,
    );
    expect(
      merge({ execution_policy: { ...policy(), default_profile: "missing" } }),
    ).toBe(draft);
    const p = policy();
    p.profiles[0]!.quality = 6;
    expect(merge({ execution_policy: p })).toBe(draft);
    expect(
      merge(
        { instance_agent: true },
        { ...context, canCreateInstanceAgents: false },
      ),
    ).toBe(draft);
    expect(merge({ execution_policy: policy(), model: "conflict" })).toBe(
      draft,
    );
  });
  it("validates extra runtime profiles and restricts custom routers", () => {
    const c = {
      ...context,
      runtimes: [
        ...context.runtimes,
        {
          ...context.runtimes[0]!,
          runtime: {
            ...context.runtimes[0]!.runtime,
            id: "rt2",
            provider: "custom-provider",
            profile_id: "custom",
          },
        },
      ],
    };
    const p = policy();
    p.profiles.push({ ...p.profiles[0]!, id: "extra", provider: "custom-provider" });
    p.default_profile = "extra";
    expect(
      validateBuilderConfiguration(draft, { execution_policy: p }, c),
    ).toBeNull();
    p.router_profile = "extra";
    expect(
      validateBuilderConfiguration(draft, { execution_policy: p }, c),
    ).toContain("built-in");
  });
  it("keeps profiles when omitted and does not invent capabilities with a missing catalog", () => {
    const current = { ...draft, executionPolicy: policy() };
    expect(
      mergeBuilderDraft(
        current,
        { name: "New" },
        new Set(),
        new Set(),
        null,
        context,
      ).executionPolicy,
    ).toBe(current.executionPolicy);
    expect(
      merge(
        { thinking_level: "high" },
        { ...context, runtimes: [{ ...context.runtimes[0]!, models: null }] },
      ),
    ).toBe(draft);
    expect(
      merge({ thinking_level: "high", service_tier: "priority" }),
    ).toMatchObject({ thinkingLevel: "high", serviceTier: "priority" });
  });
});

it("distinguishes an unavailable catalog from an unsupported profile setting", () => {
  const p = policy();
  p.profiles[0]!.thinking_level = "high";
  const loading = {
    ...context,
    runtimes: [{ ...context.runtimes[0]!, models: null }],
  };
  expect(
    validateBuilderConfiguration(draft, { execution_policy: p }, loading),
  ).toContain("Model discovery is unavailable");
  expect(
    validateBuilderConfiguration(draft, { execution_policy: p }, context),
  ).toBeNull();
  p.profiles[0]!.thinking_level = "invented";
  expect(
    validateBuilderConfiguration(draft, { execution_policy: p }, context),
  ).toContain('reasoning "invented"');
});
