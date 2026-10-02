import { describe, expect, it } from "vitest";
import { isAgentRuntimeBound } from "./runtime-binding";

describe("isAgentRuntimeBound", () => {
  it("accepts portable agents without a bound machine", () => { expect(isAgentRuntimeBound({runtime_id:"",runtime_bound:true,portable_execution:true})).toBe(true); });
  it("accepts a bound response from new and old servers", () => {
    expect(
      isAgentRuntimeBound({ runtime_id: "runtime-1", runtime_bound: true }),
    ).toBe(true);
    expect(isAgentRuntimeBound({ runtime_id: "runtime-1" })).toBe(true);
  });

  it("rejects explicit and legacy unbound responses", () => {
    expect(
      isAgentRuntimeBound({ runtime_id: "", runtime_bound: false }),
    ).toBe(false);
    expect(isAgentRuntimeBound({ runtime_id: "" })).toBe(false);
  });

  it("fails closed when additive and legacy signals disagree", () => {
    expect(
      isAgentRuntimeBound({ runtime_id: "runtime-1", runtime_bound: false }),
    ).toBe(false);
    expect(
      isAgentRuntimeBound({ runtime_id: "", runtime_bound: true }),
    ).toBe(false);
  });

  it("fails closed for partial legacy payloads", () => {
    expect(
      isAgentRuntimeBound({
        runtime_id: undefined as unknown as string,
        runtime_bound: true,
      }),
    ).toBe(false);
  });
});
