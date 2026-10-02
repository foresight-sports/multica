// @vitest-environment node
import { describe, expect, it } from "vitest";
import { WorkRecordsSchema } from "./work-record-schema";
describe("work record boundary", () => {
  const record = { id: "10000000-0000-4000-8000-000000000001", kind: "installation", state: "pending", revision: 1, data: { tool: "fvm" } };
  it("never grants approval based on a malformed or absent capability", () => {
    const result = WorkRecordsSchema.parse({ records: [{ ...record, can_approve: "true" }] });
    expect(result.records[0]?.can_approve).toBe(false);
    expect(result.records[0]?.data.tool).toBe("fvm");
  });
  it("rejects corrupt data rather than hiding a pending approval", () => {
    expect(WorkRecordsSchema.safeParse({ records: [{ ...record, data: "base64" }] }).success).toBe(false);
    expect(WorkRecordsSchema.safeParse({ records: [{ ...record, revision: 0 }] }).success).toBe(false);
  });
});
