// @vitest-environment node
import { describe, it, expect, vi, afterEach } from "vitest";
import { ApiClient } from "./client";
import {
  ExecutionPolicySchema,
  ExecutionSelectionSchema,
} from "./execution-schema";
afterEach(() => vi.unstubAllGlobals());
describe("execution API", () => {
  it("keeps old tasks without profiles compatible", () => {
    expect(ExecutionPolicySchema.parse({}).profiles).toEqual([]);
    expect(
      ExecutionSelectionSchema.parse({ state: "pending" }).history,
    ).toEqual([]);
  });
  it("rejects a malformed policy instead of overwriting profiles", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(new Response(JSON.stringify({ profiles: "wrong" }))),
    );
    await expect(
      new ApiClient("https://example.test").getAgentExecution("a"),
    ).rejects.toThrow("Invalid execution policy");
  });
  it("sends exact per-task overrides", async () => {
    const fetch = vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify({ id: "task", status: "queued" })),
      );
    vi.stubGlobal("fetch", fetch);
    await new ApiClient("https://example.test").rerunIssue("i", "t", {
      profile_id: "deep",
      fresh_session: true,
    });
    const init = fetch.mock.calls[0]?.[1] as RequestInit;
    expect(JSON.parse(String(init.body))).toEqual({
      task_id: "t",
      execution: { profile_id: "deep", fresh_session: true },
    });
  });
});
