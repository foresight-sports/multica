// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiClient } from "./client";
import { InstanceAgentSchema } from "./instance-schema";
import { parseAgentResponse } from "./agent-schema";

afterEach(() => vi.unstubAllGlobals());
describe("instance API", () => {
  it("drops malformed source identities rather than treating them as editable", () => {
    const instance = InstanceAgentSchema.parse({ id: "00000000-0000-4000-8000-000000000001", agent_id: "00000000-0000-4000-8000-000000000002", name: "Shared", description: "", instructions: "", revision: 1, enabled: true, runtime_bound: true, source_owner_id: false, source_agent_id: 123, source_workspace_id: "bad" });
    expect(instance.source_agent_id).toBeUndefined();
    expect(instance.source_owner_id).toBeUndefined();
    expect(instance.source_workspace_id).toBeUndefined();
    expect(() => parseAgentResponse({ id: "agent", instance_source_agent_id: {} })).toThrow("Could not load agent");
  });
  it("rejects malformed settings instead of treating them as an empty policy", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ revision: "1" }))));
    await expect(new ApiClient("https://example.test").getInstanceConfiguration()).rejects.toThrow("Could not load");
  });
  it("rejects malformed agent enablement instead of enabling a workspace accidentally", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify([{ id: "bad", enabled: "true" }]))));
    await expect(new ApiClient("https://example.test").listInstanceAgents()).rejects.toThrow("Could not load");
  });
  it("sends revision-checked instructions and validates the saved response", async () => {
    const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({ instructions: "Shared policy", revision: 3 })));
    vi.stubGlobal("fetch", fetch);
    const result = await new ApiClient("https://example.test").updateInstanceConfiguration({ instructions: "Shared policy", revision: 2 });
    expect(JSON.parse(fetch.mock.calls[0]![1].body)).toEqual({ instructions: "Shared policy", revision: 2 });
    expect(result.revision).toBe(3);
  });
  it("requires a confirmed identity after creating an agent", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({}))));
    await expect(new ApiClient("https://example.test").saveInstanceAgent({ name: "Reviewer", description: "", instructions: "", revision: 0 })).rejects.toThrow("Reload before retrying");
  });
});
